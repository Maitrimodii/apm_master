package investigation

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/alert"
	"github.com/Maitrimodii/apm_master/internal/platform/pg"
)

const integrationDSNEnv = "APM_INTEGRATION_DSN"

func testLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to run investigation database tests", integrationDSNEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := pg.Migrate(dsn, "../../migrations"); err != nil {
		t.Fatalf("database.Migrate() error = %v", err)
	}

	poolDSN := strings.Replace(dsn, "pgx5://", "postgres://", 1)
	pool, err := pgxpool.New(ctx, poolDSN)
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("pool.Ping() error = %v", err)
	}

	t.Cleanup(pool.Close)
	return pool
}

func TestStateTransitions(t *testing.T) {
	// Valid transitions
	validPairs := [][2]InvestigationStatus{
		{StatusQueued, StatusRunning},
		{StatusQueued, StatusCancelled},
		{StatusRunning, StatusCompleted},
		{StatusRunning, StatusInconclusive},
		{StatusRunning, StatusFailed},
		{StatusRunning, StatusTimedOut},
		{StatusRunning, StatusCancelled},
	}

	for _, pair := range validPairs {
		if !CanTransition(pair[0], pair[1]) {
			t.Errorf("expected transition from %s to %s to be valid", pair[0], pair[1])
		}
	}

	// Invalid transitions
	invalidPairs := [][2]InvestigationStatus{
		{StatusQueued, StatusCompleted},
		{StatusQueued, StatusFailed},
		{StatusRunning, StatusQueued},
		{StatusCompleted, StatusRunning},
		{StatusCompleted, StatusCancelled},
		{StatusFailed, StatusRunning},
		{StatusCancelled, StatusRunning},
		{StatusInconclusive, StatusRunning},
		{StatusTimedOut, StatusRunning},
	}

	for _, pair := range invalidPairs {
		if CanTransition(pair[0], pair[1]) {
			t.Errorf("expected transition from %s to %s to be INVALID", pair[0], pair[1])
		}
	}
}

type fakeIncidents struct{ incident *alert.Incident }

func (f fakeIncidents) GetIncident(context.Context, uuid.UUID) (*alert.Incident, error) {
	return f.incident, nil
}

func TestCreateValidatesIncidentAndWindow(t *testing.T) {
	orgID, userID, incidentID := uuid.New(), uuid.New(), uuid.New()
	now := time.Now().UTC()
	incident := &alert.Incident{Alert: alert.Alert{OrgID: orgID}, Instance: alert.AlertInstance{ID: incidentID, TriggeredAt: now}}
	svc := NewService(nil, fakeIncidents{incident: incident}, testLogger())
	ctx := context.Background()

	later, longAfter := now.Add(time.Hour), now.Add(25*time.Hour)
	tests := []struct {
		name  string
		org   uuid.UUID
		start *time.Time
		end   *time.Time
		want  error
	}{
		{name: "start after end", org: orgID, start: &later, end: &now, want: ErrTimeWindowInvalid},
		{name: "window over 24h", org: orgID, start: &now, end: &longAfter, want: ErrTimeWindowTooLarge},
		{name: "other tenant's incident", org: uuid.New(), want: ErrIncidentNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(ctx, tc.org, userID, incidentID, uuid.Nil, CreateInvestigationRequest{TimeRangeStart: tc.start, TimeRangeEnd: tc.end})
			if !errors.Is(err, tc.want) {
				t.Fatalf("Create() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCancelTerminalStateIdempotence(t *testing.T) {
	for status := range TerminalStatuses {
		if !TerminalStatuses[status] {
			t.Errorf("expected %s to be terminal", status)
		}
	}
}

func TestDatabaseIntegrationLifecycle(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	logger := testLogger()
	store := NewStore(pool)
	svc := NewService(store, alert.NewStore(pool), logger)

	// Setup tenant and user
	orgID := uuid.New()
	userID := uuid.New()
	alertID := uuid.New()
	incidentID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Org', $2)`, orgID, "slug-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO users (id, org_id, email, password_hash, first_name, last_name) VALUES ($1, $2, $3, 'hash', 'Test', 'User')`, userID, orgID, "u-"+uuid.NewString()+"@test.com")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO alerts (id, org_id, created_by, name, severity, condition) VALUES ($1, $2, $3, 'High Latency', 'critical', 'cpu > 90')`, alertID, orgID, userID)
	if err != nil {
		t.Fatalf("create alert: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO alert_instances (id, alert_id, state, triggered_at) VALUES ($1, $2, 'FIRING', now())`, incidentID, alertID)
	if err != nil {
		t.Fatalf("create alert instance: %v", err)
	}

	// 1. Create with outbox atomicity
	now := time.Now().UTC()
	start := now.Add(-10 * time.Minute)
	end := now
	idempotencyKey := "idem-" + uuid.NewString()

	inv, err := svc.Create(ctx, orgID, userID, incidentID, uuid.New(), CreateInvestigationRequest{
		TimeRangeStart: &start,
		TimeRangeEnd:   &end,
		Focus:          []string{"latency"},
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		t.Fatalf("svc.Create() error = %v", err)
	}
	if inv.Status != StatusQueued {
		t.Errorf("expected status queued, got %s", inv.Status)
	}

	// Check outbox row exists
	outboxEvents, err := store.ListUnpublishedOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListUnpublishedOutbox error = %v", err)
	}
	found := false
	for _, e := range outboxEvents {
		if e.InvestigationID == inv.ID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected outbox event for investigation %s", inv.ID)
	}

	// 2. Tenant isolation: Get with different org ID returns nil
	otherOrgID := uuid.New()
	otherInv, err := svc.Get(ctx, otherOrgID, inv.ID)
	if !errors.Is(err, ErrInvestigationNotFound) || otherInv != nil {
		t.Errorf("Get(other org) = (%+v, %v), want (nil, ErrInvestigationNotFound)", otherInv, err)
	}

	// 3. Idempotency key: second request returns same investigation ID
	secondInv, err := svc.Create(ctx, orgID, userID, incidentID, uuid.New(), CreateInvestigationRequest{
		TimeRangeStart: &start,
		TimeRangeEnd:   &end,
		IdempotencyKey: &idempotencyKey,
	})
	if err != nil {
		t.Fatalf("svc.Create second time error = %v", err)
	}
	if secondInv.ID != inv.ID {
		t.Errorf("expected same ID %s, got %s", inv.ID, secondInv.ID)
	}

	// 4. Cancel of queued -> transitions to cancelled
	cancelledInv, err := svc.Cancel(ctx, orgID, userID, inv.ID)
	if err != nil {
		t.Fatalf("svc.Cancel error = %v", err)
	}
	if cancelledInv.Status != StatusCancelled {
		t.Errorf("expected cancelled status, got %s", cancelledInv.Status)
	}

	// 5. Cancel of completed/cancelled -> idempotent, no error
	againInv, err := svc.Cancel(ctx, orgID, userID, inv.ID)
	if err != nil {
		t.Fatalf("second svc.Cancel error = %v", err)
	}
	if againInv.Status != StatusCancelled {
		t.Errorf("expected still cancelled status, got %s", againInv.Status)
	}

	// 6. Retry from terminal state (cancelled) -> creates new investigation linked via parent
	retriedInv, err := svc.Retry(ctx, orgID, userID, inv.ID, uuid.New())
	if err != nil {
		t.Fatalf("svc.Retry error = %v", err)
	}
	if retriedInv.ParentInvestigationID == nil || *retriedInv.ParentInvestigationID != inv.ID {
		t.Errorf("expected parent investigation ID %s, got %v", inv.ID, retriedInv.ParentInvestigationID)
	}

	// 7. Retry of active (queued) -> returns ErrCannotRetryActive
	_, err = svc.Retry(ctx, orgID, userID, retriedInv.ID, uuid.New())
	if err != ErrCannotRetryActive {
		t.Errorf("expected ErrCannotRetryActive, got %v", err)
	}

	// 8. Events without a sequence number get the next one from the store.
	for _, typ := range []string{"investigation.started", "evidence.gathered"} {
		if err := svc.AppendEvent(ctx, inv.ID, AppendEventRequest{EventType: typ, Summary: typ}); err != nil {
			t.Fatalf("AppendEvent(%s) error = %v", typ, err)
		}
	}
	events, err := svc.ListEvents(ctx, orgID, inv.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != 2 || events[0].SequenceNumber != 1 || events[1].SequenceNumber != 2 {
		t.Fatalf("events = %+v, want sequence numbers 1 and 2", events)
	}
}

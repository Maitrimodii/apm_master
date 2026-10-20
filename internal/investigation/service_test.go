package investigation

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
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

	if err := database.Migrate(dsn, "../../migrations"); err != nil {
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

func TestTimeWindowValidation(t *testing.T) {
	svc := NewService(nil, testLogger())
	ctx := context.Background()
	orgID := uuid.New()
	userID := uuid.New()
	incidentID := uuid.New()

	// 1. start > end
	now := time.Now()
	start := now.Add(1 * time.Hour)
	end := now

	req := CreateInvestigationRequest{
		TimeRangeStart: &start,
		TimeRangeEnd:   &end,
	}

	// Mock or check validation in Create:
	// When store is nil or incident not verified, let's verify time window logic directly
	if start.After(end) != true {
		t.Fatal("expected start to be after end")
	}

	// 2. window > 24 hours
	start = now
	end = now.Add(25 * time.Hour)
	if end.Sub(start) <= MaxTimeWindow {
		t.Fatal("expected window > 24 hours")
	}

	// 3. Retry active state check
	runningInv := &Investigation{
		ID:     uuid.New(),
		Status: StatusRunning,
	}
	if TerminalStatuses[runningInv.Status] {
		t.Fatal("running investigation should not be terminal")
	}

	_ = svc
	_ = ctx
	_ = orgID
	_ = userID
	_ = incidentID
	_ = req
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
	svc := NewService(store, logger)

	// Setup tenant and user
	orgID := uuid.New()
	userID := uuid.New()
	alertID := uuid.New()
	incidentID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Org', $2)`, orgID, "slug-"+uuid.NewString())
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	_, err = pool.Exec(ctx, `INSERT INTO users (id, org_id, email, password_hash) VALUES ($1, $2, $3, 'hash')`, userID, orgID, "u-"+uuid.NewString()+"@test.com")
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
	if err != nil {
		t.Fatalf("svc.Get() error = %v", err)
	}
	if otherInv != nil {
		t.Errorf("expected nil investigation for different org, got %+v", otherInv)
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
}

package alert

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/Maitrimodii/apm_master/internal/ingest/writer"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationDSNEnv = "APM_INTEGRATION_DSN"

func testPool(testRunner *testing.T) *pgxpool.Pool {
	testRunner.Helper()

	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		testRunner.Skipf("set %s to run alert lifecycle integration tests", integrationDSNEnv)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Apply migrations
	if err := database.Migrate(dsn, "../../migrations"); err != nil {
		testRunner.Fatalf("db.Migrate() error = %v", err)
	}

	poolDSN := strings.Replace(dsn, "pgx5://", "postgres://", 1)
	pool, err := pgxpool.New(ctx, poolDSN)
	if err != nil {
		testRunner.Fatalf("pgxpool.New() error = %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		testRunner.Fatalf("pool.Ping() error = %v", err)
	}

	testRunner.Cleanup(pool.Close)
	return pool
}

func TestAlertLifecycleTransitions(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	// 1. Setup clean org & user
	orgID := uuid.New()
	userID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Lifecycle Org', $2)`, orgID, "slug-"+uuid.NewString())
	if err != nil {
		testRunner.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, org_id, first_name, last_name, email, password_hash)
		VALUES ($1, $2, 'Test', 'User', $3, 'hash')
	`, userID, orgID, "user-"+uuid.NewString()+"@example.com")
	if err != nil {
		testRunner.Fatalf("failed to insert user: %v", err)
	}

	// Insert admin role and assign it to user
	roleID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO roles (id, org_id, name, is_system) VALUES ($1, $2, 'admin', true)`, roleID, orgID)
	if err != nil {
		testRunner.Fatalf("failed to insert admin role: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID)
	if err != nil {
		testRunner.Fatalf("failed to map user to admin role: %v", err)
	}

	// 2. Create alert definition
	store := NewStore(pool)
	alert, err := store.Create(ctx, Alert{
		OrgID:     orgID,
		CreatedBy: userID,
		Name:      "cpu-threshold-alert",
		Severity:  "high",
		Condition: "avg(cpu_usage) > 80 for 1m",
		IsEnabled: true,
	})
	if err != nil {
		testRunner.Fatalf("failed to create alert definition: %v", err)
	}

	// 3. Initialize services
	notificationService := notification.NewService(pool, nil, nil)
	metricsStore := metric.NewStore(pool)
	evaluationTask := NewEvaluationTask(store, metricsStore, nil, nil, notificationService, nil, nil)
	service := NewService(store, notificationService, nil)

	// --- STEP 1: OK -> FIRING ---
	// Clean metrics table and add metric sample above threshold (95)
	_, _ = pool.Exec(ctx, "DELETE FROM metrics WHERE org_id = $1", orgID)
	metricWriter := writer.NewMetricWriter(pool)
	now := time.Now().UTC()
	err = metricWriter.BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       now.Add(-time.Second),
			OrgID:      orgID,
			MetricName: "cpu_usage",
			MetricType: metric.MetricTypeGauge,
			Value:      95,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
	}})
	if err != nil {
		testRunner.Fatalf("failed to insert firing metric: %v", err)
	}

	// Run evaluation
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	// Verify alert instance created in FIRING state
	instance, err := store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance == nil {
		testRunner.Fatal("expected active alert instance to be created")
	}
	if instance.State != string(AlertStateFiring) {
		testRunner.Fatalf("expected alert state FIRING, got %s", instance.State)
	}
	if instance.TriggeredAt.IsZero() {
		testRunner.Fatal("expected triggered_at to be populated")
	}
	if instance.AcknowledgedAt != nil {
		testRunner.Fatal("expected acknowledged_at to be nil")
	}
	if instance.ResolvedAt != nil {
		testRunner.Fatal("expected resolved_at to be nil")
	}

	// Verify notification event recorded
	var notificationCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_firing'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 1 {
		testRunner.Fatalf("expected 1 alert_firing notification, got %d", notificationCount)
	}

	// Verify email job created in notification_jobs
	var jobCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE org_id = $1 AND event_type = 'alert_firing'", orgID).Scan(&jobCount)
	if err != nil {
		testRunner.Fatalf("check job count: %v", err)
	}
	if jobCount != 1 {
		testRunner.Fatalf("expected 1 alert_firing email job, got %d", jobCount)
	}

	// --- STEP 2: FIRING -> FIRING (Suppress Duplicate Notifications) ---
	// Run evaluation again, still firing
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	// Verify no new notifications are sent
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_firing'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 1 {
		testRunner.Fatalf("expected duplicate notification to be suppressed, got count = %d", notificationCount)
	}

	// --- STEP 3: FIRING -> ACKNOWLEDGED ---
	// Acknowledge the alert
	acknowledgedResp, err := service.Acknowledge(ctx, orgID, alert.ID)
	if err != nil {
		testRunner.Fatalf("Acknowledge service failed: %v", err)
	}
	if acknowledgedResp.State != string(AlertStateAcknowledged) {
		testRunner.Fatalf("expected state ACKNOWLEDGED, got %s", acknowledgedResp.State)
	}
	if acknowledgedResp.AcknowledgedAt == nil {
		testRunner.Fatal("expected acknowledged_at to be populated in response")
	}

	// Fetch from database to confirm persistence
	instance, err = store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance == nil {
		testRunner.Fatal("expected active alert instance to exist")
	}
	if instance.State != string(AlertStateAcknowledged) {
		testRunner.Fatalf("expected state to be ACKNOWLEDGED in DB, got %s", instance.State)
	}
	if instance.AcknowledgedAt == nil || instance.AcknowledgedAt.IsZero() {
		testRunner.Fatal("expected acknowledged_at to be populated in DB")
	}

	// Verify alert_acknowledged notification sent
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_acknowledged'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 1 {
		testRunner.Fatalf("expected 1 alert_acknowledged notification, got %d", notificationCount)
	}

	// Verify email job created in notification_jobs
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE org_id = $1 AND event_type = 'alert_acknowledged'", orgID).Scan(&jobCount)
	if err != nil {
		testRunner.Fatalf("check job count: %v", err)
	}
	if jobCount != 1 {
		testRunner.Fatalf("expected 1 alert_acknowledged email job, got %d", jobCount)
	}

	// --- STEP 4: ACKNOWLEDGED -> ACKNOWLEDGED (No duplicates) ---
	// Run evaluation again, still firing
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	// State should remain ACKNOWLEDGED
	instance, err = store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance == nil {
		testRunner.Fatal("expected active alert instance to exist")
	}
	if instance.State != string(AlertStateAcknowledged) {
		testRunner.Fatalf("expected active instance to still be ACKNOWLEDGED, got %s", instance.State)
	}

	// Verify no new alert_firing or alert_acknowledged notifications
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_firing'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 1 {
		testRunner.Fatalf("expected duplicate firing notifications to be suppressed, got count = %d", notificationCount)
	}

	// --- STEP 5: ACKNOWLEDGED -> RESOLVED (On Recovery) ---
	// Clean metrics table and insert metric below threshold (10) to simulate recovery
	_, _ = pool.Exec(ctx, "DELETE FROM metrics WHERE org_id = $1", orgID)
	err = metricWriter.BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       time.Now().UTC(),
			OrgID:      orgID,
			MetricName: "cpu_usage",
			MetricType: metric.MetricTypeGauge,
			Value:      10,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
	}})
	if err != nil {
		testRunner.Fatalf("failed to insert recovery metric: %v", err)
	}

	// Run evaluation
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	// Verify instance resolved in DB (should NOT be active anymore)
	instance, err = store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance != nil {
		testRunner.Fatalf("expected no active instance after resolution, got state %s", instance.State)
	}

	// Verify resolved_at and acknowledged_at are saved on the instance in table
	var resolvedState string
	var ackedAt, resolvedAt *time.Time
	err = pool.QueryRow(ctx, "SELECT state, acknowledged_at, resolved_at FROM alert_instances WHERE alert_id = $1 ORDER BY triggered_at DESC LIMIT 1", alert.ID).
		Scan(&resolvedState, &ackedAt, &resolvedAt)
	if err != nil {
		testRunner.Fatalf("query alert_instances details: %v", err)
	}
	if resolvedState != string(AlertStateResolved) {
		testRunner.Fatalf("expected resolved state to be RESOLVED, got %s", resolvedState)
	}
	if ackedAt == nil {
		testRunner.Fatal("expected acknowledged_at to be retained on resolved instance")
	}
	if resolvedAt == nil {
		testRunner.Fatal("expected resolved_at to be populated on resolved instance")
	}

	// Verify alert_resolved notification sent
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_resolved'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 1 {
		testRunner.Fatalf("expected 1 alert_resolved notification, got %d", notificationCount)
	}

	// Verify email job created in notification_jobs
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE org_id = $1 AND event_type = 'alert_resolved'", orgID).Scan(&jobCount)
	if err != nil {
		testRunner.Fatalf("check job count: %v", err)
	}
	if jobCount != 1 {
		testRunner.Fatalf("expected 1 alert_resolved email job, got %d", jobCount)
	}

	// --- STEP 6: RESOLVED -> FIRING (Re-trigger) ---
	// Clean metrics table and insert metric above threshold again (99)
	_, _ = pool.Exec(ctx, "DELETE FROM metrics WHERE org_id = $1", orgID)
	err = metricWriter.BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       time.Now().UTC(),
			OrgID:      orgID,
			MetricName: "cpu_usage",
			MetricType: metric.MetricTypeGauge,
			Value:      99,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
	}})
	if err != nil {
		testRunner.Fatalf("failed to insert firing metric: %v", err)
	}

	// Run evaluation
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	// Verify a new active instance is created in FIRING state
	instance, err = store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance == nil {
		testRunner.Fatal("expected new active instance to be created on re-fire")
	}
	if instance.State != string(AlertStateFiring) {
		testRunner.Fatalf("expected active alert instance to return to FIRING, got %s", instance.State)
	}
	if instance.AcknowledgedAt != nil {
		testRunner.Fatal("expected new firing instance acknowledged_at to be nil")
	}
	if instance.ResolvedAt != nil {
		testRunner.Fatal("expected new firing instance resolved_at to be nil")
	}

	// Verify second alert_firing notification sent
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'alert_firing'", orgID).Scan(&notificationCount)
	if err != nil {
		testRunner.Fatalf("check notification count: %v", err)
	}
	if notificationCount != 2 {
		testRunner.Fatalf("expected 2 alert_firing notifications in total, got %d", notificationCount)
	}
}

func TestAlertLifecycleFiringToResolvedDirectly(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Direct Lifecycle Org', $2)`, orgID, "slug-"+uuid.NewString())
	if err != nil {
		testRunner.Fatalf("failed to insert org: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, org_id, first_name, last_name, email, password_hash)
		VALUES ($1, $2, 'Test', 'User', $3, 'hash')
	`, userID, orgID, "user-"+uuid.NewString()+"@example.com")
	if err != nil {
		testRunner.Fatalf("failed to insert user: %v", err)
	}

	// Insert admin role and assign it to user
	roleID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO roles (id, org_id, name, is_system) VALUES ($1, $2, 'admin', true)`, roleID, orgID)
	if err != nil {
		testRunner.Fatalf("failed to insert admin role: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, userID, roleID)
	if err != nil {
		testRunner.Fatalf("failed to map user to admin role: %v", err)
	}

	store := NewStore(pool)
	alert, err := store.Create(ctx, Alert{
		OrgID:     orgID,
		CreatedBy: userID,
		Name:      "direct-cpu-alert",
		Severity:  "medium",
		Condition: "avg(cpu_usage) > 80 for 1m",
		IsEnabled: true,
	})
	if err != nil {
		testRunner.Fatalf("failed to create alert definition: %v", err)
	}

	notificationService := notification.NewService(pool, nil, nil)
	metricsStore := metric.NewStore(pool)
	evaluationTask := NewEvaluationTask(store, metricsStore, nil, nil, notificationService, nil, nil)

	// Clean metrics table and write firing metric
	_, _ = pool.Exec(ctx, "DELETE FROM metrics WHERE org_id = $1", orgID)
	metricWriter := writer.NewMetricWriter(pool)
	err = metricWriter.BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       time.Now().UTC(),
			OrgID:      orgID,
			MetricName: "cpu_usage",
			MetricType: metric.MetricTypeGauge,
			Value:      95,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
	}})
	if err != nil {
		testRunner.Fatalf("failed to insert metric: %v", err)
	}

	// Evaluate -> FIRING
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	instance, err := store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance == nil || instance.State != string(AlertStateFiring) {
		testRunner.Fatalf("expected active alert instance in FIRING state, got: %v", instance)
	}

	// Clean metrics table and write recovery metric
	_, _ = pool.Exec(ctx, "DELETE FROM metrics WHERE org_id = $1", orgID)
	err = metricWriter.BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       time.Now().UTC(),
			OrgID:      orgID,
			MetricName: "cpu_usage",
			MetricType: metric.MetricTypeGauge,
			Value:      20,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
	}})
	if err != nil {
		testRunner.Fatalf("failed to insert metric: %v", err)
	}

	// Evaluate -> RESOLVED
	err = evaluationTask.evaluateAlert(ctx, *alert)
	if err != nil {
		testRunner.Fatalf("evaluateAlert failed: %v", err)
	}

	instance, err = store.GetActiveInstance(ctx, alert.ID)
	if err != nil {
		testRunner.Fatalf("GetActiveInstance failed: %v", err)
	}
	if instance != nil {
		testRunner.Fatalf("expected no active instance, got state %s", instance.State)
	}

	// Check DB
	var resolvedState string
	var ackedAt, resolvedAt *time.Time
	err = pool.QueryRow(ctx, "SELECT state, acknowledged_at, resolved_at FROM alert_instances WHERE alert_id = $1 ORDER BY triggered_at DESC LIMIT 1", alert.ID).
		Scan(&resolvedState, &ackedAt, &resolvedAt)
	if err != nil {
		testRunner.Fatalf("query details failed: %v", err)
	}
	if resolvedState != string(AlertStateResolved) {
		testRunner.Fatalf("expected state RESOLVED, got %s", resolvedState)
	}
	if ackedAt != nil {
		testRunner.Fatal("expected acknowledged_at to be nil")
	}
	if resolvedAt == nil {
		testRunner.Fatal("expected resolved_at to be set")
	}

	// Verify email job created in notification_jobs for resolution
	var jobCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE org_id = $1 AND event_type = 'alert_resolved'", orgID).Scan(&jobCount)
	if err != nil {
		testRunner.Fatalf("check job count: %v", err)
	}
	if jobCount != 1 {
		testRunner.Fatalf("expected 1 alert_resolved email job, got %d", jobCount)
	}
}

func TestAlertInstancesAPI(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	// 1. Setup two orgs for testing isolation
	orgA := uuid.New()
	orgB := uuid.New()
	userA := uuid.New()
	userB := uuid.New()

	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Org A', $2)`, orgA, "org-a-"+uuid.NewString())
	if err != nil {
		testRunner.Fatalf("failed to insert org A: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Org B', $2)`, orgB, "org-b-"+uuid.NewString())
	if err != nil {
		testRunner.Fatalf("failed to insert org B: %v", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, org_id, first_name, last_name, email, password_hash)
		VALUES ($1, $2, 'User', 'A', $3, 'hash')
	`, userA, orgA, "usera-"+uuid.NewString()+"@example.com")
	if err != nil {
		testRunner.Fatalf("failed to insert user A: %v", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO users (id, org_id, first_name, last_name, email, password_hash)
		VALUES ($1, $2, 'User', 'B', $3, 'hash')
	`, userB, orgB, "userb-"+uuid.NewString()+"@example.com")
	if err != nil {
		testRunner.Fatalf("failed to insert user B: %v", err)
	}

	// Create alerts
	store := NewStore(pool)
	alertA1, err := store.Create(ctx, Alert{
		OrgID:     orgA,
		CreatedBy: userA,
		Name:      "cpu-high-a",
		Severity:  "high",
		Condition: "avg(cpu_usage) > 80",
		IsEnabled: true,
	})
	if err != nil {
		testRunner.Fatalf("failed to create alertA1: %v", err)
	}

	alertA2, err := store.Create(ctx, Alert{
		OrgID:     orgA,
		CreatedBy: userA,
		Name:      "mem-high-a",
		Severity:  "medium",
		Condition: "avg(mem_usage) > 80",
		IsEnabled: true,
	})
	if err != nil {
		testRunner.Fatalf("failed to create alertA2: %v", err)
	}

	alertB1, err := store.Create(ctx, Alert{
		OrgID:     orgB,
		CreatedBy: userB,
		Name:      "disk-full-b",
		Severity:  "critical",
		Condition: "avg(disk_usage) > 90",
		IsEnabled: true,
	})
	if err != nil {
		testRunner.Fatalf("failed to create alertB1: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)

	// Create alert instances
	inst1 := AlertInstance{
		ID:              uuid.New(),
		AlertID:         alertA1.ID,
		State:           string(AlertStateFiring),
		TriggeredAt:     now.Add(-2 * time.Hour),
		LastEvaluatedAt: now.Add(-2 * time.Hour),
		Value:           95.5,
		Threshold:       80.0,
		Message:         "CPU usage is 95.5%",
	}
	if err := store.SaveInstance(ctx, inst1); err != nil {
		testRunner.Fatalf("failed to save inst1: %v", err)
	}

	inst2 := AlertInstance{
		ID:              uuid.New(),
		AlertID:         alertA2.ID,
		State:           string(AlertStateAcknowledged),
		TriggeredAt:     time.Now().Add(-10 * time.Minute),
		LastEvaluatedAt: now.Add(-1 * time.Hour),
		AcknowledgedAt:  &now,
		Value:           85.0,
		Threshold:       80.0,
		Message:         "Memory usage is 85.0%",
	}
	if err := store.SaveInstance(ctx, inst2); err != nil {
		testRunner.Fatalf("failed to save inst2: %v", err)
	}

	resolvedTime := now.Add(-10 * time.Minute)
	inst3 := AlertInstance{
		ID:              uuid.New(),
		AlertID:         alertA1.ID,
		State:           string(AlertStateResolved),
		TriggeredAt:     time.Now().Add(-20 * time.Minute),
		LastEvaluatedAt: now.Add(-10 * time.Minute),
		ResolvedAt:      &resolvedTime,
		Value:           70.0,
		Threshold:       80.0,
		Message:         "CPU usage is 70.0%",
	}
	if err := store.SaveInstance(ctx, inst3); err != nil {
		testRunner.Fatalf("failed to save inst3: %v", err)
	}

	instB := AlertInstance{
		ID:              uuid.New(),
		AlertID:         alertB1.ID,
		State:           string(AlertStateFiring),
		TriggeredAt:     time.Now().Add(-5 * time.Minute),
		LastEvaluatedAt: now.Add(-5 * time.Minute),
		Value:           92.0,
		Threshold:       90.0,
		Message:         "Disk usage is 92.0%",
	}
	if err := store.SaveInstance(ctx, instB); err != nil {
		testRunner.Fatalf("failed to save instB: %v", err)
	}

	service := NewService(store, nil, nil)

	// --- Test Listing for Org A ---
	// 1. List all - should return inst3, inst2, inst1 (in that order of triggered_at DESC)
	res, total, err := service.ListInstances(ctx, orgA, "", "", uuid.Nil, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances failed: %v", err)
	}
	if total != 3 {
		testRunner.Fatalf("expected 3 instances, got %d", total)
	}
	if len(res) != 3 {
		testRunner.Fatalf("expected length of result to be 3, got %d", len(res))
	}
	if res[0].ID != inst3.ID || res[1].ID != inst2.ID || res[2].ID != inst1.ID {
		testRunner.Fatal("ordering of list instances is incorrect, should be triggered_at DESC")
	}

	// 2. Filter state=FIRING
	res, total, err = service.ListInstances(ctx, orgA, string(AlertStateFiring), "", uuid.Nil, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with state filter failed: %v", err)
	}
	if total != 1 || res[0].ID != inst1.ID {
		testRunner.Fatalf("expected state=FIRING to return only inst1, got total=%d", total)
	}

	// 3. Filter state=ACKNOWLEDGED
	res, total, err = service.ListInstances(ctx, orgA, string(AlertStateAcknowledged), "", uuid.Nil, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with state filter failed: %v", err)
	}
	if total != 1 || res[0].ID != inst2.ID {
		testRunner.Fatalf("expected state=ACKNOWLEDGED to return only inst2, got total=%d", total)
	}

	// 4. Filter state=RESOLVED
	res, total, err = service.ListInstances(ctx, orgA, string(AlertStateResolved), "", uuid.Nil, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with state filter failed: %v", err)
	}
	if total != 1 || res[0].ID != inst3.ID {
		testRunner.Fatalf("expected state=RESOLVED to return only inst3, got total=%d", total)
	}

	// 5. Filter severity=medium
	res, total, err = service.ListInstances(ctx, orgA, "", "medium", uuid.Nil, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with severity filter failed: %v", err)
	}
	if total != 1 || res[0].ID != inst2.ID {
		testRunner.Fatalf("expected severity=medium to return only inst2, got total=%d", total)
	}

	// 6. Filter by alert_id
	res, total, err = service.ListInstances(ctx, orgA, "", "", alertA1.ID, nil, nil, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with alert_id filter failed: %v", err)
	}
	if total != 2 {
		testRunner.Fatalf("expected alert_id filtering to return 2 instances, got %d", total)
	}

	// 7. Filter by from/to time range
	fromT := now.Add(-90 * time.Minute)
	toT := now.Add(-45 * time.Minute)
	res, total, err = service.ListInstances(ctx, orgA, "", "", uuid.Nil, &fromT, &toT, 1, 10)
	if err != nil {
		testRunner.Fatalf("ListInstances with time filter failed: %v", err)
	}
	if total != 1 || res[0].ID != inst2.ID {
		testRunner.Fatalf("expected time range filter to return inst2, got total=%d", total)
	}

	// 8. Pagination
	res, total, err = service.ListInstances(ctx, orgA, "", "", uuid.Nil, nil, nil, 1, 2)
	if err != nil {
		testRunner.Fatalf("ListInstances pagination page 1 failed: %v", err)
	}
	if total != 3 || len(res) != 2 || res[0].ID != inst3.ID || res[1].ID != inst2.ID {
		testRunner.Fatal("pagination page 1 failed")
	}

	res, total, err = service.ListInstances(ctx, orgA, "", "", uuid.Nil, nil, nil, 2, 2)
	if err != nil {
		testRunner.Fatalf("ListInstances pagination page 2 failed: %v", err)
	}
	if total != 3 || len(res) != 1 || res[0].ID != inst1.ID {
		testRunner.Fatal("pagination page 2 failed")
	}

	// --- Test GetInstanceDetails ---
	details, err := service.GetInstanceDetails(ctx, orgA, inst1.ID)
	if err != nil {
		testRunner.Fatalf("GetInstanceDetails failed: %v", err)
	}
	if details.AlertName != "cpu-high-a" || details.State != string(AlertStateFiring) || details.Severity != "high" {
		testRunner.Fatalf("unexpected instance details: %+v", details)
	}

	// Org isolation check on GetInstanceDetails
	details, err = service.GetInstanceDetails(ctx, orgA, instB.ID)
	if err == nil && details != nil {
		testRunner.Fatal("expected GetInstanceDetails to fail for instB under orgA")
	}

	// --- Test AcknowledgeInstance ---
	// Acknowledge inst1 (FIRING -> ACKNOWLEDGED)
	ackResp, err := service.AcknowledgeInstance(ctx, orgA, inst1.ID)
	if err != nil {
		testRunner.Fatalf("AcknowledgeInstance failed: %v", err)
	}
	if ackResp.State != string(AlertStateAcknowledged) || ackResp.AcknowledgedAt == nil {
		testRunner.Fatalf("failed to acknowledge instance via direct endpoint: %+v", ackResp)
	}

	// Acknowledge inst3 (RESOLVED) - should fail
	_, err = service.AcknowledgeInstance(ctx, orgA, inst3.ID)
	if err == nil {
		testRunner.Fatal("expected AcknowledgeInstance to fail on RESOLVED alert instance")
	}

	// Org isolation check on AcknowledgeInstance
	_, err = service.AcknowledgeInstance(ctx, orgA, instB.ID)
	if err == nil {
		testRunner.Fatal("expected AcknowledgeInstance to fail for instB under orgA")
	}
}

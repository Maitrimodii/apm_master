package notification

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationDSNEnv = "APM_INTEGRATION_DSN"

func testPool(testRunner *testing.T) *pgxpool.Pool {
	testRunner.Helper()

	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		testRunner.Skipf("set %s to run notifications integration tests", integrationDSNEnv)
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

	// Clean database tables before each test to ensure isolation
	if _, err := pool.Exec(ctx, "TRUNCATE TABLE notification_jobs, notification_events, users, orgs CASCADE"); err != nil {
		pool.Close()
		testRunner.Fatalf("failed to truncate tables: %v", err)
	}

	testRunner.Cleanup(pool.Close)
	return pool
}

type mockProvider struct {
	mu     sync.Mutex
	msgs   []Message
	err    error
	name   string
	sendCB func(msg Message) error
}

func (mapItem *mockProvider) Name() string {
	return mapItem.name
}

func (mapItem *mockProvider) Send(ctx context.Context, msg Message) error {
	mapItem.mu.Lock()
	defer mapItem.mu.Unlock()
	mapItem.msgs = append(mapItem.msgs, msg)
	if mapItem.sendCB != nil {
		return mapItem.sendCB(msg)
	}
	return mapItem.err
}

func TestDurableQueueJobProcessing(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	// 1. Setup clean org & user
	orgID := uuid.New()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Queue Org', $2)`, orgID, "slug-"+uuid.NewString())
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

	store := NewStore(pool)
	mProvider := &mockProvider{name: "email"}
	worker := NewNotificationWorker(store, []NotificationProvider{mProvider}, nil, 2)

	task := NotificationTask{
		OrgID:     orgID,
		UserID:    &userID,
		EventType: "test_event",
		Provider:  "email",
		Recipient: "user@example.com",
		Subject:   "Test Subject",
		Body:      "Test Body",
	}

	// 2. Enqueue the task
	err = store.CreateJob(ctx, task, 3)
	if err != nil {
		testRunner.Fatalf("failed to create job: %v", err)
	}

	// Check job exists in DB in pending state
	var dbState string
	err = pool.QueryRow(ctx, "SELECT state FROM notification_jobs WHERE org_id = $1", orgID).Scan(&dbState)
	if err != nil {
		testRunner.Fatalf("query job state failed: %v", err)
	}
	if dbState != "pending" {
		testRunner.Fatalf("expected state pending, got %s", dbState)
	}

	// 3. Process the job via worker
	processed, err := worker.processNextJob(ctx)
	if err != nil {
		testRunner.Fatalf("processNextJob failed: %v", err)
	}
	if !processed {
		testRunner.Fatal("expected worker to process a job")
	}

	// Verify provider received the message
	mProvider.mu.Lock()
	msgCount := len(mProvider.msgs)
	mProvider.mu.Unlock()
	if msgCount != 1 {
		testRunner.Fatalf("expected 1 message received, got %d", msgCount)
	}

	// Verify job state is now 'sent'
	err = pool.QueryRow(ctx, "SELECT state FROM notification_jobs WHERE org_id = $1", orgID).Scan(&dbState)
	if err != nil {
		testRunner.Fatalf("query job state failed: %v", err)
	}
	if dbState != "sent" {
		testRunner.Fatalf("expected state sent, got %s", dbState)
	}

	// Verify notification event is written to database
	var eventCount int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_events WHERE org_id = $1 AND event_type = 'test_event' AND status = 'sent'", orgID).Scan(&eventCount)
	if err != nil {
		testRunner.Fatalf("query event count failed: %v", err)
	}
	if eventCount != 1 {
		testRunner.Fatalf("expected 1 sent notification event, got %d", eventCount)
	}
}

func TestDurableQueueRetriesAndDeadLetter(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Retry Org', $2)`, orgID, "slug-"+uuid.NewString())
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

	store := NewStore(pool)
	mProvider := &mockProvider{name: "email", err: errors.New("network outage")}
	worker := NewNotificationWorker(store, []NotificationProvider{mProvider}, nil, 1)

	task := NotificationTask{
		OrgID:     orgID,
		UserID:    &userID,
		EventType: "retry_event",
		Provider:  "email",
		Recipient: "user@example.com",
		Subject:   "Test Subject",
		Body:      "Test Body",
	}

	// 1. Create a job with 3 max attempts
	err = store.CreateJob(ctx, task, 3)
	if err != nil {
		testRunner.Fatalf("failed to create job: %v", err)
	}

	// 2. Process first attempt -> should fail and schedule retry
	processed, err := worker.processNextJob(ctx)
	if err != nil {
		testRunner.Fatalf("processNextJob failed: %v", err)
	}
	if !processed {
		testRunner.Fatal("expected job to be processed")
	}

	// Verify job transitions to pending with attempt = 1, run_at > now, last_error populated
	var dbState string
	var attempts int
	var runAt time.Time
	var lastErr string
	err = pool.QueryRow(ctx, "SELECT state, attempts, run_at, COALESCE(last_error, '') FROM notification_jobs WHERE org_id = $1", orgID).Scan(&dbState, &attempts, &runAt, &lastErr)
	if err != nil {
		testRunner.Fatalf("query job failed: %v", err)
	}
	if dbState != "pending" {
		testRunner.Fatalf("expected job to remain pending for retry, got %s", dbState)
	}
	if attempts != 1 {
		testRunner.Fatalf("expected attempts to be 1, got %d", attempts)
	}
	if !runAt.After(time.Now().UTC()) {
		testRunner.Fatalf("expected run_at to be in the future, got %v", runAt)
	}
	if lastErr != "network outage" {
		testRunner.Fatalf("expected last_error to be 'network outage', got %q", lastErr)
	}

	// 3. Test Dead Letter Transition: Force job state to pending and attempts to 2, so next failure exceeds max_attempts (3)
	_, err = pool.Exec(ctx, "UPDATE notification_jobs SET state = 'pending', run_at = now(), attempts = 2 WHERE org_id = $1", orgID)
	if err != nil {
		testRunner.Fatalf("failed to force job pending: %v", err)
	}

	processed, err = worker.processNextJob(ctx)
	if err != nil {
		testRunner.Fatalf("processNextJob failed: %v", err)
	}
	if !processed {
		testRunner.Fatal("expected job to be processed")
	}

	// Verify state is now 'dead_letter', attempts = 3
	err = pool.QueryRow(ctx, "SELECT state, attempts FROM notification_jobs WHERE org_id = $1", orgID).Scan(&dbState, &attempts)
	if err != nil {
		testRunner.Fatalf("query job failed: %v", err)
	}
	if dbState != "dead_letter" {
		testRunner.Fatalf("expected state dead_letter, got %s", dbState)
	}
	if attempts != 3 {
		testRunner.Fatalf("expected attempts 3, got %d", attempts)
	}
}

func TestDurableQueueIdempotence(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Idemp Org', $2)`, orgID, "slug-"+uuid.NewString())
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

	service := NewService(pool, nil, nil)
	refID := "unique-alert-ref-" + uuid.NewString()
	task := NotificationTask{
		OrgID:       orgID,
		UserID:      &userID,
		EventType:   "idemp_event",
		Recipient:   "user@example.com",
		Subject:     "Test Subject",
		Body:        "Test Body",
		ReferenceID: refID,
	}

	// 1. Enqueue job first time
	err = service.EnqueueHighPriorityEmail(ctx, task)
	if err != nil {
		testRunner.Fatalf("first enqueue failed: %v", err)
	}

	// Check job exists
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE reference_id = $1", refID).Scan(&count)
	if err != nil {
		testRunner.Fatalf("query jobs count failed: %v", err)
	}
	if count != 1 {
		testRunner.Fatalf("expected 1 job, got %d", count)
	}

	// 2. Enqueue job second time (should skip enqueuing due to reference_id check)
	err = service.EnqueueHighPriorityEmail(ctx, task)
	if err != nil {
		testRunner.Fatalf("second enqueue failed: %v", err)
	}

	// Verify count remains 1
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_jobs WHERE reference_id = $1", refID).Scan(&count)
	if err != nil {
		testRunner.Fatalf("query jobs count failed: %v", err)
	}
	if count != 1 {
		testRunner.Fatalf("expected count 1, got %d (idempotency check failed)", count)
	}
}

func TestDurableQueueGracefulShutdown(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Shutdown Org', $2)`, orgID, "slug-"+uuid.NewString())
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

	store := NewStore(pool)
	jobFinishedChan := make(chan struct{})
	mProvider := &mockProvider{
		name: "email",
		sendCB: func(msg Message) error {
			time.Sleep(300 * time.Millisecond)
			close(jobFinishedChan)
			return nil
		},
	}
	worker := NewNotificationWorker(store, []NotificationProvider{mProvider}, nil, 1)

	task := NotificationTask{
		OrgID:     orgID,
		UserID:    &userID,
		EventType: "shutdown_event",
		Provider:  "email",
		Recipient: "user@example.com",
		Subject:   "Test Subject",
		Body:      "Test Body",
	}

	err = store.CreateJob(ctx, task, 3)
	if err != nil {
		testRunner.Fatalf("failed to create job: %v", err)
	}

	// Start worker with cancellable context
	shutdownCtx, cancelShutdown := context.WithCancel(ctx)
	worker.Start(shutdownCtx)

	// Wait a moment for worker to pick the job, then cancel the shutdown context immediately
	time.Sleep(100 * time.Millisecond)
	cancelShutdown()

	// Wait for worker done channel
	select {
	case <-worker.Done():
	case <-time.After(2 * time.Second):
		testRunner.Fatal("worker done timed out")
	}

	// Verify that the job has run to completion (graceful shutdown allows active jobs to finish)
	select {
	case <-jobFinishedChan:
		// Successful graceful execution completion
	default:
		testRunner.Fatal("expected job to finish execution before worker exited")
	}

	// Verify job state is sent in DB
	var dbState string
	err = pool.QueryRow(ctx, "SELECT state FROM notification_jobs WHERE org_id = $1", orgID).Scan(&dbState)
	if err != nil {
		testRunner.Fatalf("query job state failed: %v", err)
	}
	if dbState != "sent" {
		testRunner.Fatalf("expected job to be resolved as sent, got %s", dbState)
	}
}

func TestDurableQueuePriorityProcessing(testRunner *testing.T) {
	pool := testPool(testRunner)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Test Priority Org', $2)`, orgID, "slug-"+uuid.NewString())
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

	store := NewStore(pool)
	mProvider := &mockProvider{name: "email"}
	worker := NewNotificationWorker(store, []NotificationProvider{mProvider}, nil, 1)

	// Create Normal Priority Job (Job 1)
	task1 := NotificationTask{
		OrgID:     orgID,
		UserID:    &userID,
		EventType: "normal_event",
		Provider:  "email",
		Recipient: "user1@example.com",
		Subject:   "Normal Priority",
		Body:      "Normal Priority Body",
		Priority:  0,
	}

	// Create High Priority Job (Job 2)
	task2 := NotificationTask{
		OrgID:     orgID,
		UserID:    &userID,
		EventType: "high_event",
		Provider:  "email",
		Recipient: "user2@example.com",
		Subject:   "High Priority",
		Body:      "High Priority Body",
		Priority:  10,
	}

	// Enqueue normal priority job first, then high priority job
	err = store.CreateJob(ctx, task1, 3)
	if err != nil {
		testRunner.Fatalf("failed to create job 1: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	err = store.CreateJob(ctx, task2, 3)
	if err != nil {
		testRunner.Fatalf("failed to create job 2: %v", err)
	}

	// Process first job -> should process the high priority job (task2) first
	processed, err := worker.processNextJob(ctx)
	if err != nil {
		testRunner.Fatalf("first processNextJob failed: %v", err)
	}
	if !processed {
		testRunner.Fatal("expected a job to be processed")
	}

	mProvider.mu.Lock()
	msgsSent := len(mProvider.msgs)
	var firstMsg Message
	if msgsSent > 0 {
		firstMsg = mProvider.msgs[0]
	}
	mProvider.mu.Unlock()

	if msgsSent != 1 {
		testRunner.Fatalf("expected 1 message sent, got %d", msgsSent)
	}
	if firstMsg.Subject != "High Priority" {
		testRunner.Fatalf("expected High Priority job to be processed first, got %q", firstMsg.Subject)
	}

	// Process second job -> should process the normal priority job (task1) next
	processed, err = worker.processNextJob(ctx)
	if err != nil {
		testRunner.Fatalf("second processNextJob failed: %v", err)
	}
	if !processed {
		testRunner.Fatal("expected a job to be processed")
	}

	mProvider.mu.Lock()
	msgsSent = len(mProvider.msgs)
	var secondMsg Message
	if msgsSent > 1 {
		secondMsg = mProvider.msgs[1]
	}
	mProvider.mu.Unlock()

	if msgsSent != 2 {
		testRunner.Fatalf("expected 2 messages sent in total, got %d", msgsSent)
	}
	if secondMsg.Subject != "Normal Priority" {
		testRunner.Fatalf("expected Normal Priority job to be processed second, got %q", secondMsg.Subject)
	}
}

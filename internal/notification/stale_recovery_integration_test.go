package notification

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A job abandoned in 'processing' (its worker died mid-send) is requeued with
// its ciphertext intact, then delivered by a worker.
func TestStaleProcessingJobIsRequeuedAndDelivered(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	ctx := context.Background()
	store := NewStore(pool, testKeyring(t))
	if err := store.CreateJob(ctx, pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}
	// A worker claims the job and dies before finishing.
	if job, err := store.FetchNextJob(ctx); err != nil || job == nil {
		t.Fatalf("claim = %v, %v", job, err)
	}
	task := NewStaleJobRecoveryTask(store, 15*time.Minute, nil)

	// Not stale yet: left alone.
	if err := task.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if job := onlyJob(t, pool); job.state != string(JobProcessing) {
		t.Fatalf("fresh processing job changed to %s", job.state)
	}

	if _, err := pool.Exec(ctx, `UPDATE notification_jobs SET updated_at = now() - interval '20 minutes'`); err != nil {
		t.Fatal(err)
	}
	if err := task.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	job := onlyJob(t, pool)
	if job.state != string(JobPending) || len(job.ciphertext) == 0 || job.attempts != 1 {
		t.Fatalf("recovered job = %+v, want pending with ciphertext, attempts kept at 1", job)
	}
	if job.lastError == nil || !strings.Contains(*job.lastError, "recovered") {
		t.Fatalf("last_error = %v, want a recovery note", job.lastError)
	}

	provider := &mockProvider{name: ChannelEmail}
	if _, err := NewWorker(store, []Provider{provider}, nil, 1).processNextJob(ctx); err != nil {
		t.Fatal(err)
	}
	if len(provider.msgs) != 1 || !strings.Contains(provider.msgs[0].Body, secretToken) {
		t.Fatalf("recovered job not delivered with its decrypted link: %+v", provider.msgs)
	}
	requireNoSecret(t, pool)
}

// A stale job that has used all its attempts is dead-lettered, not retried
// forever (the interrupted attempt counts).
func TestStaleProcessingJobWithoutAttemptsLeftIsDeadLettered(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	ctx := context.Background()
	store := NewStore(pool, testKeyring(t))
	if err := store.CreateJob(ctx, pool, resetTask(orgID, userID), 1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FetchNextJob(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE notification_jobs SET updated_at = now() - interval '20 minutes'`); err != nil {
		t.Fatal(err)
	}

	requeued, deadLettered, err := store.RecoverStaleProcessing(ctx, 15*time.Minute)
	if err != nil || requeued != 0 || deadLettered != 1 {
		t.Fatalf("RecoverStaleProcessing = %d requeued, %d dead-lettered, %v; want 0, 1", requeued, deadLettered, err)
	}
	if job := onlyJob(t, pool); job.state != string(JobDeadLetter) || len(job.ciphertext) == 0 {
		t.Fatalf("job = %+v, want dead_letter with ciphertext kept for recovery", job)
	}
}

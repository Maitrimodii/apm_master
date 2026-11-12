package notification

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/platform/secretbox"
)

// secretToken stands for the one-time token inside a reset link. It must
// never appear in any stored row.
const secretToken = "SECRET-LINK-TOKEN-7f3a"

func randomKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, secretbox.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func keyring(t *testing.T, spec, active string) *secretbox.Keyring {
	t.Helper()
	ring, err := secretbox.ParseKeyring(spec, active)
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func testKeyring(t *testing.T) *secretbox.Keyring {
	t.Helper()
	return keyring(t, "test:"+randomKey(t), "test")
}

func seedOrg(t *testing.T, pool *pgxpool.Pool) (orgID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	orgID, userID = uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO orgs (id, name, slug) VALUES ($1, 'Enc Org', $2)`, orgID, "enc-"+uuid.NewString()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id, org_id, first_name, last_name, email, password_hash)
		VALUES ($1, $2, 'Ada', 'L', $3, 'hash')`, userID, orgID, "ada-"+uuid.NewString()+"@example.com"); err != nil {
		t.Fatal(err)
	}
	return orgID, userID
}

func resetTask(orgID, userID uuid.UUID) Task {
	link := "https://ui.test/reset-password?token=" + secretToken
	return Task{
		OrgID: orgID, UserID: &userID, EventType: EventPasswordResetRequested, Provider: ChannelEmail,
		Recipient: "ada@example.com", Subject: "Reset your password",
		Body:     "Use this link: " + link,
		Metadata: map[string]string{"first_name": "Ada", "reset_link": link},
	}
}

// notificationDump renders every job and event row as text, ciphertext
// excluded, so a test can check a secret appears nowhere.
func notificationDump(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var dump string
	err := pool.QueryRow(context.Background(), `
		SELECT coalesce(string_agg(r::text, E'\n'), '') FROM (
			SELECT to_jsonb(j) - 'payload_ciphertext' AS r FROM notification_jobs j
			UNION ALL SELECT to_jsonb(e) FROM notification_events e
		) rows`).Scan(&dump)
	if err != nil {
		t.Fatal(err)
	}
	return dump
}

func requireNoSecret(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if dump := notificationDump(t, pool); strings.Contains(dump, secretToken) || strings.Contains(dump, "reset_link") {
		t.Fatalf("a stored row contains the one-time link:\n%s", dump)
	}
}

type jobRow struct {
	state      string
	body       string
	attempts   int
	ciphertext []byte
	keyID      *string
	lastError  *string
}

func onlyJob(t *testing.T, pool *pgxpool.Pool) jobRow {
	t.Helper()
	var r jobRow
	err := pool.QueryRow(context.Background(), `
		SELECT state, body, attempts, payload_ciphertext, payload_key_id, last_error FROM notification_jobs`).
		Scan(&r.state, &r.body, &r.attempts, &r.ciphertext, &r.keyID, &r.lastError)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSensitiveJobIsStoredEncrypted(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	store := NewStore(pool, testKeyring(t))

	if err := store.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}

	job := onlyJob(t, pool)
	if job.body != encryptedBody || len(job.ciphertext) == 0 || job.keyID == nil || *job.keyID != "test" {
		t.Fatalf("stored job = %+v, want placeholder body and ciphertext under key test", job)
	}
	requireNoSecret(t, pool)
	// Non-secret metadata stays readable for operators.
	var firstName string
	if err := pool.QueryRow(context.Background(), `SELECT metadata->>'first_name' FROM notification_jobs`).Scan(&firstName); err != nil || firstName != "Ada" {
		t.Fatalf("first_name = %q, %v", firstName, err)
	}
}

func TestWorkerDeliversDecryptedPayloadThenErasesIt(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	store := NewStore(pool, testKeyring(t))
	provider := &mockProvider{name: ChannelEmail}
	worker := NewWorker(store, []Provider{provider}, nil, 1)
	if err := store.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}

	if _, err := worker.processNextJob(context.Background()); err != nil {
		t.Fatal(err)
	}

	if len(provider.msgs) != 1 {
		t.Fatalf("provider got %d messages, want 1", len(provider.msgs))
	}
	msg := provider.msgs[0]
	if !strings.Contains(msg.Body, secretToken) || !strings.Contains(msg.Meta["reset_link"], secretToken) {
		t.Fatalf("delivered message lacks the decrypted link: %+v", msg)
	}
	job := onlyJob(t, pool)
	if job.state != string(JobSent) || job.ciphertext != nil || job.keyID != nil {
		t.Fatalf("after delivery job = %+v, want sent with ciphertext erased", job)
	}
	requireNoSecret(t, pool)
	var eventBody string
	if err := pool.QueryRow(context.Background(), `SELECT body FROM notification_events`).Scan(&eventBody); err != nil || eventBody != redactedBody {
		t.Fatalf("event body = %q, %v; want %q", eventBody, err, redactedBody)
	}
}

func TestRetryKeepsCiphertextAndStoresNoPlaintext(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	store := NewStore(pool, testKeyring(t))
	worker := NewWorker(store, []Provider{&mockProvider{name: ChannelEmail, err: errors.New("smtp unavailable")}}, nil, 1)
	if err := store.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}

	if _, err := worker.processNextJob(context.Background()); err != nil {
		t.Fatal(err)
	}

	job := onlyJob(t, pool)
	if job.state != string(JobPending) || len(job.ciphertext) == 0 {
		t.Fatalf("after a failed attempt job = %+v, want pending with ciphertext kept", job)
	}
	requireNoSecret(t, pool)
}

// A job that cannot be decrypted is dead-lettered at once (no retry, no
// plaintext fallback, nothing sent) and keeps its ciphertext for recovery.
func TestUndecryptableJobsAreDeadLetteredWithoutFallback(t *testing.T) {
	cases := map[string]func(t *testing.T, pool *pgxpool.Pool, writer *Store) *Store{
		"unknown key": func(t *testing.T, _ *pgxpool.Pool, _ *Store) *Store {
			return NewStore(nil, keyring(t, "other:"+randomKey(t), "other"))
		},
		"tampered ciphertext": func(t *testing.T, pool *pgxpool.Pool, writer *Store) *Store {
			if _, err := pool.Exec(context.Background(),
				`UPDATE notification_jobs SET payload_ciphertext = set_byte(payload_ciphertext, 20, get_byte(payload_ciphertext, 20) # 1)`); err != nil {
				t.Fatal(err)
			}
			return writer
		},
		"ciphertext moved to another recipient": func(t *testing.T, pool *pgxpool.Pool, writer *Store) *Store {
			if _, err := pool.Exec(context.Background(), `UPDATE notification_jobs SET recipient = 'attacker@example.com'`); err != nil {
				t.Fatal(err)
			}
			return writer
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			pool := testPool(t)
			orgID, userID := seedOrg(t, pool)
			writer := NewStore(pool, testKeyring(t))
			if err := writer.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
				t.Fatal(err)
			}
			reader := setup(t, pool, writer)
			reader.database = pool
			provider := &mockProvider{name: ChannelEmail}

			if _, err := NewWorker(reader, []Provider{provider}, nil, 1).processNextJob(context.Background()); err != nil {
				t.Fatal(err)
			}

			if len(provider.msgs) != 0 {
				t.Fatalf("an undecryptable job was sent: %+v", provider.msgs)
			}
			job := onlyJob(t, pool)
			if job.state != string(JobDeadLetter) || job.attempts != 1 || len(job.ciphertext) == 0 {
				t.Fatalf("job = %+v, want dead_letter after 1 attempt with ciphertext kept", job)
			}
			requireNoSecret(t, pool)
		})
	}
}

// After rotating to a new active key, jobs sealed with the old key still go out.
func TestKeyRotationStillDeliversOldJobs(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	k1, k2 := randomKey(t), randomKey(t)
	before := NewStore(pool, keyring(t, "k1:"+k1, "k1"))
	if err := before.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}

	after := NewStore(pool, keyring(t, "k1:"+k1+",k2:"+k2, "k2"))
	provider := &mockProvider{name: ChannelEmail}
	if _, err := NewWorker(after, []Provider{provider}, nil, 1).processNextJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(provider.msgs) != 1 || !strings.Contains(provider.msgs[0].Body, secretToken) {
		t.Fatalf("old-key job not delivered after rotation: %+v", provider.msgs)
	}

	// New jobs are sealed with the new key.
	if err := after.CreateJob(context.Background(), pool, resetTask(orgID, userID), 3); err != nil {
		t.Fatal(err)
	}
	var keyID string
	if err := pool.QueryRow(context.Background(), `SELECT payload_key_id FROM notification_jobs WHERE state = 'pending'`).Scan(&keyID); err != nil || keyID != "k2" {
		t.Fatalf("new job key = %q, %v; want k2", keyID, err)
	}
}

func TestDeadLetterPayloadKeptForRecoveryWindowOnly(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	store := NewStore(pool, testKeyring(t))
	if err := store.CreateJob(context.Background(), pool, resetTask(orgID, userID), 1); err != nil {
		t.Fatal(err)
	}
	worker := NewWorker(store, []Provider{&mockProvider{name: ChannelEmail, err: errors.New("rejected")}}, nil, 1)
	if _, err := worker.processNextJob(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job := onlyJob(t, pool); job.state != string(JobDeadLetter) || len(job.ciphertext) == 0 {
		t.Fatalf("job = %+v, want dead_letter with ciphertext", job)
	}

	task := NewPayloadPurgeTask(store, 72*time.Hour, nil)
	if err := task.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job := onlyJob(t, pool); len(job.ciphertext) == 0 {
		t.Fatal("ciphertext purged inside the recovery window")
	}

	if _, err := pool.Exec(context.Background(), `UPDATE notification_jobs SET updated_at = now() - interval '73 hours'`); err != nil {
		t.Fatal(err)
	}
	if err := task.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if job := onlyJob(t, pool); job.ciphertext != nil || job.keyID != nil {
		t.Fatalf("ciphertext kept after the recovery window: %+v", job)
	}
}

func TestSensitiveJobsRefusedWithoutKeysOrEncryption(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	ctx := context.Background()

	if err := NewStore(pool, nil).CreateJob(ctx, pool, resetTask(orgID, userID), 3); !errors.Is(err, errNoKeyring) {
		t.Fatalf("CreateJob without keys = %v, want errNoKeyring", err)
	}
	// A link smuggled into a non-sensitive event type is refused, not stored.
	smuggled := resetTask(orgID, userID)
	smuggled.EventType = EventAlertShared
	if err := NewStore(pool, testKeyring(t)).CreateJob(ctx, pool, smuggled, 3); err == nil {
		t.Fatal("CreateJob stored a link under a non-sensitive event type")
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("%d jobs stored, want 0", jobs)
	}
}

// The database itself rejects plaintext links, whatever code wrote them.
func TestDatabaseRejectsPlaintextLinks(t *testing.T) {
	pool := testPool(t)
	orgID, _ := seedOrg(t, pool)
	ctx := context.Background()
	link := `{"reset_link": "https://ui/reset-password?token=` + secretToken + `"}`

	for name, sql := range map[string]string{
		"job with link metadata": `INSERT INTO notification_jobs (org_id, event_type, provider, recipient, subject, body, metadata)
			VALUES ($1, 'alert_shared', 'email', 'a@x', 's', 'b', '` + link + `')`,
		"plaintext sensitive job": `INSERT INTO notification_jobs (org_id, event_type, provider, recipient, subject, body)
			VALUES ($1, 'password_reset_requested', 'email', 'a@x', 's', 'token=` + secretToken + `')`,
		"queued sensitive job without ciphertext": `INSERT INTO notification_jobs (org_id, event_type, provider, recipient, subject, body)
			VALUES ($1, 'password_reset_requested', 'email', 'a@x', 's', '[encrypted]')`,
		"sensitive event with body": `INSERT INTO notification_events (org_id, event_type, channel, recipient, subject, body)
			VALUES ($1, 'invite_created', 'email', 'a@x', 's', 'token=` + secretToken + `')`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := pool.Exec(ctx, sql, orgID)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("insert = %v, want a check_violation (23514)", err)
			}
		})
	}

	// The store redacts events before writing them.
	if _, err := NewStore(pool, nil).CreateEvent(ctx, Event{
		OrgID: orgID, EventType: EventInviteCreated, Channel: ChannelEmail, Recipient: "a@x", Subject: "s",
		Body: "token=" + secretToken, Metadata: []byte(`{"invite_link":"` + secretToken + `","org_name":"Acme"}`), Status: StatusSent,
	}); err != nil {
		t.Fatalf("CreateEvent: %v", err)
	}
	requireNoSecret(t, pool)
}

// Queuing inside a transaction that rolls back leaves no job.
func TestEnqueueInsideRolledBackTransactionLeavesNoJob(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	ctx := context.Background()
	service := NewService(NewStore(pool, testKeyring(t)), nil, nil)

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnqueueEmailTx(ctx, tx, resetTask(orgID, userID)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var jobs int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM notification_jobs`).Scan(&jobs); err != nil || jobs != 0 {
		t.Fatalf("%d jobs after rollback, want 0", jobs)
	}
}

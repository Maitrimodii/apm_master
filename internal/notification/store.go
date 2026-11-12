package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/platform/secretbox"
)

// Execer runs a statement. Both *pgxpool.Pool and pgx.Tx satisfy it, so a
// caller can queue a job inside its own transaction: the job then exists only
// if that transaction commits.
type Execer interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

var errNoKeyring = errors.New("notification encryption keys are not configured; sensitive emails cannot be queued")

type Store struct {
	database *pgxpool.Pool
	keys     *secretbox.Keyring
}

// NewStore creates the store. keys encrypt the payload of sensitive jobs; a
// store without keys refuses to queue them rather than store plaintext.
func NewStore(database *pgxpool.Pool, keys *secretbox.Keyring) *Store {
	return &Store{database: database, keys: keys}
}

// CreateEvent records a delivery outcome or in-app event. Whatever the
// caller passes, the stored row never carries a one-time link: sensitive
// events get a placeholder body and secret metadata keys are dropped.
func (s *Store) CreateEvent(ctx context.Context, event Event) (*Event, error) {
	metadata := map[string]string{}
	if len(event.Metadata) > 0 {
		if err := json.Unmarshal(event.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("decode event metadata: %w", err)
		}
	}
	encoded, err := json.Marshal(publicMetadata(metadata))
	if err != nil {
		return nil, fmt.Errorf("encode event metadata: %w", err)
	}
	event.Metadata = encoded
	if IsSensitive(event.EventType) {
		event.Body = redactedBody
	}

	query := `
		INSERT INTO notification_events (
			org_id, user_id, event_type, channel, recipient, subject, body, status, error, metadata, sent_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	err = s.database.QueryRow(
		ctx,
		query,
		event.OrgID,
		event.UserID,
		event.EventType,
		event.Channel,
		event.Recipient,
		event.Subject,
		event.Body,
		event.Status,
		event.Error,
		event.Metadata,
		event.SentAt,
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create notification event: %w", err)
	}

	return &event, nil
}

func (s *Store) ExistsByReference(ctx context.Context, referenceID, recipient string, eventType EventType) (bool, error) {
	queryEvents := `
		SELECT id
		FROM notification_events
		WHERE metadata ->> 'reference_id' = $1
			AND recipient = $2
			AND event_type = $3
		LIMIT 1
	`

	var identifier string
	err := s.database.QueryRow(ctx, queryEvents, referenceID, recipient, eventType).Scan(&identifier)
	if err == nil {
		return true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check notification events idempotency: %w", err)
	}

	queryJobs := `
		SELECT id
		FROM notification_jobs
		WHERE reference_id = $1
			AND recipient = $2
			AND event_type = $3
			AND state IN ('pending', 'processing', 'sent')
		LIMIT 1
	`
	err = s.database.QueryRow(ctx, queryJobs, referenceID, recipient, eventType).Scan(&identifier)
	if err == nil {
		return true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check notification jobs idempotency: %w", err)
	}

	return false, nil
}

// CreateJob queues task through exec (the pool, or the caller's transaction).
//
// A sensitive task's body and metadata are sealed with AES-256-GCM into
// payload_ciphertext, bound to the job's ID, org, event type and recipient;
// the plaintext columns get a placeholder body and the metadata without the
// link. A non-sensitive task must not carry secret metadata at all.
func (s *Store) CreateJob(ctx context.Context, exec Execer, task Task, maxAttempts int) error {
	jobID := uuid.New()
	body, metadata := task.Body, task.Metadata
	var (
		ciphertext []byte
		keyID      *string
	)

	if IsSensitive(task.EventType) {
		if s.keys == nil {
			return errNoKeyring
		}
		plaintext, err := encodePayload(task.Body, task.Metadata)
		if err != nil {
			return err
		}
		id, sealed, err := s.keys.Seal(plaintext, payloadAAD(jobID, task.OrgID, task.EventType, task.Recipient))
		if err != nil {
			return fmt.Errorf("encrypt notification payload: %w", err)
		}
		ciphertext, keyID = sealed, &id
		body, metadata = encryptedBody, publicMetadata(task.Metadata)
	} else if hasSecretMetadata(task.Metadata) {
		return fmt.Errorf("event type %q is not marked sensitive but carries a one-time link", task.EventType)
	}

	metadataJSON, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	_, err = exec.Exec(ctx, `
		INSERT INTO notification_jobs (
			id, org_id, user_id, event_type, provider, recipient, subject, body, reference_id, metadata,
			state, attempts, max_attempts, run_at, priority, payload_ciphertext, payload_key_id
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 'pending', 0, $11, now(), $12, $13, $14)`,
		jobID, task.OrgID, task.UserID, task.EventType, task.Provider, task.Recipient, task.Subject, body,
		task.ReferenceID, metadataJSON, maxAttempts, task.Priority, ciphertext, keyID,
	)
	if err != nil {
		return fmt.Errorf("create notification job: %w", err)
	}
	return nil
}

func (s *Store) FetchNextJob(ctx context.Context) (*Job, error) {
	query := `
		UPDATE notification_jobs
		SET state = 'processing',
			attempts = attempts + 1,
			updated_at = now()
		WHERE id = (
			SELECT id
			FROM notification_jobs
			WHERE state = 'pending'
			  AND run_at <= now()
			ORDER BY priority DESC, created_at ASC
			FOR UPDATE SKIP LOCKED
			LIMIT 1
		)
		RETURNING id, org_id, user_id, event_type, provider, recipient, subject, body, COALESCE(reference_id, ''), metadata, state, attempts, max_attempts, run_at, last_error, created_at, updated_at, priority,
			payload_ciphertext, payload_key_id
	`

	var job Job
	var metadataBytes []byte
	err := s.database.QueryRow(ctx, query).Scan(
		&job.ID, &job.OrgID, &job.UserID, &job.EventType, &job.Provider, &job.Recipient, &job.Subject, &job.Body, &job.ReferenceID, &metadataBytes, &job.State, &job.Attempts, &job.MaxAttempts, &job.RunAt, &job.LastError, &job.CreatedAt, &job.UpdatedAt, &job.Priority,
		&job.PayloadCiphertext, &job.PayloadKeyID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if len(metadataBytes) > 0 {
		if err := json.Unmarshal(metadataBytes, &job.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &job, nil
}

// OpenPayload decrypts a sensitive job's body and metadata. It never falls
// back to the plaintext columns: a job without ciphertext, a key that is not
// configured or a failed authentication is an error.
func (s *Store) OpenPayload(job *Job) (sealedPayload, error) {
	if len(job.PayloadCiphertext) == 0 || job.PayloadKeyID == nil {
		return sealedPayload{}, errors.New("sensitive job has no encrypted payload")
	}
	if s.keys == nil {
		return sealedPayload{}, errNoKeyring
	}
	plaintext, err := s.keys.Open(*job.PayloadKeyID, job.PayloadCiphertext,
		payloadAAD(job.ID, job.OrgID, job.EventType, job.Recipient))
	if err != nil {
		return sealedPayload{}, fmt.Errorf("decrypt payload (key %q): %w", *job.PayloadKeyID, err)
	}
	return decodePayload(plaintext)
}

// MarkSent records delivery and erases the encrypted payload in the same
// statement: once sent, nothing needs the link again.
func (s *Store) MarkSent(ctx context.Context, jobID uuid.UUID, attempts int) error {
	_, err := s.database.Exec(ctx, `
		UPDATE notification_jobs
		SET state = 'sent', last_error = NULL, attempts = $2,
			payload_ciphertext = NULL, payload_key_id = NULL, updated_at = now()
		WHERE id = $1`, jobID, attempts)
	return err
}

// UpdateJobState schedules a retry or dead-letters a job. The encrypted
// payload is kept in both cases: a retry needs it, and a dead-lettered job
// keeps it for the recovery window (see PurgeDeadLetterPayloads).
func (s *Store) UpdateJobState(ctx context.Context, jobID uuid.UUID, state JobState, lastErr string, attempts int, runAt *time.Time) error {
	var query string
	var err error
	var lastErrPtr *string
	if lastErr != "" {
		lastErrPtr = &lastErr
	}

	if runAt != nil {
		query = `
			UPDATE notification_jobs
			SET state = $2,
				last_error = $3,
				attempts = $4,
				run_at = $5,
				updated_at = now()
			WHERE id = $1
		`
		_, err = s.database.Exec(ctx, query, jobID, state, lastErrPtr, attempts, *runAt)
	} else {
		query = `
			UPDATE notification_jobs
			SET state = $2,
				last_error = $3,
				attempts = $4,
				updated_at = now()
			WHERE id = $1
		`
		_, err = s.database.Exec(ctx, query, jobID, state, lastErrPtr, attempts)
	}
	return err
}

// PurgeDeadLetterPayloads erases the ciphertext of jobs dead-lettered more
// than recovery ago and reports how many it erased. Until then an operator
// can fix the cause (say, restore a missing key) and re-queue the job.
func (s *Store) PurgeDeadLetterPayloads(ctx context.Context, recovery time.Duration) (int64, error) {
	tag, err := s.database.Exec(ctx, `
		UPDATE notification_jobs
		SET payload_ciphertext = NULL, payload_key_id = NULL
		WHERE state = 'dead_letter'
		  AND payload_ciphertext IS NOT NULL
		  AND updated_at < now() - make_interval(secs => $1)`, recovery.Seconds())
	if err != nil {
		return 0, fmt.Errorf("purge dead-letter payloads: %w", err)
	}
	return tag.RowsAffected(), nil
}

// RecoverStaleProcessing returns jobs left in 'processing' for longer than
// staleAfter (their worker died mid-send) to the queue: 'pending' to run now,
// or 'dead_letter' when they have used all their attempts (the claim already
// counted the interrupted one). Their encrypted payload is untouched. It
// reports how many jobs went each way.
//
// Delivery is therefore at-least-once: a worker that crashed after the SMTP
// server accepted the message but before recording it as sent causes a
// second delivery. staleAfter must comfortably exceed the per-job processing
// timeout so a job still being worked on is never taken.
func (s *Store) RecoverStaleProcessing(ctx context.Context, staleAfter time.Duration) (requeued, deadLettered int64, err error) {
	rows, err := s.database.Query(ctx, `
		WITH stale AS (
			SELECT id FROM notification_jobs
			WHERE state = 'processing' AND updated_at < now() - make_interval(secs => $1)
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_jobs j
		SET state = CASE WHEN j.attempts >= j.max_attempts THEN 'dead_letter' ELSE 'pending' END,
			run_at = now(),
			last_error = 'worker stopped while processing; recovered by notification_stale_job_recovery',
			updated_at = now()
		FROM stale
		WHERE j.id = stale.id
		RETURNING j.state`, staleAfter.Seconds())
	if err != nil {
		return 0, 0, fmt.Errorf("recover stale notification jobs: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			return 0, 0, fmt.Errorf("scan recovered job: %w", err)
		}
		if state == string(JobDeadLetter) {
			deadLettered++
		} else {
			requeued++
		}
	}
	return requeued, deadLettered, rows.Err()
}

func marshalMetadata(metadata map[string]string) ([]byte, error) {
	if metadata == nil {
		return []byte("{}"), nil
	}

	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	return encoded, nil
}

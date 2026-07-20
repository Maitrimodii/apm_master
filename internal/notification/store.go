package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	database *pgxpool.Pool
}

func NewStore(database *pgxpool.Pool) *Store {
	return &Store{database: database}
}

func (store *Store) CreateEvent(ctx context.Context, event Event) (*Event, error) {
	if event.Metadata == nil {
		event.Metadata = []byte("{}")
	}

	query := `
		INSERT INTO notification_events (
			org_id, user_id, event_type, channel, recipient, subject, body, status, error, metadata, sent_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at
	`

	err := store.database.QueryRow(
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

func (store *Store) ExistsByReference(ctx context.Context, referenceID, recipient, eventType string) (bool, error) {
	queryEvents := `
		SELECT id
		FROM notification_events
		WHERE metadata ->> 'reference_id' = $1
			AND recipient = $2
			AND event_type = $3
		LIMIT 1
	`

	var identifier string
	err := store.database.QueryRow(ctx, queryEvents, referenceID, recipient, eventType).Scan(&identifier)
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
	err = store.database.QueryRow(ctx, queryJobs, referenceID, recipient, eventType).Scan(&identifier)
	if err == nil {
		return true, nil
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("check notification jobs idempotency: %w", err)
	}

	return false, nil
}

func (store *Store) CreateJob(ctx context.Context, task NotificationTask, maxAttempts int) error {
	query := `
		INSERT INTO notification_jobs (
			org_id, user_id, event_type, provider, recipient, subject, body, reference_id, metadata, state, attempts, max_attempts, run_at, priority
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'pending', 0, $10, now(), $11)
	`
	metadataJSON, err := json.Marshal(task.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	_, err = store.database.Exec(ctx, query,
		task.OrgID, task.UserID, task.EventType, task.Provider, task.Recipient, task.Subject, task.Body, task.ReferenceID, metadataJSON, maxAttempts, task.Priority,
	)
	return err
}

func (store *Store) FetchNextJob(ctx context.Context) (*NotificationJob, error) {
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
		RETURNING id, org_id, user_id, event_type, provider, recipient, subject, body, COALESCE(reference_id, ''), metadata, state, attempts, max_attempts, run_at, last_error, created_at, updated_at, priority
	`

	var job NotificationJob
	var metadataBytes []byte
	err := store.database.QueryRow(ctx, query).Scan(
		&job.ID, &job.OrgID, &job.UserID, &job.EventType, &job.Provider, &job.Recipient, &job.Subject, &job.Body, &job.ReferenceID, &metadataBytes, &job.State, &job.Attempts, &job.MaxAttempts, &job.RunAt, &job.LastError, &job.CreatedAt, &job.UpdatedAt, &job.Priority,
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

func (store *Store) UpdateJobState(ctx context.Context, jobID uuid.UUID, state string, lastErr string, attempts int, runAt *time.Time) error {
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
		_, err = store.database.Exec(ctx, query, jobID, state, lastErrPtr, attempts, *runAt)
	} else {
		query = `
			UPDATE notification_jobs
			SET state = $2,
				last_error = $3,
				attempts = $4,
				updated_at = now()
			WHERE id = $1
		`
		_, err = store.database.Exec(ctx, query, jobID, state, lastErrPtr, attempts)
	}
	return err
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

package investigation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateWithOutbox writes to investigations and investigation_outbox atomically in a single pgx.Tx.
func (s *Store) CreateWithOutbox(
	ctx context.Context,
	inv *Investigation,
	outboxPayload []byte,
) (*Investigation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if inv.ID == uuid.Nil {
		inv.ID = uuid.New()
	}
	if inv.Status == "" {
		inv.Status = StatusQueued
	}

	query := `
		INSERT INTO investigations (
			id, org_id, incident_id, requested_by, status,
			time_range_start, time_range_end, focus, correlation_id,
			idempotency_key, parent_investigation_id, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, now())
		RETURNING id, created_at
	`
	err = tx.QueryRow(
		ctx, query,
		inv.ID, inv.OrgID, inv.IncidentID, inv.RequestedBy, inv.Status,
		inv.TimeRangeStart, inv.TimeRangeEnd, inv.Focus, inv.CorrelationID,
		inv.IdempotencyKey, inv.ParentInvestigationID,
	).Scan(&inv.ID, &inv.CreatedAt)

	if err != nil {
		// Check for idempotency key duplicate
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && inv.IdempotencyKey != nil {
			// Fetch existing investigation
			existing, fetchErr := s.getByOrgAndIdempotencyKeyTx(ctx, tx, inv.OrgID, *inv.IdempotencyKey)
			if fetchErr == nil && existing != nil {
				return existing, nil
			}
		}
		return nil, fmt.Errorf("insert investigation: %w", err)
	}

	// Insert into outbox
	outboxQuery := `
		INSERT INTO investigation_outbox (
			investigation_id, correlation_id, payload, created_at
		) VALUES ($1, $2, $3, now())
	`
	_, err = tx.Exec(ctx, outboxQuery, inv.ID, inv.CorrelationID, outboxPayload)
	if err != nil {
		return nil, fmt.Errorf("insert outbox: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	return inv, nil
}

func (s *Store) getByOrgAndIdempotencyKeyTx(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, key string) (*Investigation, error) {
	query := `
		SELECT id, org_id, incident_id, requested_by, status,
		       time_range_start, time_range_end, focus, correlation_id,
		       idempotency_key, parent_investigation_id, cancel_requested_at,
		       created_at, started_at, completed_at, failure_code, failure_message
		FROM investigations
		WHERE org_id = $1 AND idempotency_key = $2
	`
	return scanInvestigation(tx.QueryRow(ctx, query, orgID, key))
}

// Get fetches an investigation scoped to an organization.
func (s *Store) Get(ctx context.Context, orgID, investigationID uuid.UUID) (*Investigation, error) {
	query := `
		SELECT id, org_id, incident_id, requested_by, status,
		       time_range_start, time_range_end, focus, correlation_id,
		       idempotency_key, parent_investigation_id, cancel_requested_at,
		       created_at, started_at, completed_at, failure_code, failure_message
		FROM investigations
		WHERE org_id = $1 AND id = $2
	`
	inv, err := scanInvestigation(s.pool.QueryRow(ctx, query, orgID, investigationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get investigation: %w", err)
	}
	return inv, nil
}

// GetByID fetches an investigation by ID regardless of tenant (used by internal endpoints).
func (s *Store) GetByID(ctx context.Context, investigationID uuid.UUID) (*Investigation, error) {
	query := `
		SELECT id, org_id, incident_id, requested_by, status,
		       time_range_start, time_range_end, focus, correlation_id,
		       idempotency_key, parent_investigation_id, cancel_requested_at,
		       created_at, started_at, completed_at, failure_code, failure_message
		FROM investigations
		WHERE id = $1
	`
	inv, err := scanInvestigation(s.pool.QueryRow(ctx, query, investigationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get investigation by id: %w", err)
	}
	return inv, nil
}

// List returns a paginated list of investigations for an org, optionally filtered by incident.
func (s *Store) List(ctx context.Context, orgID, incidentID uuid.UUID, limit, offset int) ([]Investigation, int, error) {
	filter := "WHERE org_id = $1"
	args := []any{orgID}
	argIdx := 2

	if incidentID != uuid.Nil {
		filter += fmt.Sprintf(" AND incident_id = $%d", argIdx)
		args = append(args, incidentID)
		argIdx++
	}

	countQuery := "SELECT count(*) FROM investigations " + filter
	var total int
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count investigations: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, org_id, incident_id, requested_by, status,
		       time_range_start, time_range_end, focus, correlation_id,
		       idempotency_key, parent_investigation_id, cancel_requested_at,
		       created_at, started_at, completed_at, failure_code, failure_message
		FROM investigations
		%s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, filter, argIdx, argIdx+1)

	args = append(args, limit, offset)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list investigations: %w", err)
	}
	defer rows.Close()

	var list []Investigation
	for rows.Next() {
		var inv Investigation
		if err := scanInvestigationRow(rows, &inv); err != nil {
			return nil, 0, fmt.Errorf("scan investigation row: %w", err)
		}
		list = append(list, inv)
	}

	return list, total, rows.Err()
}

// UpdateStatus performs a conditional state transition.
func (s *Store) UpdateStatus(
	ctx context.Context,
	investigationID uuid.UUID,
	newStatus InvestigationStatus,
	prevStatus InvestigationStatus,
	failureCode *string,
	failureMessage *string,
) error {
	query := `
		UPDATE investigations
		SET status = $1,
		    started_at = CASE WHEN $1 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
		    completed_at = CASE WHEN $1 IN ('completed','inconclusive','failed','cancelled','timed_out') AND completed_at IS NULL THEN now() ELSE completed_at END,
		    failure_code = COALESCE($2, failure_code),
		    failure_message = COALESCE($3, failure_message)
		WHERE id = $4 AND status = $5
	`
	cmd, err := s.pool.Exec(ctx, query, newStatus, failureCode, failureMessage, investigationID, prevStatus)
	if err != nil {
		return fmt.Errorf("update status: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrInvalidTransition
	}
	return nil
}

// UpdateStatusDirect sets status without requiring a previous status match (for internal sync).
func (s *Store) UpdateStatusDirect(
	ctx context.Context,
	investigationID uuid.UUID,
	newStatus InvestigationStatus,
	failureCode *string,
	failureMessage *string,
) error {
	query := `
		UPDATE investigations
		SET status = $1,
		    started_at = CASE WHEN $1 = 'running' AND started_at IS NULL THEN now() ELSE started_at END,
		    completed_at = CASE WHEN $1 IN ('completed','inconclusive','failed','cancelled','timed_out') AND completed_at IS NULL THEN now() ELSE completed_at END,
		    failure_code = COALESCE($2, failure_code),
		    failure_message = COALESCE($3, failure_message)
		WHERE id = $4
	`
	_, err := s.pool.Exec(ctx, query, newStatus, failureCode, failureMessage, investigationID)
	if err != nil {
		return fmt.Errorf("update status direct: %w", err)
	}
	return nil
}

// Cancel sets cancel_requested_at and cancels queued investigations.
func (s *Store) Cancel(ctx context.Context, orgID, investigationID uuid.UUID) (*Investigation, error) {
	query := `
		UPDATE investigations
		SET cancel_requested_at = COALESCE(cancel_requested_at, now()),
		    status = CASE WHEN status = 'queued' THEN 'cancelled' ELSE status END,
		    completed_at = CASE WHEN status = 'queued' THEN now() ELSE completed_at END
		WHERE org_id = $1 AND id = $2
		RETURNING id, org_id, incident_id, requested_by, status,
		          time_range_start, time_range_end, focus, correlation_id,
		          idempotency_key, parent_investigation_id, cancel_requested_at,
		          created_at, started_at, completed_at, failure_code, failure_message
	`
	inv, err := scanInvestigation(s.pool.QueryRow(ctx, query, orgID, investigationID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrInvestigationNotFound
		}
		return nil, fmt.Errorf("cancel investigation: %w", err)
	}
	return inv, nil
}

// CountRunningByTenant counts active investigations for tenant concurrency limit.
func (s *Store) CountRunningByTenant(ctx context.Context, orgID uuid.UUID) (int, error) {
	query := `
		SELECT count(*)
		FROM investigations
		WHERE org_id = $1 AND status IN ('queued', 'running')
	`
	var count int
	err := s.pool.QueryRow(ctx, query, orgID).Scan(&count)
	return count, err
}

// Outbox methods

func (s *Store) ListUnpublishedOutbox(ctx context.Context, limit int) ([]OutboxEvent, error) {
	query := `
		SELECT id, investigation_id, correlation_id, payload, created_at, published_at
		FROM investigation_outbox
		WHERE published_at IS NULL
		ORDER BY created_at ASC
		LIMIT $1
	`
	rows, err := s.pool.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list unpublished outbox: %w", err)
	}
	defer rows.Close()

	var events []OutboxEvent
	for rows.Next() {
		var ev OutboxEvent
		if err := rows.Scan(&ev.ID, &ev.InvestigationID, &ev.CorrelationID, &ev.Payload, &ev.CreatedAt, &ev.PublishedAt); err != nil {
			return nil, fmt.Errorf("scan outbox event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

func (s *Store) MarkOutboxPublished(ctx context.Context, eventID uuid.UUID) error {
	query := `UPDATE investigation_outbox SET published_at = now() WHERE id = $1`
	_, err := s.pool.Exec(ctx, query, eventID)
	return err
}

// Evidence methods

func (s *Store) SaveEvidence(ctx context.Context, ev *InvestigationEvidence) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	paramsJSON, _ := json.Marshal(ev.QueryParameters)
	resultJSON, _ := json.Marshal(ev.Result)
	provJSON, _ := json.Marshal(ev.Provenance)

	query := `
		INSERT INTO investigation_evidence (
			id, investigation_id, evidence_type, source_service,
			query_parameters, summary, result, provenance,
			data_quality_warnings, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
	`
	_, err := s.pool.Exec(
		ctx, query,
		ev.ID, ev.InvestigationID, ev.EvidenceType, ev.SourceService,
		paramsJSON, ev.Summary, resultJSON, provJSON,
		ev.DataQualityWarnings,
	)
	return err
}

func (s *Store) GetEvidence(ctx context.Context, orgID, investigationID, evidenceID uuid.UUID) (*InvestigationEvidence, error) {
	query := `
		SELECT e.id, e.investigation_id, e.evidence_type, e.source_service,
		       e.query_parameters, e.summary, e.result, e.provenance,
		       e.data_quality_warnings, e.created_at
		FROM investigation_evidence e
		JOIN investigations i ON e.investigation_id = i.id
		WHERE i.org_id = $1 AND e.investigation_id = $2 AND e.id = $3
	`
	var ev InvestigationEvidence
	var paramsRaw, resultRaw, provRaw []byte
	err := s.pool.QueryRow(ctx, query, orgID, investigationID, evidenceID).Scan(
		&ev.ID, &ev.InvestigationID, &ev.EvidenceType, &ev.SourceService,
		&paramsRaw, &ev.Summary, &resultRaw, &provRaw,
		&ev.DataQualityWarnings, &ev.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get evidence: %w", err)
	}
	_ = json.Unmarshal(paramsRaw, &ev.QueryParameters)
	_ = json.Unmarshal(resultRaw, &ev.Result)
	_ = json.Unmarshal(provRaw, &ev.Provenance)
	return &ev, nil
}

// Report methods

func (s *Store) SaveReport(ctx context.Context, report *InvestigationReportRecord) error {
	if report.ID == uuid.Nil {
		report.ID = uuid.New()
	}
	reportJSON, err := json.Marshal(report.ReportJSON)
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	query := `
		INSERT INTO investigation_reports (
			id, investigation_id, schema_version, report_json, validation_status, created_at
		) VALUES ($1, $2, $3, $4, $5, now())
		ON CONFLICT (investigation_id) DO UPDATE SET
			schema_version = EXCLUDED.schema_version,
			report_json = EXCLUDED.report_json,
			validation_status = EXCLUDED.validation_status
	`
	_, err = s.pool.Exec(ctx, query, report.ID, report.InvestigationID, report.SchemaVersion, reportJSON, report.ValidationStatus)
	return err
}

func (s *Store) GetReport(ctx context.Context, orgID, investigationID uuid.UUID) (*InvestigationReportRecord, error) {
	query := `
		SELECT r.id, r.investigation_id, r.schema_version, r.report_json, r.validation_status, r.created_at
		FROM investigation_reports r
		JOIN investigations i ON r.investigation_id = i.id
		WHERE i.org_id = $1 AND r.investigation_id = $2
	`
	var r InvestigationReportRecord
	var rawJSON []byte
	err := s.pool.QueryRow(ctx, query, orgID, investigationID).Scan(
		&r.ID, &r.InvestigationID, &r.SchemaVersion, &rawJSON, &r.ValidationStatus, &r.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get report: %w", err)
	}
	_ = json.Unmarshal(rawJSON, &r.ReportJSON)
	return &r, nil
}

// Events methods

func (s *Store) AppendEvent(ctx context.Context, ev *InvestigationEvent) error {
	if ev.ID == uuid.Nil {
		ev.ID = uuid.New()
	}
	payloadJSON, _ := json.Marshal(ev.Payload)

	query := `
		INSERT INTO investigation_events (
			id, investigation_id, sequence_number, event_type, summary, payload, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, now())
	`
	_, err := s.pool.Exec(ctx, query, ev.ID, ev.InvestigationID, ev.SequenceNumber, ev.EventType, ev.Summary, payloadJSON)
	return err
}

func (s *Store) ListEvents(ctx context.Context, orgID, investigationID uuid.UUID) ([]InvestigationEvent, error) {
	query := `
		SELECT e.id, e.investigation_id, e.sequence_number, e.event_type, e.summary, e.payload, e.created_at
		FROM investigation_events e
		JOIN investigations i ON e.investigation_id = i.id
		WHERE i.org_id = $1 AND e.investigation_id = $2
		ORDER BY e.sequence_number ASC
	`
	rows, err := s.pool.Query(ctx, query, orgID, investigationID)
	if err != nil {
		return nil, fmt.Errorf("list events: %w", err)
	}
	defer rows.Close()

	var events []InvestigationEvent
	for rows.Next() {
		var ev InvestigationEvent
		var rawPayload []byte
		if err := rows.Scan(&ev.ID, &ev.InvestigationID, &ev.SequenceNumber, &ev.EventType, &ev.Summary, &rawPayload, &ev.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		_ = json.Unmarshal(rawPayload, &ev.Payload)
		events = append(events, ev)
	}
	return events, rows.Err()
}

// Incident context for internal API
func (s *Store) GetIncidentContext(ctx context.Context, incidentID uuid.UUID) (*IncidentContext, error) {
	query := `
		SELECT ai.id, ai.alert_id, a.org_id, a.name, a.severity, a.condition,
		       ai.service_name, ai.triggered_at, ai.resolved_at,
		       ai.value, ai.threshold, ai.message
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE ai.id = $1
	`
	var ic IncidentContext
	err := s.pool.QueryRow(ctx, query, incidentID).Scan(
		&ic.IncidentID, &ic.AlertID, &ic.OrgID, &ic.AlertName, &ic.Severity, &ic.Condition,
		&ic.ServiceName, &ic.TriggeredAt, &ic.ResolvedAt,
		&ic.Value, &ic.Threshold, &ic.Message,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get incident context: %w", err)
	}
	return &ic, nil
}

// Helpers

func scanInvestigation(row pgx.Row) (*Investigation, error) {
	var inv Investigation
	err := row.Scan(
		&inv.ID, &inv.OrgID, &inv.IncidentID, &inv.RequestedBy, &inv.Status,
		&inv.TimeRangeStart, &inv.TimeRangeEnd, &inv.Focus, &inv.CorrelationID,
		&inv.IdempotencyKey, &inv.ParentInvestigationID, &inv.CancelRequestedAt,
		&inv.CreatedAt, &inv.StartedAt, &inv.CompletedAt, &inv.FailureCode, &inv.FailureMessage,
	)
	if err != nil {
		return nil, err
	}
	return &inv, nil
}

func scanInvestigationRow(rows pgx.Rows, inv *Investigation) error {
	return rows.Scan(
		&inv.ID, &inv.OrgID, &inv.IncidentID, &inv.RequestedBy, &inv.Status,
		&inv.TimeRangeStart, &inv.TimeRangeEnd, &inv.Focus, &inv.CorrelationID,
		&inv.IdempotencyKey, &inv.ParentInvestigationID, &inv.CancelRequestedAt,
		&inv.CreatedAt, &inv.StartedAt, &inv.CompletedAt, &inv.FailureCode, &inv.FailureMessage,
	)
}

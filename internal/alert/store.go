package alert

import (
	"context"
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

func (store *Store) Create(ctx context.Context, alert Alert) (*Alert, error) {
	if len(alert.NotificationRoles) == 0 {
		var adminRoleID uuid.UUID
		err := store.database.QueryRow(ctx, "SELECT id FROM roles WHERE org_id = $1 AND name = 'admin'", alert.OrgID).Scan(&adminRoleID)
		if err == nil {
			alert.NotificationRoles = []uuid.UUID{adminRoleID}
		} else {
			alert.NotificationRoles = []uuid.UUID{}
		}
	}
	query := `
		INSERT INTO alerts (org_id, created_by, name, description, severity, condition, is_enabled, notification_roles)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at
	`

	err := store.database.QueryRow(ctx, query, alert.OrgID, alert.CreatedBy, alert.Name, alert.Description, alert.Severity, alert.Condition, alert.IsEnabled, alert.NotificationRoles).
		Scan(&alert.ID, &alert.CreatedAt, &alert.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create alert: %w", err)
	}

	return &alert, nil
}

func (store *Store) Get(ctx context.Context, orgID, alertID uuid.UUID) (*Alert, error) {
	query := `
		SELECT id, org_id, created_by, name, description, severity, condition, is_enabled, notification_roles, created_at, updated_at
		FROM alerts
		WHERE org_id = $1 AND id = $2
	`

	var alert Alert
	err := store.database.QueryRow(ctx, query, orgID, alertID).Scan(
		&alert.ID,
		&alert.OrgID,
		&alert.CreatedBy,
		&alert.Name,
		&alert.Description,
		&alert.Severity,
		&alert.Condition,
		&alert.IsEnabled,
		&alert.NotificationRoles,
		&alert.CreatedAt,
		&alert.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get alert: %w", err)
	}

	return &alert, nil
}

func (store *Store) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Alert, int, error) {
	countQuery := `SELECT count(*) FROM alerts WHERE org_id = $1`
	var total int
	if err := store.database.QueryRow(ctx, countQuery, orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alerts: %w", err)
	}

	query := `
		SELECT id, org_id, created_by, name, description, severity, condition, is_enabled, notification_roles, created_at, updated_at
		FROM alerts
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`

	rows, err := store.database.Query(ctx, query, orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	alerts := make([]Alert, 0)
	for rows.Next() {
		var alert Alert
		if err := rows.Scan(
			&alert.ID,
			&alert.OrgID,
			&alert.CreatedBy,
			&alert.Name,
			&alert.Description,
			&alert.Severity,
			&alert.Condition,
			&alert.IsEnabled,
			&alert.NotificationRoles,
			&alert.CreatedAt,
			&alert.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, alert)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate alerts: %w", err)
	}

	return alerts, total, nil
}

func (store *Store) Update(ctx context.Context, alert Alert) (*Alert, error) {
	if len(alert.NotificationRoles) == 0 {
		var adminRoleID uuid.UUID
		err := store.database.QueryRow(ctx, "SELECT id FROM roles WHERE org_id = $1 AND name = 'admin'", alert.OrgID).Scan(&adminRoleID)
		if err == nil {
			alert.NotificationRoles = []uuid.UUID{adminRoleID}
		} else {
			alert.NotificationRoles = []uuid.UUID{}
		}
	}
	query := `
		UPDATE alerts
		SET name = $1, description = $2, severity = $3, condition = $4, is_enabled = $5, notification_roles = $6, updated_at = now()
		WHERE org_id = $7 AND id = $8
		RETURNING created_at, updated_at, created_by
	`

	err := store.database.QueryRow(ctx, query, alert.Name, alert.Description, alert.Severity, alert.Condition, alert.IsEnabled, alert.NotificationRoles, alert.OrgID, alert.ID).
		Scan(&alert.CreatedAt, &alert.UpdatedAt, &alert.CreatedBy)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update alert: %w", err)
	}

	return &alert, nil
}

func (store *Store) ListEnabledPage(ctx context.Context, limit, offset int) ([]Alert, error) {
	query := `
		SELECT id, org_id, created_by, name, description, severity, condition, is_enabled, notification_roles, created_at, updated_at
		FROM alerts
		WHERE is_enabled = TRUE
		ORDER BY id
		LIMIT $1 OFFSET $2
	`

	rows, err := store.database.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list enabled alerts page: %w", err)
	}
	defer rows.Close()

	alerts := make([]Alert, 0)
	for rows.Next() {
		var alert Alert
		if err := rows.Scan(&alert.ID, &alert.OrgID, &alert.CreatedBy, &alert.Name, &alert.Description, &alert.Severity, &alert.Condition, &alert.IsEnabled, &alert.NotificationRoles, &alert.CreatedAt, &alert.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan enabled alert: %w", err)
		}
		alerts = append(alerts, alert)
	}
	return alerts, rows.Err()
}

type ResolvedUser struct {
	ID    uuid.UUID
	Email string
}

func (store *Store) ResolveUsersByRoles(ctx context.Context, orgID uuid.UUID, roles []uuid.UUID) ([]ResolvedUser, error) {
	query := `
		SELECT DISTINCT u.id, u.email
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		WHERE u.org_id = $1 AND ur.role_id = ANY($2)
	`
	rows, err := store.database.Query(ctx, query, orgID, roles)
	if err != nil {
		return nil, fmt.Errorf("resolve users by roles: %w", err)
	}
	defer rows.Close()

	var users []ResolvedUser
	for rows.Next() {
		var user ResolvedUser
		if err := rows.Scan(&user.ID, &user.Email); err != nil {
			return nil, fmt.Errorf("scan resolved user: %w", err)
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (store *Store) GetActiveInstance(ctx context.Context, alertID uuid.UUID) (*AlertInstance, error) {
	query := `
		SELECT id, alert_id, state, pending_fired_at, triggered_at, acknowledged_at, resolved_at, last_evaluated_at, value, threshold, message, notification_sent, created_at, updated_at
		FROM alert_instances
		WHERE alert_id = $1 AND state IN ('PENDING', 'FIRING','ACKNOWLEDGED')
		ORDER BY triggered_at DESC
		LIMIT 1
	`

	var instance AlertInstance
	err := store.database.QueryRow(ctx, query, alertID).Scan(&instance.ID, &instance.AlertID, &instance.State, &instance.PendingFiredAt, &instance.TriggeredAt, &instance.AcknowledgedAt, &instance.ResolvedAt, &instance.LastEvaluatedAt, &instance.Value, &instance.Threshold, &instance.Message, &instance.NotificationSent, &instance.CreatedAt, &instance.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get active alert instance: %w", err)
	}
	return &instance, nil
}

func (store *Store) SaveInstance(ctx context.Context, instance AlertInstance) error {
	query := `
		INSERT INTO alert_instances (id, alert_id, state, pending_fired_at, triggered_at, acknowledged_at, resolved_at, last_evaluated_at, value, threshold, message, notification_sent, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, now(), now())
		ON CONFLICT (id) DO UPDATE
		SET state = EXCLUDED.state,
		    pending_fired_at = EXCLUDED.pending_fired_at,
		    triggered_at = EXCLUDED.triggered_at,
		    acknowledged_at = EXCLUDED.acknowledged_at,
		    resolved_at = EXCLUDED.resolved_at,
		    last_evaluated_at = EXCLUDED.last_evaluated_at,
		    value = EXCLUDED.value,
		    threshold = EXCLUDED.threshold,
		    message = EXCLUDED.message,
		    notification_sent = EXCLUDED.notification_sent,
		    updated_at = now()
	`

	_, err := store.database.Exec(ctx, query,
		instance.ID, instance.AlertID, instance.State, instance.PendingFiredAt, instance.TriggeredAt, instance.AcknowledgedAt, instance.ResolvedAt, instance.LastEvaluatedAt, instance.Value, instance.Threshold, instance.Message, instance.NotificationSent,
	)
	return err
}

func (store *Store) ListInstances(ctx context.Context, orgID uuid.UUID, state, severity string, alertID uuid.UUID, from, timeout *time.Time, limit, offset int) ([]AlertInstance, int, error) {
	args := []any{orgID}
	filters := ""
	nextArg := 2

	if state != "" {
		filters += fmt.Sprintf(" AND ai.state = $%d", nextArg)
		args = append(args, state)
		nextArg++
	}
	if severity != "" {
		filters += fmt.Sprintf(" AND a.severity = $%d", nextArg)
		args = append(args, severity)
		nextArg++
	}
	if alertID != uuid.Nil {
		filters += fmt.Sprintf(" AND ai.alert_id = $%d", nextArg)
		args = append(args, alertID)
		nextArg++
	}
	if from != nil {
		filters += fmt.Sprintf(" AND ai.triggered_at >= $%d", nextArg)
		args = append(args, *from)
		nextArg++
	}
	if timeout != nil {
		filters += fmt.Sprintf(" AND ai.triggered_at <= $%d", nextArg)
		args = append(args, *timeout)
		nextArg++
	}

	countQuery := `
		SELECT COUNT(*)
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1` + filters

	var total int
	if err := store.database.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alert instances: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, limit, offset)

	query := fmt.Sprintf(`
		SELECT ai.id, ai.alert_id, ai.state, ai.triggered_at, ai.acknowledged_at, ai.resolved_at, ai.last_evaluated_at, ai.value, ai.threshold, ai.message, ai.notification_sent, ai.created_at, ai.updated_at
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1%s
		ORDER BY ai.triggered_at DESC
		LIMIT $%d OFFSET $%d`, filters, limitIdx, offsetIdx)

	rows, err := store.database.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list alert instances: %w", err)
	}
	defer rows.Close()

	var instances []AlertInstance
	for rows.Next() {
		var ai AlertInstance
		err := rows.Scan(
			&ai.ID, &ai.AlertID, &ai.State, &ai.TriggeredAt, &ai.AcknowledgedAt, &ai.ResolvedAt, &ai.LastEvaluatedAt, &ai.Value, &ai.Threshold, &ai.Message, &ai.NotificationSent, &ai.CreatedAt, &ai.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan alert instance: %w", err)
		}
		instances = append(instances, ai)
	}

	return instances, total, rows.Err()
}

func (store *Store) GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error) {
	query := `
		SELECT ai.id, ai.alert_id, a.name, a.description, ai.state, a.severity, ai.triggered_at, ai.acknowledged_at, ai.resolved_at, ai.last_evaluated_at, ai.value, ai.threshold, ai.message, ai.notification_sent, ai.created_at, ai.updated_at
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1 AND ai.id = $2
	`
	var details AlertInstanceDetailsResponse
	err := store.database.QueryRow(ctx, query, orgID, instanceID).Scan(
		&details.ID, &details.AlertID, &details.AlertName, &details.AlertDescription, &details.State, &details.Severity, &details.TriggeredAt, &details.AcknowledgedAt, &details.ResolvedAt, &details.LastEvaluatedAt, &details.Value, &details.Threshold, &details.Message, &details.NotificationSent, &details.CreatedAt, &details.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get alert instance details: %w", err)
	}
	return &details, nil
}

func (store *Store) GetInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstance, error) {
	query := `
		SELECT ai.id, ai.alert_id, ai.state, ai.triggered_at, ai.acknowledged_at, ai.resolved_at, ai.last_evaluated_at, ai.value, ai.threshold, ai.message, ai.notification_sent, ai.created_at, ai.updated_at
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1 AND ai.id = $2
	`
	var ai AlertInstance
	err := store.database.QueryRow(ctx, query, orgID, instanceID).Scan(
		&ai.ID, &ai.AlertID, &ai.State, &ai.TriggeredAt, &ai.AcknowledgedAt, &ai.ResolvedAt, &ai.LastEvaluatedAt, &ai.Value, &ai.Threshold, &ai.Message, &ai.NotificationSent, &ai.CreatedAt, &ai.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get alert instance: %w", err)
	}
	return &ai, nil
}

func (store *Store) Delete(ctx context.Context, orgID, alertID uuid.UUID) error {
	query := `DELETE FROM alerts WHERE org_id = $1 AND id = $2`
	tag, err := store.database.Exec(ctx, query, orgID, alertID)
	if err != nil {
		return fmt.Errorf("delete alert: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (store *Store) AcquireLock(ctx context.Context, lockKey int64) (bool, error) {
	var acquired bool
	err := store.database.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	return acquired, err
}

func (store *Store) ReleaseLock(ctx context.Context, lockKey int64) error {
	_, err := store.database.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
	return err
}

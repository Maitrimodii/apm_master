package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const alertColumns = `id, org_id, created_by, name, description, severity, condition, condition_parsed,
	is_enabled, notification_roles, created_at, updated_at`

const instanceColumns = `ai.id, ai.alert_id, ai.state, ai.pending_fired_at, ai.triggered_at, ai.acknowledged_at,
	ai.resolved_at, ai.last_evaluated_at, ai.value, ai.threshold, ai.message, ai.notification_sent,
	ai.created_at, ai.updated_at`

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, alert Alert) (*Alert, error) {
	if err := s.defaultRoles(ctx, &alert); err != nil {
		return nil, err
	}
	parsed, err := encodeExpression(alert.Expression)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRow(ctx, `
		INSERT INTO alerts (org_id, created_by, name, description, severity, condition, condition_parsed, is_enabled, notification_roles)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		alert.OrgID, alert.CreatedBy, alert.Name, alert.Description, alert.Severity, alert.Condition, parsed, alert.IsEnabled, alert.NotificationRoles,
	).Scan(&alert.ID, &alert.CreatedAt, &alert.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create alert: %w", err)
	}
	return &alert, nil
}

func (s *Store) Get(ctx context.Context, orgID, alertID uuid.UUID) (*Alert, error) {
	row := s.db.QueryRow(ctx, "SELECT "+alertColumns+" FROM alerts WHERE org_id = $1 AND id = $2", orgID, alertID)
	alert, err := scanAlert(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert: %w", err)
	}
	return &alert, nil
}

func (s *Store) List(ctx context.Context, orgID uuid.UUID, limit, offset int) ([]Alert, int, error) {
	var total int
	if err := s.db.QueryRow(ctx, "SELECT count(*) FROM alerts WHERE org_id = $1", orgID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alerts: %w", err)
	}
	rows, err := s.db.Query(ctx,
		"SELECT "+alertColumns+" FROM alerts WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3",
		orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list alerts: %w", err)
	}
	alerts, err := collectAlerts(rows)
	return alerts, total, err
}

func (s *Store) Update(ctx context.Context, alert Alert) (*Alert, error) {
	if err := s.defaultRoles(ctx, &alert); err != nil {
		return nil, err
	}
	parsed, err := encodeExpression(alert.Expression)
	if err != nil {
		return nil, err
	}
	err = s.db.QueryRow(ctx, `
		UPDATE alerts
		SET name = $1, description = $2, severity = $3, condition = $4, condition_parsed = $5,
		    is_enabled = $6, notification_roles = $7, updated_at = now()
		WHERE org_id = $8 AND id = $9
		RETURNING created_at, updated_at, created_by`,
		alert.Name, alert.Description, alert.Severity, alert.Condition, parsed, alert.IsEnabled, alert.NotificationRoles, alert.OrgID, alert.ID,
	).Scan(&alert.CreatedAt, &alert.UpdatedAt, &alert.CreatedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update alert: %w", err)
	}
	return &alert, nil
}

// Delete reports whether an alert was deleted.
func (s *Store) Delete(ctx context.Context, orgID, alertID uuid.UUID) (bool, error) {
	tag, err := s.db.Exec(ctx, "DELETE FROM alerts WHERE org_id = $1 AND id = $2", orgID, alertID)
	if err != nil {
		return false, fmt.Errorf("delete alert: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// ListEnabledPage pages through enabled alerts across all orgs, for evaluation.
func (s *Store) ListEnabledPage(ctx context.Context, limit, offset int) ([]Alert, error) {
	rows, err := s.db.Query(ctx,
		"SELECT "+alertColumns+" FROM alerts WHERE is_enabled = TRUE ORDER BY id LIMIT $1 OFFSET $2",
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list enabled alerts page: %w", err)
	}
	return collectAlerts(rows)
}

// defaultRoles notifies the org's admin role when no roles were chosen.
func (s *Store) defaultRoles(ctx context.Context, alert *Alert) error {
	if len(alert.NotificationRoles) > 0 {
		return nil
	}
	var adminRoleID uuid.UUID
	err := s.db.QueryRow(ctx, "SELECT id FROM roles WHERE org_id = $1 AND name = $2", alert.OrgID, adminRoleName).Scan(&adminRoleID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		alert.NotificationRoles = []uuid.UUID{}
	case err != nil:
		return fmt.Errorf("look up admin role: %w", err)
	default:
		alert.NotificationRoles = []uuid.UUID{adminRoleID}
	}
	return nil
}

func (s *Store) ResolveUsersByRoles(ctx context.Context, orgID uuid.UUID, roles []uuid.UUID) ([]Recipient, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT u.id, u.email
		FROM users u
		JOIN user_roles ur ON u.id = ur.user_id
		WHERE u.org_id = $1 AND ur.role_id = ANY($2)`, orgID, roles)
	if err != nil {
		return nil, fmt.Errorf("resolve users by roles: %w", err)
	}
	defer rows.Close()

	var users []Recipient
	for rows.Next() {
		var u Recipient
		if err := rows.Scan(&u.ID, &u.Email); err != nil {
			return nil, fmt.Errorf("scan recipient: %w", err)
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *Store) GetActiveInstance(ctx context.Context, alertID uuid.UUID) (*AlertInstance, error) {
	row := s.db.QueryRow(ctx, `
		SELECT `+instanceColumns+`
		FROM alert_instances ai
		WHERE ai.alert_id = $1 AND ai.state = ANY($2)
		ORDER BY ai.triggered_at DESC
		LIMIT 1`, alertID, activeStates)
	inst, err := scanInstance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active alert instance: %w", err)
	}
	return &inst, nil
}

func (s *Store) SaveInstance(ctx context.Context, i AlertInstance) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO alert_instances (id, alert_id, state, pending_fired_at, triggered_at, acknowledged_at, resolved_at,
		                             last_evaluated_at, value, threshold, message, notification_sent, created_at, updated_at)
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
		    updated_at = now()`,
		i.ID, i.AlertID, i.State, i.PendingFiredAt, i.TriggeredAt, i.AcknowledgedAt, i.ResolvedAt,
		i.LastEvaluatedAt, i.Value, i.Threshold, i.Message, i.NotificationSent,
	)
	if err != nil {
		return fmt.Errorf("save alert instance: %w", err)
	}
	return nil
}

func (s *Store) ListInstances(ctx context.Context, f InstanceFilter) ([]AlertInstance, int, error) {
	conds := []string{"a.org_id = $1"}
	args := []any{f.OrgID}
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.State != "" {
		add("ai.state = $%d", f.State)
	}
	if f.Severity != "" {
		add("a.severity = $%d", f.Severity)
	}
	if f.AlertID != uuid.Nil {
		add("ai.alert_id = $%d", f.AlertID)
	}
	if f.From != nil {
		add("ai.triggered_at >= $%d", *f.From)
	}
	if f.To != nil {
		add("ai.triggered_at <= $%d", *f.To)
	}
	from := " FROM alert_instances ai JOIN alerts a ON ai.alert_id = a.id WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.db.QueryRow(ctx, "SELECT COUNT(*)"+from, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count alert instances: %w", err)
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.Query(ctx,
		fmt.Sprintf("SELECT %s, a.name, a.severity%s ORDER BY ai.triggered_at DESC LIMIT $%d OFFSET $%d",
			instanceColumns, from, len(args)-1, len(args)),
		args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list alert instances: %w", err)
	}
	defer rows.Close()

	var instances []AlertInstance
	for rows.Next() {
		var inst AlertInstance
		err := rows.Scan(&inst.ID, &inst.AlertID, &inst.State, &inst.PendingFiredAt, &inst.TriggeredAt,
			&inst.AcknowledgedAt, &inst.ResolvedAt, &inst.LastEvaluatedAt, &inst.Value, &inst.Threshold,
			&inst.Message, &inst.NotificationSent, &inst.CreatedAt, &inst.UpdatedAt, &inst.AlertName, &inst.Severity)
		if err != nil {
			return nil, 0, fmt.Errorf("scan alert instance: %w", err)
		}
		instances = append(instances, inst)
	}
	return instances, total, rows.Err()
}

func (s *Store) GetInstanceDetails(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstanceDetailsResponse, error) {
	var d AlertInstanceDetailsResponse
	err := s.db.QueryRow(ctx, `
		SELECT ai.id, ai.alert_id, a.name, a.description, ai.state, a.severity, ai.triggered_at, ai.acknowledged_at,
		       ai.resolved_at, ai.last_evaluated_at, ai.value, ai.threshold, ai.message, ai.notification_sent,
		       ai.created_at, ai.updated_at
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1 AND ai.id = $2`, orgID, instanceID,
	).Scan(
		&d.ID, &d.AlertID, &d.AlertName, &d.AlertDescription, &d.State, &d.Severity, &d.TriggeredAt, &d.AcknowledgedAt,
		&d.ResolvedAt, &d.LastEvaluatedAt, &d.Value, &d.Threshold, &d.Message, &d.NotificationSent,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert instance details: %w", err)
	}
	return &d, nil
}

func (s *Store) GetInstance(ctx context.Context, orgID, instanceID uuid.UUID) (*AlertInstance, error) {
	row := s.db.QueryRow(ctx, `
		SELECT `+instanceColumns+`
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE a.org_id = $1 AND ai.id = $2`, orgID, instanceID)
	inst, err := scanInstance(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get alert instance: %w", err)
	}
	return &inst, nil
}

func scanAlert(row pgx.Row) (Alert, error) {
	var a Alert
	var parsed []byte
	if err := row.Scan(&a.ID, &a.OrgID, &a.CreatedBy, &a.Name, &a.Description, &a.Severity, &a.Condition, &parsed,
		&a.IsEnabled, &a.NotificationRoles, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return Alert{}, err
	}
	if len(parsed) > 0 {
		var expr Expression
		if err := json.Unmarshal(parsed, &expr); err != nil {
			return Alert{}, fmt.Errorf("decode condition_parsed for alert %s: %w", a.ID, err)
		}
		a.Expression = &expr
	}
	return a, nil
}

func collectAlerts(rows pgx.Rows) ([]Alert, error) {
	defer rows.Close()
	alerts := []Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func scanInstance(row pgx.Row) (AlertInstance, error) {
	var i AlertInstance
	err := row.Scan(&i.ID, &i.AlertID, &i.State, &i.PendingFiredAt, &i.TriggeredAt, &i.AcknowledgedAt,
		&i.ResolvedAt, &i.LastEvaluatedAt, &i.Value, &i.Threshold, &i.Message, &i.NotificationSent,
		&i.CreatedAt, &i.UpdatedAt)
	return i, err
}

func encodeExpression(expr *Expression) ([]byte, error) {
	if expr == nil {
		return nil, nil
	}
	b, err := json.Marshal(expr)
	if err != nil {
		return nil, fmt.Errorf("encode condition: %w", err)
	}
	return b, nil
}

// GetIncident loads an alert instance and its definition by instance ID,
// across orgs. Callers must check Alert.OrgID against the caller's tenant.
func (s *Store) GetIncident(ctx context.Context, instanceID uuid.UUID) (*Incident, error) {
	var inc Incident
	var parsed []byte
	a, i := &inc.Alert, &inc.Instance
	err := s.db.QueryRow(ctx, `
		SELECT a.id, a.org_id, a.created_by, a.name, a.description, a.severity, a.condition, a.condition_parsed,
		       a.is_enabled, a.notification_roles, a.created_at, a.updated_at,
		       `+instanceColumns+`, ai.service_name
		FROM alert_instances ai
		JOIN alerts a ON ai.alert_id = a.id
		WHERE ai.id = $1`, instanceID,
	).Scan(
		&a.ID, &a.OrgID, &a.CreatedBy, &a.Name, &a.Description, &a.Severity, &a.Condition, &parsed,
		&a.IsEnabled, &a.NotificationRoles, &a.CreatedAt, &a.UpdatedAt,
		&i.ID, &i.AlertID, &i.State, &i.PendingFiredAt, &i.TriggeredAt, &i.AcknowledgedAt,
		&i.ResolvedAt, &i.LastEvaluatedAt, &i.Value, &i.Threshold, &i.Message, &i.NotificationSent,
		&i.CreatedAt, &i.UpdatedAt, &inc.ServiceName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get incident: %w", err)
	}
	if len(parsed) > 0 {
		var expr Expression
		if err := json.Unmarshal(parsed, &expr); err != nil {
			return nil, fmt.Errorf("decode condition_parsed for alert %s: %w", a.ID, err)
		}
		a.Expression = &expr
	}
	return &inc, nil
}

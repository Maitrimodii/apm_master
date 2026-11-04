package logs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const logColumns = `time, org_id, service_name, trace_id, span_id,
	severity_num, severity_text, body, resource_attrs, log_attrs`

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListLogs(ctx context.Context, filter ListFilter) ([]LogRecord, int, error) {
	conds := []string{"org_id = $1"}
	args := []any{filter.OrgID}
	add := func(cond string, arg any) {
		args = append(args, arg)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if !filter.From.IsZero() {
		add("time >= $%d", filter.From)
	}
	if !filter.To.IsZero() {
		add("time <= $%d", filter.To)
	}
	if filter.ServiceName != "" {
		add("service_name = $%d", filter.ServiceName)
	}
	if filter.MinSeverity > 0 {
		add("severity_num >= $%d", filter.MinSeverity)
	}
	if filter.TraceID != "" {
		add("trace_id = $%d", filter.TraceID)
	}
	if filter.Search != "" {
		add("body ILIKE $%d", "%"+filter.Search+"%")
	}
	where := "WHERE " + strings.Join(conds, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM logs "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count logs: %w", err)
	}

	args = append(args, filter.Limit, filter.Offset)
	query := fmt.Sprintf("SELECT %s FROM logs %s ORDER BY time DESC LIMIT $%d OFFSET $%d",
		logColumns, where, len(args)-1, len(args))
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	records, err := scanLogs(rows)
	return records, total, err
}

func (s *Store) GetTraceLogs(ctx context.Context, orgID uuid.UUID, traceID string) ([]LogRecord, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT "+logColumns+" FROM logs WHERE org_id = $1 AND trace_id = $2 ORDER BY time ASC",
		orgID, traceID)
	if err != nil {
		return nil, fmt.Errorf("get trace logs: %w", err)
	}
	defer rows.Close()
	return scanLogs(rows)
}

func scanLogs(rows pgx.Rows) ([]LogRecord, error) {
	var records []LogRecord
	for rows.Next() {
		var r LogRecord
		var resourceRaw, logAttrsRaw []byte
		if err := rows.Scan(
			&r.Time, &r.OrgID, &r.ServiceName, &r.TraceID, &r.SpanID,
			&r.SeverityNum, &r.SeverityText, &r.Body, &resourceRaw, &logAttrsRaw,
		); err != nil {
			return nil, fmt.Errorf("scan log record: %w", err)
		}
		if err := unmarshalAttrs(resourceRaw, &r.ResourceAttrs); err != nil {
			return nil, fmt.Errorf("decode resource_attrs: %w", err)
		}
		if err := unmarshalAttrs(logAttrsRaw, &r.LogAttrs); err != nil {
			return nil, fmt.Errorf("decode log_attrs: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func unmarshalAttrs(raw []byte, dst *map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

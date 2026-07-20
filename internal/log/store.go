package log

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

type ListLogsQuery struct {
	OrgID       string
	From        *time.Time
	To          *time.Time
	ServiceName string
	MinSeverity int
	TraceID     string
	Search      string
	Limit       int
	Offset      int
}

func (store *Store) ListLogs(ctx context.Context, query ListLogsQuery) ([]LogRecord, int, error) {
	args := []any{query.OrgID}
	filter := ""
	nextArg := 2

	if query.From != nil {
		filter += fmt.Sprintf(" AND time >= $%d", nextArg)
		args = append(args, query.From)
		nextArg++
	}
	if query.To != nil {
		filter += fmt.Sprintf(" AND time <= $%d", nextArg)
		args = append(args, query.To)
		nextArg++
	}
	if query.ServiceName != "" {
		filter += fmt.Sprintf(" AND service_name = $%d", nextArg)
		args = append(args, query.ServiceName)
		nextArg++
	}
	if query.MinSeverity > 0 {
		filter += fmt.Sprintf(" AND severity_num >= $%d", nextArg)
		args = append(args, query.MinSeverity)
		nextArg++
	}
	if query.TraceID != "" {
		filter += fmt.Sprintf(" AND trace_id = $%d", nextArg)
		args = append(args, query.TraceID)
		nextArg++
	}
	if query.Search != "" {
		filter += fmt.Sprintf(" AND body ILIKE $%d", nextArg)
		args = append(args, "%"+query.Search+"%")
		nextArg++
	}

	baseWhere := `WHERE org_id = $1` + filter

	var total int
	err := store.pool.QueryRow(ctx, "SELECT COUNT(*) FROM logs "+baseWhere, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting logs: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, query.Limit, query.Offset)

	rows, err := store.pool.Query(ctx, fmt.Sprintf(`
        SELECT time, org_id,
               service_name, trace_id, span_id,
               severity_num, severity_text,
               body, resource_attrs, log_attrs
        FROM logs
        %s
        ORDER BY time DESC
        LIMIT $%d OFFSET $%d`, baseWhere, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list logs: %w", err)
	}
	defer rows.Close()

	records, err := scanLogs(rows)
	return records, total, err
}

func (store *Store) GetTraceLogs(ctx context.Context, orgID, traceID string) ([]LogRecord, error) {
	rows, err := store.pool.Query(ctx, `
        SELECT time, org_id,
               service_name, trace_id, span_id,
               severity_num, severity_text,
               body, resource_attrs, log_attrs
        FROM logs
        WHERE org_id = $1 AND trace_id = $2
        ORDER BY time ASC`, orgID, traceID)
	if err != nil {
		return nil, fmt.Errorf("get trace logs: %w", err)
	}
	defer rows.Close()
	return scanLogs(rows)
}

func scanLogs(rows pgx.Rows) ([]LogRecord, error) {
	var records []LogRecord
	for rows.Next() {
		var logRecord LogRecord
		var resourceRaw, logAttrsRaw []byte

		err := rows.Scan(
			&logRecord.Time, &logRecord.OrgID, &logRecord.ServiceName, &logRecord.TraceID, &logRecord.SpanID,
			&logRecord.SeverityNum, &logRecord.SeverityText, &logRecord.Body,
			&resourceRaw, &logAttrsRaw, // scan as bytes
		)
		if err != nil {
			return nil, fmt.Errorf("scanning log record: %w", err)
		}

		_ = json.Unmarshal(resourceRaw, &logRecord.ResourceAttrs)
		_ = json.Unmarshal(logAttrsRaw, &logRecord.LogAttrs)

		records = append(records, logRecord)
	}
	return records, rows.Err()
}

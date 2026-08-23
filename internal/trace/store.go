package trace

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ListSpansQuery struct {
	OrgID         uuid.UUID
	From          time.Time
	To            time.Time
	ServiceName   string
	TraceID       string
	Endpoint      string
	StatusCode    string
	MinDurationMS float64
	MaxDurationMS float64
	Limit         int
	Offset        int
}

type Store struct {
	db *pgxpool.Pool
}

func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db}
}

func (s *Store) GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]Span, error) {
	query := `
		SELECT
			span_id, trace_id, parent_id, org_id, service_name, span_name, span_kind,
			start_time, end_time, duration_ns, status_code, status_msg,
			resource_attrs, span_attrs, events, links
		FROM spans
		WHERE org_id = $1 AND trace_id = $2
		ORDER BY start_time ASC
	`
	rows, err := s.db.Query(ctx, query, orgID, traceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query trace: %w", err)
	}
	defer rows.Close()

	var spans []Span
	for rows.Next() {
		var span Span
		err := rows.Scan(
			&span.SpanID, &span.TraceID, &span.ParentID, &span.OrgID, &span.ServiceName, &span.SpanName, &span.SpanKind,
			&span.StartTime, &span.EndTime, &span.DurationNS, &span.StatusCode, &span.StatusMessage,
			&span.ResourceAttrs, &span.SpanAttrs, &span.Events, &span.Links,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan span: %w", err)
		}
		span.DurationMS = float64(span.DurationNS) / 1e6
		spans = append(spans, span)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return spans, nil
}

func (s *Store) ListTraceSummaries(ctx context.Context, query ListSpansQuery) ([]TraceSummary, int, error) {
	baseSQL := `
		SELECT
			trace_id,
			MAX(service_name) as service_name,
			MAX(span_name) as endpoint,
			SUM(duration_ns) / 1000000.0 as duration_ms,
			MAX(status_code) as status,
			COUNT(span_id) as span_count,
			COUNT(CASE WHEN status_code = 'ERROR' THEN 1 END) as error_count,
			0.0 as db_time_ms,
			0.0 as middleware_time_ms
		FROM spans
		WHERE org_id = $1
	`
	
	args := []any{query.OrgID}
	argID := 2
	
	var conds []string
	if !query.From.IsZero() {
		conds = append(conds, fmt.Sprintf("start_time >= $%d", argID))
		args = append(args, query.From)
		argID++
	}
	if !query.To.IsZero() {
		conds = append(conds, fmt.Sprintf("start_time <= $%d", argID))
		args = append(args, query.To)
		argID++
	}
	if query.ServiceName != "" {
		conds = append(conds, fmt.Sprintf("service_name = $%d", argID))
		args = append(args, query.ServiceName)
		argID++
	}
	if query.TraceID != "" {
		conds = append(conds, fmt.Sprintf("trace_id = $%d", argID))
		args = append(args, query.TraceID)
		argID++
	}
	if query.StatusCode != "" {
		conds = append(conds, fmt.Sprintf("status_code = $%d", argID))
		args = append(args, query.StatusCode)
		argID++
	}

	if len(conds) > 0 {
		baseSQL += " AND " + strings.Join(conds, " AND ")
	}

	baseSQL += " GROUP BY trace_id"
	
	var havingConds []string
	if query.MinDurationMS > 0 {
		havingConds = append(havingConds, fmt.Sprintf("SUM(duration_ns) / 1000000.0 >= $%d", argID))
		args = append(args, query.MinDurationMS)
		argID++
	}
	if query.MaxDurationMS > 0 {
		havingConds = append(havingConds, fmt.Sprintf("SUM(duration_ns) / 1000000.0 <= $%d", argID))
		args = append(args, query.MaxDurationMS)
		argID++
	}
	
	if len(havingConds) > 0 {
		baseSQL += " HAVING " + strings.Join(havingConds, " AND ")
	}

	countSQL := "SELECT COUNT(*) FROM (" + baseSQL + ") as sub"
	var total int
	err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count trace summaries: %w", err)
	}

	baseSQL += " ORDER BY MAX(start_time) DESC"
	
	if query.Limit > 0 {
		baseSQL += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, query.Limit)
		argID++
	}
	if query.Offset > 0 {
		baseSQL += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, query.Offset)
		argID++
	}

	rows, err := s.db.Query(ctx, baseSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query trace summaries: %w", err)
	}
	defer rows.Close()

	var summaries []TraceSummary
	for rows.Next() {
		var ts TraceSummary
		var count int64
		var errCount int64
		err := rows.Scan(
			&ts.TraceID, &ts.ServiceName, &ts.Endpoint, &ts.DurationMS, &ts.Status,
			&count, &errCount, &ts.DBTimeMS, &ts.MiddlewareTimeMS,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan trace summary: %w", err)
		}
		ts.SpanCount = int(count)
		ts.ErrorCount = int(errCount)
		summaries = append(summaries, ts)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return summaries, total, nil
}

func (s *Store) ListSpans(ctx context.Context, query ListSpansQuery) ([]Span, int, error) {
	baseSQL := `
		SELECT
			span_id, trace_id, parent_id, org_id, service_name, span_name, span_kind,
			start_time, end_time, duration_ns, status_code, status_msg,
			resource_attrs, span_attrs, events, links
		FROM spans
		WHERE org_id = $1
	`
	
	args := []any{query.OrgID}
	argID := 2
	
	var conds []string
	if !query.From.IsZero() {
		conds = append(conds, fmt.Sprintf("start_time >= $%d", argID))
		args = append(args, query.From)
		argID++
	}
	if !query.To.IsZero() {
		conds = append(conds, fmt.Sprintf("start_time <= $%d", argID))
		args = append(args, query.To)
		argID++
	}
	if query.ServiceName != "" {
		conds = append(conds, fmt.Sprintf("service_name = $%d", argID))
		args = append(args, query.ServiceName)
		argID++
	}
	if query.TraceID != "" {
		conds = append(conds, fmt.Sprintf("trace_id = $%d", argID))
		args = append(args, query.TraceID)
		argID++
	}
	if query.StatusCode != "" {
		conds = append(conds, fmt.Sprintf("status_code = $%d", argID))
		args = append(args, query.StatusCode)
		argID++
	}
	if query.MinDurationMS > 0 {
		conds = append(conds, fmt.Sprintf("duration_ns >= $%d", argID))
		args = append(args, int64(query.MinDurationMS*1e6))
		argID++
	}
	if query.MaxDurationMS > 0 {
		conds = append(conds, fmt.Sprintf("duration_ns <= $%d", argID))
		args = append(args, int64(query.MaxDurationMS*1e6))
		argID++
	}

	if len(conds) > 0 {
		baseSQL += " AND " + strings.Join(conds, " AND ")
	}

	countSQL := "SELECT COUNT(*) FROM (" + baseSQL + ") as sub"
	var total int
	err := s.db.QueryRow(ctx, countSQL, args...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count spans: %w", err)
	}

	baseSQL += " ORDER BY start_time DESC"
	
	if query.Limit > 0 {
		baseSQL += fmt.Sprintf(" LIMIT $%d", argID)
		args = append(args, query.Limit)
		argID++
	}
	if query.Offset > 0 {
		baseSQL += fmt.Sprintf(" OFFSET $%d", argID)
		args = append(args, query.Offset)
		argID++
	}

	rows, err := s.db.Query(ctx, baseSQL, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query spans: %w", err)
	}
	defer rows.Close()

	var spans []Span
	for rows.Next() {
		var span Span
		err := rows.Scan(
			&span.SpanID, &span.TraceID, &span.ParentID, &span.OrgID, &span.ServiceName, &span.SpanName, &span.SpanKind,
			&span.StartTime, &span.EndTime, &span.DurationNS, &span.StatusCode, &span.StatusMessage,
			&span.ResourceAttrs, &span.SpanAttrs, &span.Events, &span.Links,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan span: %w", err)
		}
		span.DurationMS = float64(span.DurationNS) / 1e6
		spans = append(spans, span)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return spans, total, nil
}

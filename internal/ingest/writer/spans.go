package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SpanWriter struct {
	pool *pgxpool.Pool
}

func NewSpanWriter(pool *pgxpool.Pool) *SpanWriter {
	return &SpanWriter{pool: pool}
}

func (writer *SpanWriter) BulkInsert(ctx context.Context, spans []trace.Span) error {
	if len(spans) == 0 {
		return nil
	}

	rows := make([][]any, 0, len(spans))

	for _, span := range spans {
		eventsJSON, err := json.Marshal(span.Events)
		if err != nil {
			return fmt.Errorf("marshal events for span %s: %w", span.SpanID, err)
		}

		linksJSON, err := json.Marshal(span.Links)
		if err != nil {
			return fmt.Errorf("marshal links for span %s: %w", span.SpanID, err)
		}

		resourceJSON, err := json.Marshal(span.ResourceAttrs)
		if err != nil {
			return fmt.Errorf("marshal resource_attrs for span %s: %w", span.SpanID, err)
		}

		spanAttrsJSON, err := json.Marshal(span.SpanAttrs)
		if err != nil {
			return fmt.Errorf("marshal span_attrs for span %s: %w", span.SpanID, err)
		}

		rows = append(rows, []any{
			span.SpanID,
			span.TraceID,
			span.ParentID,
			span.OrgID,
			span.ServiceName,
			span.SpanName,
			span.SpanKind,
			span.StartTime,
			span.EndTime,
			span.DurationNS, // stored as nanoseconds — matches DB column duration_ns
			span.StatusCode,
			span.StatusMessage,
			resourceJSON,
			spanAttrsJSON,
			eventsJSON,
			linksJSON,
		})
	}

	cols := []string{
		"span_id", "trace_id", "parent_id", "org_id",
		"service_name", "span_name", "span_kind",
		"start_time", "end_time", "duration_ns",
		"status_code", "status_msg",
		"resource_attrs", "span_attrs",
		"events", "links",
	}

	start := time.Now()
	_, err := writer.pool.CopyFrom(
		ctx,
		pgx.Identifier{"spans"},
		cols,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("bulk insert %d spans: %w", len(spans), err)
	}

	slog.DebugContext(ctx, "bulk spans inserted successfully", "count", len(spans), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

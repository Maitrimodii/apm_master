package writer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type LogWriter struct {
	pool *pgxpool.Pool
}

func NewLogWriter(pool *pgxpool.Pool) *LogWriter {
	return &LogWriter{pool: pool}
}

func (writer *LogWriter) BulkInsert(ctx context.Context, records []log.LogRecord) error {
	if len(records) == 0 {
		return nil
	}

	rows := make([][]any, 0, len(records))

	for _, logRecord := range records {
		resourceJSON, err := json.Marshal(logRecord.ResourceAttrs)
		if err != nil {
			return fmt.Errorf("marshal resource_attrs for log at %s: %w", logRecord.Time, err)
		}

		logAttrsJSON, err := json.Marshal(logRecord.LogAttrs)
		if err != nil {
			return fmt.Errorf("marshal log_attrs for log at %s: %w", logRecord.Time, err)
		}

		rows = append(rows, []any{
			logRecord.Time,
			logRecord.OrgID,
			logRecord.ServiceName,
			logRecord.TraceID,
			logRecord.SpanID,
			logRecord.SeverityNum,
			logRecord.SeverityText,
			logRecord.Body,
			resourceJSON,
			logAttrsJSON,
		})
	}

	cols := []string{
		"time", "org_id", "service_name", "trace_id", "span_id",
		"severity_num", "severity_text", "body", "resource_attrs", "log_attrs",
	}

	start := time.Now()
	_, err := writer.pool.CopyFrom(ctx, pgx.Identifier{"logs"}, cols, pgx.CopyFromRows(rows))

	if err != nil {
		return fmt.Errorf("bulk insert logs: %w", err)
	}

	slog.DebugContext(ctx, "bulk logs inserted successfully", "count", len(records), "duration_ms", time.Since(start).Milliseconds())
	return nil

}

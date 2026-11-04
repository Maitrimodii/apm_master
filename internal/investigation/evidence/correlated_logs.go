package evidence

import (
	"context"
	"fmt"
	"time"

	"github.com/Maitrimodii/apm_master/internal/logs"
)

const (
	defaultCorrelatedLogs = 50
	maxCorrelatedLogs     = 100
)

type logLister interface {
	ListLogs(ctx context.Context, filter logs.ListFilter) ([]logs.LogRecord, int, error)
}

// correlatedLogs searches logs in the incident window.
type correlatedLogs struct{ logs logLister }

func (e correlatedLogs) Execute(ctx context.Context, q Query) (Result, error) {
	var warnings []string
	svc := paramString(q.Parameters, "service_name")
	records, total, err := e.logs.ListLogs(ctx, logs.ListFilter{
		OrgID:       q.TenantID,
		From:        q.From,
		To:          q.To,
		ServiceName: svc,
		MinSeverity: paramInt(q.Parameters, "min_severity", 0),
		TraceID:     paramString(q.Parameters, "trace_id"),
		Search:      paramString(q.Parameters, "search"),
		Limit:       capLimit(q.Parameters, defaultCorrelatedLogs, maxCorrelatedLogs, &warnings),
	})
	if err != nil {
		return Result{}, fmt.Errorf("list logs: %w", err)
	}
	if total == 0 {
		warnings = append(warnings, "No correlated log records found matching query")
	}
	list := make([]map[string]any, 0, len(records))
	for _, l := range records {
		list = append(list, map[string]any{
			"time": l.Time.Format(time.RFC3339Nano), "service_name": l.ServiceName,
			"trace_id": l.TraceID, "span_id": l.SpanID, "severity_num": l.SeverityNum,
			"severity_text": l.SeverityText, "body": l.Body, "log_attrs": l.LogAttrs,
		})
	}
	return Result{
		Data:        list,
		Warnings:    warnings,
		ServiceName: svc,
		Summary:     fmt.Sprintf("Retrieved %d correlated log records (total %d matching)", len(list), total),
	}, nil
}

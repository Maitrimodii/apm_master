package evidence

import (
	"context"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/traces"
)

const (
	defaultSlowTraces = 10
	maxSlowTraces     = 50
)

type traceSummaryLister interface {
	ListTraceSummaries(ctx context.Context, filter traces.ListFilter) ([]traces.TraceSummary, int, error)
}

// slowTraces lists the slowest traces in the incident window.
type slowTraces struct{ traces traceSummaryLister }

func (e slowTraces) Execute(ctx context.Context, q Query) (Result, error) {
	var warnings []string
	svc := paramString(q.Parameters, "service_name")
	summaries, total, err := e.traces.ListTraceSummaries(ctx, traces.ListFilter{
		OrgID:         q.TenantID,
		From:          q.From,
		To:            q.To,
		ServiceName:   svc,
		MinDurationMS: paramFloat(q.Parameters, "min_duration_ms"),
		Limit:         capLimit(q.Parameters, defaultSlowTraces, maxSlowTraces, &warnings),
	})
	if err != nil {
		return Result{}, fmt.Errorf("list trace summaries: %w", err)
	}
	if total == 0 {
		warnings = append(warnings, "No slow traces found matching criteria in incident window")
	}
	list := make([]map[string]any, 0, len(summaries))
	for _, s := range summaries {
		list = append(list, map[string]any{
			"trace_id": s.TraceID, "service_name": s.ServiceName, "endpoint": s.Endpoint,
			"duration_ms": s.DurationMS, "status": s.Status, "span_count": s.SpanCount,
			"error_count": s.ErrorCount, "db_time_ms": s.DBTimeMS, "middleware_time_ms": s.MiddlewareTimeMS,
		})
	}
	return Result{
		Data:        list,
		Warnings:    warnings,
		ServiceName: svc,
		Summary:     fmt.Sprintf("Retrieved %d slow traces (total %d matching)", len(list), total),
	}, nil
}

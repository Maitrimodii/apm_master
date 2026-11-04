package evidence

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/traces"
)

type traceGetter interface {
	GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]traces.Span, error)
}

// traceDetails returns every span of one trace.
type traceDetails struct{ traces traceGetter }

func (e traceDetails) Execute(ctx context.Context, q Query) (Result, error) {
	traceID := paramString(q.Parameters, "trace_id")
	if traceID == "" {
		return Result{}, invalid("trace_id is required for %s", TraceDetails)
	}
	spans, err := e.traces.GetTrace(ctx, q.TenantID, traceID)
	if err != nil {
		return Result{}, fmt.Errorf("get trace spans: %w", err)
	}

	var warnings []string
	if len(spans) == 0 {
		warnings = append(warnings, fmt.Sprintf("No spans found for trace ID %s", traceID))
	}
	var svc string
	list := make([]map[string]any, 0, len(spans))
	for _, sp := range spans {
		if svc == "" {
			svc = sp.ServiceName
		}
		list = append(list, map[string]any{
			"span_id": sp.SpanID, "trace_id": sp.TraceID, "parent_id": sp.ParentID,
			"service_name": sp.ServiceName, "span_name": sp.SpanName, "span_kind": sp.SpanKind,
			"start_time": sp.StartTime.Format(time.RFC3339Nano), "end_time": sp.EndTime.Format(time.RFC3339Nano),
			"duration_ms": sp.DurationMS, "status_code": sp.StatusCode, "status_message": sp.StatusMessage,
			"span_attrs": sp.SpanAttrs,
		})
	}
	return Result{
		Data:        list,
		Warnings:    warnings,
		ServiceName: svc,
		Summary:     fmt.Sprintf("Retrieved trace %s with %d spans", traceID, len(list)),
	}, nil
}

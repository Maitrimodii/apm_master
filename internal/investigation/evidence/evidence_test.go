package evidence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

type fakeMetrics struct {
	byFrom map[time.Time]metrics.MetricAggregation
}

func (f fakeMetrics) GetAggregations(_ context.Context, q metrics.AggregationQuery) (metrics.MetricAggregation, error) {
	return f.byFrom[q.From], nil
}

type fakeTraces struct{ lastFilter traces.ListFilter }

func (f *fakeTraces) ListTraceSummaries(_ context.Context, filter traces.ListFilter) ([]traces.TraceSummary, int, error) {
	f.lastFilter = filter
	return []traces.TraceSummary{{TraceID: "t1", DurationMS: 900}}, 1, nil
}
func (f *fakeTraces) GetTrace(context.Context, uuid.UUID, string) ([]traces.Span, error) {
	return nil, nil
}

type fakeLogs struct{}

func (fakeLogs) ListLogs(context.Context, logs.ListFilter) ([]logs.LogRecord, int, error) {
	return nil, 0, nil
}

func TestRegistryRejectsUnknownQueryType(t *testing.T) {
	_, err := NewRegistry(Readers{}).Execute(context.Background(), "bogus", Query{})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("Execute(bogus) error = %v, want ErrInvalidQuery", err)
	}
}

func TestMetricBaselineComparesWithPrecedingWindow(t *testing.T) {
	from := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	to := from.Add(30 * time.Minute)
	reg := NewRegistry(Readers{Metrics: fakeMetrics{byFrom: map[time.Time]metrics.MetricAggregation{
		from:                        {Count: 10, Avg: 150},
		from.Add(-30 * time.Minute): {Count: 10, Avg: 100},
	}}})

	res, err := reg.Execute(context.Background(), MetricBaseline, Query{From: from, To: to, Parameters: map[string]any{"metric_name": "latency"}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data := res.Data.(map[string]any)
	if got := data["relative_change_pct"]; got != 50.0 {
		t.Fatalf("relative_change_pct = %v, want 50", got)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", res.Warnings)
	}
}

func TestMetricBaselineRequiresMetricName(t *testing.T) {
	_, err := NewRegistry(Readers{Metrics: fakeMetrics{}}).Execute(context.Background(), MetricBaseline, Query{Parameters: map[string]any{}})
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("error = %v, want ErrInvalidQuery", err)
	}
}

func TestSlowTracesCapsLimit(t *testing.T) {
	tr := &fakeTraces{}
	res, err := NewRegistry(Readers{Traces: tr, Logs: fakeLogs{}}).Execute(context.Background(), SlowTraces,
		Query{Parameters: map[string]any{"limit": float64(500), "min_duration_ms": float64(200)}})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if tr.lastFilter.Limit != maxSlowTraces || tr.lastFilter.MinDurationMS != 200 {
		t.Fatalf("filter = %+v, want limit %d and min duration 200", tr.lastFilter, maxSlowTraces)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one cap warning", res.Warnings)
	}
}

func TestRelativeChangePct(t *testing.T) {
	tests := []struct{ baseline, current, want float64 }{
		{100, 150, 50}, {100, 50, -50}, {0, 10, 100}, {0, 0, 0},
	}
	for _, tc := range tests {
		if got := relativeChangePct(tc.baseline, tc.current); got != tc.want {
			t.Errorf("relativeChangePct(%v, %v) = %v, want %v", tc.baseline, tc.current, got, tc.want)
		}
	}
}

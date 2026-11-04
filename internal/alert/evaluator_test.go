package alert

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

type fakeMetricReader struct {
	points   []metrics.MetricPoint
	err      error
	aggValue *metrics.MetricAggregation
}

func (f fakeMetricReader) GetSeries(ctx context.Context, q metrics.SeriesQuery) ([]metrics.MetricPoint, metrics.Resolution, error) {
	if f.err != nil {
		return nil, metrics.ResolutionRaw, f.err
	}
	return f.points, metrics.ResolutionRaw, nil
}

func (f fakeMetricReader) GetAggregations(ctx context.Context, q metrics.AggregationQuery) (metrics.MetricAggregation, error) {
	if f.err != nil {
		return metrics.MetricAggregation{}, f.err
	}
	if f.aggValue != nil {
		return *f.aggValue, nil
	}

	if len(f.points) == 0 {
		return metrics.MetricAggregation{}, nil
	}

	var total float64
	var count int64
	min := f.points[0].Min
	max := f.points[0].Max

	for _, p := range f.points {
		sampleCount := p.Count
		if sampleCount == 0 {
			sampleCount = 1
		}
		total += p.Avg * float64(sampleCount)
		count += sampleCount

		if p.Min < min {
			min = p.Min
		}
		if p.Max > max {
			max = p.Max
		}
	}

	avg := 0.0
	if count > 0 {
		avg = total / float64(count)
	}

	return metrics.MetricAggregation{
		MetricName: q.MetricName,
		Count:      count,
		Total:      total,
		Avg:        avg,
		Min:        min,
		Max:        max,
		P50:        avg,
		P95:        avg,
		P99:        avg,
	}, nil
}

type fakeLogReader struct {
	records []logs.LogRecord
	err     error
}

func (f fakeLogReader) ListLogs(ctx context.Context, q logs.ListFilter) ([]logs.LogRecord, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.records, len(f.records), nil
}

type fakeTraceReader struct {
	spans     []traces.Span
	summaries []traces.TraceSummary
	err       error
}

func (f fakeTraceReader) ListSpans(ctx context.Context, q traces.ListFilter) ([]traces.Span, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.spans, len(f.spans), nil
}

func (f fakeTraceReader) ListTraceSummaries(ctx context.Context, q traces.ListFilter) ([]traces.TraceSummary, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.summaries, len(f.summaries), nil
}

func TestMetricEvaluatorEvaluateFiresWhenThresholdExceeded(testRunner *testing.T) {
	evaluator := &MetricEvaluator{
		reader: fakeMetricReader{
			points: []metrics.MetricPoint{
				{Avg: 92.5, Count: 2},
			},
		},
	}

	alert := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Name:      "high cpu",
		Severity:  "high",
		Condition: "avg(cpu_usage) > 80 for 5m",
		IsEnabled: true,
	}

	result, err := evaluator.Evaluate(context.Background(), alert)
	if err != nil {
		testRunner.Fatalf("Evaluate returned error: %v", err)
	}
	if !result.Fired {
		testRunner.Fatal("expected alert to fire when average exceeds threshold")
	}
	if result.Value != 92.5 {
		testRunner.Fatalf("expected value 92.5, got %v", result.Value)
	}
	if result.Threshold != 80 {
		testRunner.Fatalf("expected threshold 80, got %v", result.Threshold)
	}
}

func TestMetricEvaluatorPercentilesAndWeightedAverage(testRunner *testing.T) {
	evaluatorP95 := &MetricEvaluator{
		reader: fakeMetricReader{
			aggValue: &metrics.MetricAggregation{
				Count: 10,
				Avg:   75.0,
				P95:   95.5,
				P99:   98.0,
			},
		},
	}

	alertP95 := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Name:      "p95 latency",
		Severity:  "high",
		Condition: "p95(http.latency) > 90 for 5m",
		IsEnabled: true,
	}

	result, err := evaluatorP95.Evaluate(context.Background(), alertP95)
	if err != nil {
		testRunner.Fatalf("Evaluate returned error: %v", err)
	}
	if !result.Fired {
		testRunner.Fatal("expected P95 alert to fire")
	}
	if result.Value != 95.5 {
		testRunner.Fatalf("expected P95 value 95.5, got %v", result.Value)
	}

	evaluatorWeighted := &MetricEvaluator{
		reader: fakeMetricReader{
			points: []metrics.MetricPoint{
				{Avg: 10.0, Count: 1},
				{Avg: 100.0, Count: 9},
			},
		},
	}

	alertAvg := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Name:      "weighted avg check",
		Severity:  "high",
		Condition: "avg(cpu_usage) > 90 for 5m",
		IsEnabled: true,
	}

	resultAvg, err := evaluatorWeighted.Evaluate(context.Background(), alertAvg)
	if err != nil {
		testRunner.Fatalf("Evaluate returned error: %v", err)
	}
	if !resultAvg.Fired {
		testRunner.Fatal("expected weighted average alert to fire because true average is 91.0")
	}
	if resultAvg.Value != 91.0 {
		testRunner.Fatalf("expected weighted average value 91.0, got %v", resultAvg.Value)
	}
}

func TestParseCondition(t *testing.T) {
	c, err := parseCondition("avg(cpu_usage) > 80 for 5m")
	if err != nil {
		t.Fatalf("parseCondition() error = %v", err)
	}
	if c.Source != SourceMetric || c.Target != "cpu_usage" || c.Aggregation != "avg" || c.Threshold != 80 || c.Window() != 5*time.Minute {
		t.Fatalf("parseCondition() = %+v", c)
	}
}

func TestParseConditionRejectsBadInput(t *testing.T) {
	for _, text := range []string{
		"avg(cpu_usage) > 80 for 5x", // B07: used to become a 0 window and fire instantly
		"avg(cpu_usage) > 80 for 0s",
		"rate(http_5xx) > 1",        // unknown aggregation
		"error_rate(logs) > 1",      // aggregation not valid for logs
		"kafka_consumer_lag > 5000", // missing agg(...)
		"",
	} {
		if _, err := ParseExpression(text); err == nil {
			t.Errorf("ParseExpression(%q) error = nil, want error", text)
		}
	}
}

func TestSourceRouting(t *testing.T) {
	tests := map[string]Source{
		"p95(http_request_duration_ms) > 500":         SourceMetric, // was routed to traces before
		"count(logs{severity=\"ERROR\"}) > 10":        SourceLog,
		"avg(spans.duration) > 450ms":                 SourceTrace,
		"error_rate(spans{service_name=\"web\"}) > 5": SourceTrace,
	}
	for text, want := range tests {
		c, err := parseCondition(text)
		if err != nil {
			t.Fatalf("parseCondition(%q) error = %v", text, err)
		}
		if c.Source != want {
			t.Errorf("parseCondition(%q).Source = %s, want %s", text, c.Source, want)
		}
	}
}

func TestParseExpressionCompound(t *testing.T) {
	expr, err := ParseExpression("avg(cpu) > 80 for 2m AND max(mem) > 90 OR count(logs{severity=\"ERROR\"}) > 10 for 10m")
	if err != nil {
		t.Fatalf("ParseExpression() error = %v", err)
	}
	if len(expr.AnyOf) != 2 || len(expr.AnyOf[0]) != 2 || len(expr.AnyOf[1]) != 1 {
		t.Fatalf("groups = %+v, want [[cpu, mem], [logs]]", expr.AnyOf)
	}
	if expr.Window() != 10*time.Minute {
		t.Fatalf("Window() = %v, want the longest condition window 10m", expr.Window())
	}
}

func TestExpressionEvaluatorSingleConditionKeepsValue(t *testing.T) {
	reader := fakeMetricReader{aggValue: &metrics.MetricAggregation{Count: 3, Avg: 92.5}}
	eval := NewExpressionEvaluator(NewMetricEvaluator(reader, nil), NewLogEvaluator(fakeLogReader{}, nil), NewTraceEvaluator(fakeTraceReader{}, nil))
	res, err := eval.Evaluate(context.Background(), Alert{Condition: "avg(cpu_usage) > 80"})
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}
	if !res.Fired || res.Value != 92.5 || res.Threshold != 80 {
		t.Fatalf("Evaluate() = %+v, want fired with value 92.5 and threshold 80", res)
	}
}

func TestLogEvaluatorEvaluatesCorrectly(testRunner *testing.T) {
	evaluator := &LogEvaluator{
		reader: fakeLogReader{
			records: make([]logs.LogRecord, 12),
		},
	}

	alert := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Name:      "high error rate logs",
		Severity:  "critical",
		Condition: "count(logs{service_name=\"checkout\",severity=\"ERROR\"}) > 10 for 5m",
		IsEnabled: true,
	}

	result, err := evaluator.Evaluate(context.Background(), alert)
	if err != nil {
		testRunner.Fatalf("Log evaluate failed: %v", err)
	}
	if !result.Fired {
		testRunner.Fatal("expected log alert to fire since count 12 > 10")
	}
	if result.Value != 12 {
		testRunner.Fatalf("expected logs count 12, got %v", result.Value)
	}
}

func TestTraceEvaluatorEvaluatesDurationAndErrorRate(testRunner *testing.T) {
	evaluator := &TraceEvaluator{
		reader: fakeTraceReader{
			summaries: []traces.TraceSummary{
				{DurationMS: 600, Status: "ERROR", SpanCount: 5},
				{DurationMS: 400, Status: "OK", SpanCount: 4},
			},
		},
	}

	// 1. Test avg duration
	alertAvg := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Condition: "avg(spans.duration) > 450ms for 5m",
	}
	resAvg, err := evaluator.Evaluate(context.Background(), alertAvg)
	if err != nil {
		testRunner.Fatalf("Trace duration evaluate failed: %v", err)
	}
	if !resAvg.Fired {
		testRunner.Fatal("expected avg duration to fire since 500ms > 450ms")
	}
	if resAvg.Value != 500 {
		testRunner.Fatalf("expected avg duration 500, got %v", resAvg.Value)
	}

	// 2. Test error rate
	alertErr := Alert{
		ID:        uuid.New(),
		OrgID:     uuid.New(),
		Condition: "error_rate(spans) > 40% for 5m",
	}
	resErr, err := evaluator.Evaluate(context.Background(), alertErr)
	if err != nil {
		testRunner.Fatalf("Trace error rate evaluate failed: %v", err)
	}
	if !resErr.Fired {
		testRunner.Fatal("expected error rate to fire since 50% > 40%")
	}
	if resErr.Value != 50 {
		testRunner.Fatalf("expected error rate 50%%, got %v", resErr.Value)
	}
}

func TestCompoundEvaluatorBooleanLogic(testRunner *testing.T) {
	// Test condition parsing split
	partsOr := splitLogicalOperator("avg(cpu) > 80 OR max(mem) > 90", "OR")
	if len(partsOr) != 2 || partsOr[0] != "avg(cpu) > 80" || partsOr[1] != "max(mem) > 90" {
		testRunner.Fatalf("OR split failed: %v", partsOr)
	}

	partsAnd := splitLogicalOperator("avg(cpu) > 80 AND max(mem) > 90", "AND")
	if len(partsAnd) != 2 || partsAnd[0] != "avg(cpu) > 80" || partsAnd[1] != "max(mem) > 90" {
		testRunner.Fatalf("AND split failed: %v", partsAnd)
	}

	// Test splitting with spaces and operators inside braces and quotes
	partsBracesQuotes := splitLogicalOperator("avg(logs{service_name=\"billing and shipping\", level=\"ERROR\"}) > 10 OR max(mem) > 90", "OR")
	if len(partsBracesQuotes) != 2 || partsBracesQuotes[0] != "avg(logs{service_name=\"billing and shipping\", level=\"ERROR\"}) > 10" || partsBracesQuotes[1] != "max(mem) > 90" {
		testRunner.Fatalf("OR split with braces/quotes failed: %v", partsBracesQuotes)
	}

	partsBracesAnd := splitLogicalOperator("avg(logs{service_name=\"checkout and payments\"}) > 10 AND avg(cpu_usage) > 80", "AND")
	if len(partsBracesAnd) != 2 || partsBracesAnd[0] != "avg(logs{service_name=\"checkout and payments\"}) > 10" || partsBracesAnd[1] != "avg(cpu_usage) > 80" {
		testRunner.Fatalf("AND split with braces/quotes failed: %v", partsBracesAnd)
	}
}

package alert

import (
	"context"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
)

type fakeMetricReader struct {
	points   []metric.MetricPoint
	err      error
	aggValue *metric.MetricAggregation
}

func (f fakeMetricReader) GetSeries(ctx context.Context, q metric.SeriesQuery) ([]metric.MetricPoint, metric.Resolution, error) {
	if f.err != nil {
		return nil, metric.ResolutionRaw, f.err
	}
	return f.points, metric.ResolutionRaw, nil
}

func (f fakeMetricReader) GetAggregations(ctx context.Context, q metric.AggregationQuery) (metric.MetricAggregation, error) {
	if f.err != nil {
		return metric.MetricAggregation{}, f.err
	}
	if f.aggValue != nil {
		return *f.aggValue, nil
	}

	if len(f.points) == 0 {
		return metric.MetricAggregation{}, nil
	}

	var total float64
	var count int64
	min := f.points[0].MinValue
	max := f.points[0].MaxValue

	for _, p := range f.points {
		sampleCount := p.SampleCount
		if sampleCount == 0 {
			sampleCount = 1
		}
		total += p.AvgValue * float64(sampleCount)
		count += sampleCount

		if p.MinValue < min {
			min = p.MinValue
		}
		if p.MaxValue > max {
			max = p.MaxValue
		}
	}

	avg := 0.0
	if count > 0 {
		avg = total / float64(count)
	}

	return metric.MetricAggregation{
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
	records []log.LogRecord
	err     error
}

func (f fakeLogReader) ListLogs(ctx context.Context, q log.ListLogsQuery) ([]log.LogRecord, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.records, len(f.records), nil
}

type fakeTraceReader struct {
	spans     []trace.Span
	summaries []trace.TraceSummary
	err       error
}

func (f fakeTraceReader) ListSpans(ctx context.Context, q trace.ListSpansQuery) ([]trace.Span, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.spans, len(f.spans), nil
}

func (f fakeTraceReader) ListTraceSummaries(ctx context.Context, q trace.ListSpansQuery) ([]trace.TraceSummary, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.summaries, len(f.summaries), nil
}

func TestMetricEvaluatorEvaluateFiresWhenThresholdExceeded(testRunner *testing.T) {
	evaluator := &MetricEvaluator{
		reader: fakeMetricReader{
			points: []metric.MetricPoint{
				{AvgValue: 92.5, SampleCount: 2},
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
			aggValue: &metric.MetricAggregation{
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
			points: []metric.MetricPoint{
				{AvgValue: 10.0, SampleCount: 1},
				{AvgValue: 100.0, SampleCount: 9},
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

func TestParseCondition(testRunner *testing.T) {
	condition := "avg(cpu_usage) > 80 for 5m"
	parsed, err := parseCondition(condition)
	if err != nil {
		testRunner.Fatalf("parseCondition returned error: %v", err)
	}
	if parsed.target != "cpu_usage" {
		testRunner.Fatalf("expected target cpu_usage, got %q", parsed.target)
	}
	if parsed.agg != "avg" {
		testRunner.Fatalf("expected agg avg, got %q", parsed.agg)
	}
	if parsed.threshold != 80 {
		testRunner.Fatalf("expected threshold 80, got %v", parsed.threshold)
	}
	if parsed.window != 5*time.Minute {
		testRunner.Fatalf("expected window 5m, got %v", parsed.window)
	}
}

func TestLogEvaluatorEvaluatesCorrectly(testRunner *testing.T) {
	evaluator := &LogEvaluator{
		reader: fakeLogReader{
			records: make([]log.LogRecord, 12),
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
			summaries: []trace.TraceSummary{
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

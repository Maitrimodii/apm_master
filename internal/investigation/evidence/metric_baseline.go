package evidence

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/Maitrimodii/apm_master/internal/metrics"
)

// serviceNameLabel is the OTel resource attribute naming the emitting service.
const serviceNameLabel = "service.name"

type metricAggregator interface {
	GetAggregations(ctx context.Context, query metrics.AggregationQuery) (metrics.MetricAggregation, error)
}

// metricBaseline compares a metric in the incident window with a baseline
// window (by default the same length immediately before the incident).
type metricBaseline struct{ metrics metricAggregator }

func (e metricBaseline) Execute(ctx context.Context, q Query) (Result, error) {
	metricName := paramString(q.Parameters, "metric_name")
	if metricName == "" {
		return Result{}, invalid("metric_name is required for %s", MetricBaseline)
	}
	svc := paramString(q.Parameters, "service_name")
	var labels map[string]string
	if svc != "" {
		labels = map[string]string{serviceNameLabel: svc}
	}

	baselineFrom, err := paramTime(q.Parameters, "baseline_from")
	if err != nil {
		return Result{}, err
	}
	baselineTo, err := paramTime(q.Parameters, "baseline_to")
	if err != nil {
		return Result{}, err
	}
	if baselineFrom.IsZero() || baselineTo.IsZero() {
		baselineTo = q.From
		baselineFrom = baselineTo.Add(-q.To.Sub(q.From))
	}

	aggregate := func(from, to time.Time) (metrics.MetricAggregation, error) {
		return e.metrics.GetAggregations(ctx, metrics.AggregationQuery{
			OrgID: q.TenantID, MetricName: metricName, From: from, To: to, Labels: labels,
		})
	}
	incident, err := aggregate(q.From, q.To)
	if err != nil {
		return Result{}, fmt.Errorf("aggregate incident metric: %w", err)
	}
	baseline, err := aggregate(baselineFrom, baselineTo)
	if err != nil {
		return Result{}, fmt.Errorf("aggregate baseline metric: %w", err)
	}

	var warnings []string
	if incident.Count == 0 {
		warnings = append(warnings, "No telemetry metric data points found in incident window")
	}
	if baseline.Count == 0 {
		warnings = append(warnings, "No baseline telemetry metric data points found in baseline window")
	}
	change := relativeChangePct(baseline.Avg, incident.Avg)
	return Result{
		Data: map[string]any{
			"metric_name":         metricName,
			"incident_window":     windowStats(q.From, q.To, incident),
			"baseline_window":     windowStats(baselineFrom, baselineTo, baseline),
			"relative_change_pct": change,
		},
		Warnings:    warnings,
		ServiceName: svc,
		Summary:     fmt.Sprintf("Metric %s shifted by %.1f%% compared to baseline", metricName, change),
	}, nil
}

// relativeChangePct is the change from baseline to current, in percent,
// rounded to two decimals. A rise from zero counts as 100%.
func relativeChangePct(baseline, current float64) float64 {
	switch {
	case baseline > 0:
		return math.Round((current-baseline)/baseline*10000) / 100
	case current > 0:
		return 100
	default:
		return 0
	}
}

func windowStats(from, to time.Time, a metrics.MetricAggregation) map[string]any {
	return map[string]any{
		"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339),
		"avg": a.Avg, "p95": a.P95, "p99": a.P99, "min": a.Min, "max": a.Max,
		"count": a.Count, "unit": a.Unit,
	}
}

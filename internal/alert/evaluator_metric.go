package alert

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/metrics"
)

type metricReader interface {
	GetAggregations(ctx context.Context, query metrics.AggregationQuery) (metrics.MetricAggregation, error)
}

// MetricEvaluator evaluates conditions on metric aggregations.
type MetricEvaluator struct {
	reader metricReader
	log    *slog.Logger
}

func NewMetricEvaluator(reader metricReader, log *slog.Logger) *MetricEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &MetricEvaluator{reader: reader, log: log}
}

func (e *MetricEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	return evaluateAlert(ctx, e, alert)
}

func (e *MetricEvaluator) evaluate(ctx context.Context, orgID uuid.UUID, c Condition) (*EvaluationResult, error) {
	now := time.Now()
	agg, err := e.reader.GetAggregations(ctx, metrics.AggregationQuery{
		OrgID:      orgID,
		MetricName: c.Target,
		From:       now.Add(-c.Window()),
		To:         now,
		Labels:     c.Filters,
	})
	if err != nil {
		return nil, fmt.Errorf("read metric aggregations: %w", err)
	}
	if agg.Count == 0 {
		return noData(c, fmt.Sprintf("no telemetry samples found for %s", c.Target)), nil
	}

	pick := map[string]float64{
		"avg": agg.Avg, "min": agg.Min, "max": agg.Max, "sum": agg.Total, "count": float64(agg.Count),
		"p50": agg.P50, "p90": agg.P90, "p95": agg.P95, "p99": agg.P99,
	}
	value := pick[c.Aggregation]
	return newResult(c, value, fmt.Sprintf("%s %s %.2f %s %.2f", c.Aggregation, c.Target, value, c.Operator, c.Threshold)), nil
}

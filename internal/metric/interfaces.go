package metric

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MetricStoreInterface abstracts database operations for metrics.
type MetricStoreInterface interface {
	ListMetricNames(ctx context.Context, query ListMetricsQuery) ([]string, int, error)
	GetSeries(ctx context.Context, query SeriesQuery) ([]MetricPoint, Resolution, error)
	GetHistogramBuckets(ctx context.Context, orgID uuid.UUID, metricName string, from, to time.Time) ([]HistogramBucket, error)
	ListMetricValues(ctx context.Context, query ListMetricValuesQuery) ([]MetricValue, int, error)
	GetAggregations(ctx context.Context, query AggregationQuery) (MetricAggregation, error)
	GetMetricUnit(ctx context.Context, orgID uuid.UUID, metricName string) (string, error)
	GetMetricType(ctx context.Context, orgID uuid.UUID, metricName string) (MetricType, error)
}

// compile-time assertion.
var _ MetricStoreInterface = (*Store)(nil)

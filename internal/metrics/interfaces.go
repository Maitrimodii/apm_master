package metrics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Reader abstracts database operations for metrics.
type Reader interface {
	ListMetricNames(ctx context.Context, query ListMetricsQuery) ([]string, int, error)
	GetSeries(ctx context.Context, query SeriesQuery) ([]MetricPoint, Resolution, error)
	GetHistogramBuckets(ctx context.Context, orgID uuid.UUID, metricName string, from, to time.Time) ([]HistogramBucket, error)
	ListMetricValues(ctx context.Context, query ListMetricValuesQuery) ([]MetricValue, int, error)
	GetAggregations(ctx context.Context, query AggregationQuery) (MetricAggregation, error)
	GetMetadata(ctx context.Context, orgID uuid.UUID, metricName string) (Metadata, error)
}

// compile-time assertion.
var _ Reader = (*Store)(nil)

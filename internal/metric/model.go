package metric

import (
	"time"

	"github.com/google/uuid"
)

// MetricType mirrors the OTel metric data model types.
// These are the only four types TimescaleDB migration supports.
type MetricType string

const (
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeCounter   MetricType = "counter"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// Metric is the internal domain model for a single metric data point.
// Histograms carry their sum here; bucket counts live in HistogramBucket.
type Metric struct {
	Time       time.Time      `json:"time"`
	OrgID      uuid.UUID      `json:"org_id"`
	MetricName string         `json:"metric_name"`
	MetricType MetricType     `json:"metric_type"`
	Value      float64        `json:"value"`
	Labels     map[string]any `json:"labels"`
}

// HistogramBucket is a single bucket of a histogram metric.
// Stored separately in metric_histogram_buckets to avoid wide pivots.
// Le is the upper bound of the bucket ("less than or equal").
type HistogramBucket struct {
	Time       time.Time      `json:"time"`
	OrgID      uuid.UUID      `json:"org_id"`
	MetricName string         `json:"metric_name"`
	Labels     map[string]any `json:"labels"`
	Le         float64        `json:"le"`
	Count      int64          `json:"count"`
}

// MetricPoint is a single data point in a time series response.
// Used by the series endpoint — keeps the response lean.
type MetricPoint struct {
	Bucket      time.Time `json:"bucket"`
	AvgValue    float64   `json:"avg_value"`
	MinValue    float64   `json:"min_value"`
	MaxValue    float64   `json:"max_value"`
	SampleCount int64     `json:"sample_count"`

	Avg   float64 `json:"avg"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Count int64   `json:"count"`
	Unit  string  `json:"unit,omitempty"`
}

type MetricValue struct {
	MetricName string         `json:"metric_name"`
	MetricType MetricType     `json:"metric_type"`
	Time       time.Time      `json:"timestamp"`
	Value      float64        `json:"value"`
	Labels     map[string]any `json:"labels"`
}

type MetricAggregation struct {
	MetricName string  `json:"metric_name"`
	Count      int64   `json:"count"`
	Total      float64 `json:"total"`
	Avg        float64 `json:"avg"`
	Min        float64 `json:"min"`
	Max        float64 `json:"max"`
	P50        float64 `json:"p50"`
	P90        float64 `json:"p90"`
	P95        float64 `json:"p95"`
	P99        float64 `json:"p99"`
	Unit       string  `json:"unit"`
}

type HistogramResponse struct {
	MetricName string            `json:"metric_name"`
	Unit       string            `json:"unit"`
	Buckets    []HistogramBucket `json:"buckets"`
}

// StandardizeUnit standardises various unit names to a canonical set of units
func StandardizeUnit(u string) string {
	switch u {
	case "s", "sec", "second", "seconds":
		return "seconds"
	case "ms", "millis", "millisecond", "milliseconds":
		return "milliseconds"
	case "us", "micro", "microsecond", "microseconds":
		return "microseconds"
	case "ns", "nano", "nanosecond", "nanoseconds":
		return "nanoseconds"
	case "b", "byte", "bytes":
		return "bytes"
	case "kb", "kilobyte", "kilobytes":
		return "kilobytes"
	case "mb", "megabyte", "megabytes":
		return "megabytes"
	case "pct", "percent", "%":
		return "percent"
	case "count", "counter", "1", "":
		return "count"
	default:
		return u
	}
}

// Resolution controls which continuous aggregate is queried.
// The store selects this automatically based on the requested time range.
type Resolution string

const (
	ResolutionRaw Resolution = "raw"
	Resolution1m  Resolution = "1m"
	Resolution5m  Resolution = "5m"
	Resolution1h  Resolution = "1h"
)

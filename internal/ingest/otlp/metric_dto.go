package otlp

// OTLP metric wire types.
// Mirrors the OTLP/JSON metric spec exactly.
// https://opentelemetry.io/docs/specs/otlp/#otlphttp

type IngestMetricsRequest struct {
	ResourceMetrics []ResourceMetrics `json:"resourceMetrics" binding:"required,min=1"`
}

type ResourceMetrics struct {
	Resource     Resource       `json:"resource"`
	ScopeMetrics []ScopeMetrics `json:"scopeMetrics"  binding:"required,min=1"`
}

type ScopeMetrics struct {
	Metrics []MetricDTO `json:"metrics" binding:"required,min=1"`
}

// MetricDTO carries exactly one of Gauge, Sum, Histogram, or Summary.
// The mapper checks which field is non-nil to determine the type.
type MetricDTO struct {
	Name        string        `json:"name"        binding:"required"`
	Description string        `json:"description"`
	Unit        string        `json:"unit"`
	Gauge       *GaugeDTO     `json:"gauge"`
	Sum         *SumDTO       `json:"sum"`
	Histogram   *HistogramDTO `json:"histogram"`
	Summary     *SummaryDTO   `json:"summary"`
}

type GaugeDTO struct {
	DataPoints []NumberDataPointDTO `json:"dataPoints"`
}

type SumDTO struct {
	DataPoints             []NumberDataPointDTO `json:"dataPoints"`
	AggregationTemporality int                  `json:"aggregationTemporality"`
	IsMonotonic            bool                 `json:"isMonotonic"`
}

type HistogramDTO struct {
	DataPoints             []HistogramDataPointDTO `json:"dataPoints"`
	AggregationTemporality int                     `json:"aggregationTemporality"`
}

type SummaryDTO struct {
	DataPoints []SummaryDataPointDTO `json:"dataPoints"`
}

type NumberDataPointDTO struct {
	TimeUnixNano int64          `json:"timeUnixNano"  binding:"required"`
	AsDouble     *float64       `json:"asDouble"`
	AsInt        *int64         `json:"asInt"`
	Attributes   map[string]any `json:"attributes"`
}

type HistogramDataPointDTO struct {
	TimeUnixNano   int64          `json:"timeUnixNano"   binding:"required"`
	Count          int64          `json:"count"`
	Sum            float64        `json:"sum"`
	BucketCounts   []int64        `json:"bucketCounts"`
	ExplicitBounds []float64      `json:"explicitBounds"`
	Attributes     map[string]any `json:"attributes"`
}

type SummaryDataPointDTO struct {
	TimeUnixNano   int64              `json:"timeUnixNano"   binding:"required"`
	Count          int64              `json:"count"`
	Sum            float64            `json:"sum"`
	QuantileValues []QuantileValueDTO `json:"quantileValues"`
	Attributes     map[string]any     `json:"attributes"`
}

type QuantileValueDTO struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}

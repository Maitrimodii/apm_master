package metric

import (
	"context"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/google/uuid"
)

type Service struct {
	store *Store
	log   *slog.Logger
}

func NewService(store *Store, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{store: store, log: log}
}

// ListParams is the input for listing metric names.
type ListParams struct {
	ServiceName string
	Limit       int
	Offset      int
}

// SeriesParams is the input for fetching a metric time series.
type SeriesParams struct {
	MetricName string
	From       time.Time
	To         time.Time
	Labels     map[string]string
	Interval   string // optional custom aggregation interval (e.g., "5m", "1h")
}

type ListMetricValuesParams struct {
	MetricName  string
	ServiceName string
	Limit       int
	Offset      int
}

type AggregationParams struct {
	MetricName  string
	From        time.Time
	To          time.Time
	Labels      map[string]string
	ServiceName string
}

// SeriesResponse carries the points alongside the resolution that was used.
// The frontend uses resolution to label the X-axis correctly
// ("per minute" vs "per hour").
type SeriesResponse struct {
	MetricName string        `json:"metric_name"`
	Resolution Resolution    `json:"resolution"`
	Points     []MetricPoint `json:"points"`
	Unit       string        `json:"unit"`
}

// HistogramParams is the input for fetching histogram bucket data.
type HistogramParams struct {
	MetricName string
	From       time.Time
	To         time.Time
}

func (service *Service) ListMetricNames(ctx context.Context, orgID uuid.UUID, params ListParams) ([]string, int, error) {
	query := ListMetricsQuery{
		OrgID:       orgID,
		ServiceName: params.ServiceName,
		Limit:       params.Limit,
		Offset:      params.Offset,
	}

	names, total, err := service.store.ListMetricNames(ctx, query)
	if err != nil {
		service.log.ErrorContext(ctx, "list metric names failed", "org_id", orgID, "error", err)
		return nil, 0, server.Internal(err)
	}

	service.log.DebugContext(ctx, "metric names listed", "org_id", orgID, "total", total, "limit", params.Limit, "offset", params.Offset)

	return names, total, nil
}

func (service *Service) ListMetricValues(ctx context.Context, orgID uuid.UUID, params ListMetricValuesParams) ([]MetricValue, int, error) {
	if params.MetricName == "" {
		return nil, 0, server.BadRequest(server.CodeValidation, "metric_name is required")
	}

	query := ListMetricValuesQuery{
		OrgID:       orgID,
		MetricName:  params.MetricName,
		ServiceName: params.ServiceName,
		Limit:       params.Limit,
		Offset:      params.Offset,
	}

	values, total, err := service.store.ListMetricValues(ctx, query)
	if err != nil {
		service.log.ErrorContext(ctx, "list metric values failed", "org_id", orgID, "metric_name", params.MetricName, "error", err)
		return nil, 0, server.Internal(err)
	}

	unit, _ := service.store.GetMetricUnit(ctx, orgID, params.MetricName)
	unit = StandardizeUnit(unit)
	mType, _ := service.store.GetMetricType(ctx, orgID, params.MetricName)

	for index := range values {
		values[index].Value = roundMetricValue(values[index].Value, params.MetricName, mType, unit)
	}

	service.log.DebugContext(ctx, "metric values listed", "org_id", orgID, "metric_name", params.MetricName, "total", total, "limit", params.Limit, "offset", params.Offset)

	return values, total, nil
}

func (service *Service) GetAggregations(ctx context.Context, orgID uuid.UUID, params AggregationParams) (MetricAggregation, error) {
	if params.MetricName == "" {
		return MetricAggregation{}, server.BadRequest(server.CodeValidation, "metric_name is required")
	}

	if params.To.Before(params.From) {
		return MetricAggregation{}, server.BadRequest(server.CodeValidation, "'to' must be after 'from'")
	}

	labels := params.Labels
	if params.ServiceName != "" {
		if labels == nil {
			labels = map[string]string{}
		}
		labels["service.name"] = params.ServiceName
	}

	query := AggregationQuery{
		OrgID:      orgID,
		MetricName: params.MetricName,
		From:       params.From,
		To:         params.To,
		Labels:     labels,
	}

	agg, err := service.store.GetAggregations(ctx, query)
	if err != nil {
		service.log.ErrorContext(ctx, "get metric aggregations failed", "org_id", orgID, "metric_name", params.MetricName, "error", err)
		return MetricAggregation{}, server.Internal(err)
	}

	service.log.DebugContext(ctx, "metric aggregations fetched", "org_id", orgID, "metric_name", params.MetricName)

	return agg, nil
}

func (service *Service) GetSeries(ctx context.Context, orgID uuid.UUID, params SeriesParams) (SeriesResponse, error) {
	if params.MetricName == "" {
		return SeriesResponse{}, server.BadRequest(server.CodeValidation, "metric_name is required")
	}

	if params.To.Before(params.From) {
		return SeriesResponse{}, server.BadRequest(server.CodeValidation, "'to' must be after 'from'")
	}

	resolution := selectResolution(params.From, params.To)

	query := SeriesQuery{
		OrgID:      orgID,
		MetricName: params.MetricName,
		From:       params.From,
		To:         params.To,
		Labels:     params.Labels,
		Interval:   params.Interval,
	}

	points, _, err := service.store.GetSeries(ctx, query)
	if err != nil {
		service.log.ErrorContext(ctx, "get metric series failed", "org_id", orgID, "metric_name", params.MetricName, "error", err)
		return SeriesResponse{}, server.Internal(err)
	}

	unit, _ := service.store.GetMetricUnit(ctx, orgID, params.MetricName)
	unit = StandardizeUnit(unit)
	mType, _ := service.store.GetMetricType(ctx, orgID, params.MetricName)

	for index := range points {
		points[index].Avg = roundMetricValue(points[index].AvgValue, params.MetricName, mType, unit)
		points[index].Min = roundMetricValue(points[index].MinValue, params.MetricName, mType, unit)
		points[index].Max = roundMetricValue(points[index].MaxValue, params.MetricName, mType, unit)
		points[index].Count = points[index].SampleCount
		points[index].Unit = unit

		points[index].AvgValue = points[index].Avg
		points[index].MinValue = points[index].Min
		points[index].MaxValue = points[index].Max
	}

	service.log.DebugContext(ctx, "metric series fetched", "org_id", orgID, "metric_name", params.MetricName, "resolution", resolution, "points", len(points))

	return SeriesResponse{
		MetricName: params.MetricName,
		Resolution: resolution,
		Points:     points,
		Unit:       unit,
	}, nil
}

func (service *Service) GetHistogramBuckets(ctx context.Context, orgID uuid.UUID, params HistogramParams) (HistogramResponse, error) {
	if params.MetricName == "" {
		return HistogramResponse{}, server.BadRequest(server.CodeValidation, "metric_name is required")
	}

	buckets, err := service.store.GetHistogramBuckets(ctx, orgID, params.MetricName, params.From, params.To)
	if err != nil {
		service.log.ErrorContext(ctx, "get histogram buckets failed", "org_id", orgID, "metric_name", params.MetricName, "error", err)
		return HistogramResponse{}, server.Internal(err)
	}

	unit, _ := service.store.GetMetricUnit(ctx, orgID, params.MetricName)
	unit = StandardizeUnit(unit)

	service.log.DebugContext(ctx, "histogram buckets fetched", "org_id", orgID, "metric_name", params.MetricName, "buckets", len(buckets))

	return HistogramResponse{
		MetricName: params.MetricName,
		Unit:       unit,
		Buckets:    buckets,
	}, nil
}

// Percentile calculates the p-th percentile of a sorted slice of float64 using linear interpolation
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	count := float64(len(sorted))
	idx := p * (count - 1)
	key := math.Floor(idx)
	d := idx - key

	intK := int(key)
	if intK >= len(sorted)-1 {
		return sorted[len(sorted)-1]
	}

	return sorted[intK] + d*(sorted[intK+1]-sorted[intK])
}

func roundMetricValue(val float64, name string, mType MetricType, unit string) float64 {
	unit = strings.ToLower(unit)
	name = strings.ToLower(name)
	mType = MetricType(strings.ToLower(string(mType)))

	// 1. Duration metrics -> 4 decimal places
	if unit == "seconds" || unit == "milliseconds" || unit == "microseconds" || unit == "nanoseconds" ||
		strings.Contains(name, "duration") || strings.Contains(name, "latency") {
		return math.Round(val*10000) / 10000
	}

	// 2. Percentages -> 2 decimal places
	if unit == "percent" || strings.Contains(name, "percent") || strings.Contains(name, "ratio") || strings.Contains(name, "pct") {
		return math.Round(val*100) / 100
	}

	// 3. Counters -> Integer output (0 decimal places)
	if mType == MetricTypeCounter || unit == "count" || strings.Contains(name, "total") || strings.Contains(name, "count") {
		return math.Round(val)
	}

	// Default fallback
	return math.Round(val*10000) / 10000
}

package metrics

import (
	"context"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

type Service struct {
	store Reader
	log   *slog.Logger
}

func NewService(store Reader, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, log: log}
}

// SeriesResponse carries the points alongside the resolution that was used,
// so the UI can label the x-axis ("per minute" vs "per hour").
type SeriesResponse struct {
	MetricName string        `json:"metric_name"`
	Resolution Resolution    `json:"resolution"`
	Unit       string        `json:"unit"`
	Points     []MetricPoint `json:"points"`
}

func (s *Service) ListMetricNames(ctx context.Context, query ListMetricsQuery) ([]string, int, error) {
	names, total, err := s.store.ListMetricNames(ctx, query)
	if err != nil {
		s.log.ErrorContext(ctx, "list metric names failed", "org_id", query.OrgID, "error", err)
		return nil, 0, apperr.Internal(err)
	}
	return names, total, nil
}

func (s *Service) ListMetricValues(ctx context.Context, query ListMetricValuesQuery) ([]MetricValue, int, error) {
	values, total, err := s.store.ListMetricValues(ctx, query)
	if err != nil {
		s.log.ErrorContext(ctx, "list metric values failed", "org_id", query.OrgID, "metric_name", query.MetricName, "error", err)
		return nil, 0, apperr.Internal(err)
	}
	return values, total, nil
}

func (s *Service) GetAggregations(ctx context.Context, query AggregationQuery) (MetricAggregation, error) {
	agg, err := s.store.GetAggregations(ctx, query)
	if err != nil {
		s.log.ErrorContext(ctx, "get metric aggregations failed", "org_id", query.OrgID, "metric_name", query.MetricName, "error", err)
		return MetricAggregation{}, apperr.Internal(err)
	}
	md, err := s.metadata(ctx, query.OrgID, query.MetricName)
	if err != nil {
		return MetricAggregation{}, err
	}
	agg.Unit = md.Unit
	return agg, nil
}

func (s *Service) GetSeries(ctx context.Context, query SeriesQuery) (SeriesResponse, error) {
	points, resolution, err := s.store.GetSeries(ctx, query)
	if err != nil {
		s.log.ErrorContext(ctx, "get metric series failed", "org_id", query.OrgID, "metric_name", query.MetricName, "error", err)
		return SeriesResponse{}, apperr.Internal(err)
	}
	md, err := s.metadata(ctx, query.OrgID, query.MetricName)
	if err != nil {
		return SeriesResponse{}, err
	}
	if points == nil {
		points = []MetricPoint{}
	}
	return SeriesResponse{MetricName: query.MetricName, Resolution: resolution, Unit: md.Unit, Points: points}, nil
}

func (s *Service) GetHistogram(ctx context.Context, orgID uuid.UUID, metricName string, tr httpx.TimeRange) (HistogramResponse, error) {
	buckets, err := s.store.GetHistogramBuckets(ctx, orgID, metricName, tr.From, tr.To)
	if err != nil {
		s.log.ErrorContext(ctx, "get histogram buckets failed", "org_id", orgID, "metric_name", metricName, "error", err)
		return HistogramResponse{}, apperr.Internal(err)
	}
	md, err := s.metadata(ctx, orgID, metricName)
	if err != nil {
		return HistogramResponse{}, err
	}
	if buckets == nil {
		buckets = []HistogramBucket{}
	}
	return HistogramResponse{MetricName: metricName, Unit: md.Unit, Buckets: buckets}, nil
}

func (s *Service) metadata(ctx context.Context, orgID uuid.UUID, metricName string) (Metadata, error) {
	md, err := s.store.GetMetadata(ctx, orgID, metricName)
	if err != nil {
		s.log.ErrorContext(ctx, "get metric metadata failed", "org_id", orgID, "metric_name", metricName, "error", err)
		return Metadata{}, apperr.Internal(err)
	}
	return md, nil
}

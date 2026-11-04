package services

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/otlp"
	"github.com/Maitrimodii/apm_master/internal/ingest/producer"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

const (
	// serviceNameLabel is the OTel resource attribute naming the emitting service.
	serviceNameLabel = "service.name"
	// unknownService is used when a metrics batch carries no service name.
	unknownService = "unknown"
)

// Service maps OTLP requests to telemetry events and publishes them to Kafka.
type Service struct {
	producer producer.Producer
	log      *slog.Logger
}

func NewService(producer producer.Producer, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{producer: producer, log: log}
}

func (s *Service) IngestTraces(ctx context.Context, orgID uuid.UUID, req otlp.IngestRequest) (*otlp.IngestResponse, error) {
	spans, rejected := otlp.MapRequest(req, orgID)
	var traceID, svc string
	if len(spans) > 0 {
		traceID, svc = spans[0].TraceID, spans[0].ServiceName
	}
	return s.publish(ctx, model.SignalTraces, orgID, len(spans), rejected, model.SpansToWire(spans), traceID, svc)
}

func (s *Service) IngestLogs(ctx context.Context, orgID uuid.UUID, req otlp.IngestLogsRequest) (*otlp.IngestResponse, error) {
	records, rejected := otlp.MapLogsRequest(req, orgID)
	var traceID, svc string
	if len(records) > 0 {
		svc = records[0].ServiceName
		if records[0].TraceID != nil {
			traceID = *records[0].TraceID
		}
	}
	return s.publish(ctx, model.SignalLogs, orgID, len(records), rejected, model.LogsToWire(records), traceID, svc)
}

func (s *Service) IngestMetrics(ctx context.Context, orgID uuid.UUID, req otlp.IngestMetricsRequest) (*otlp.IngestResponse, error) {
	batch, rejected := otlp.MapMetricsRequest(req, orgID)
	svc := unknownService
	for _, labels := range labelSets(batch) {
		if name, ok := labels[serviceNameLabel].(string); ok && name != "" {
			svc = name
			break
		}
	}
	accepted := len(batch.Metrics)
	if accepted == 0 && len(batch.Buckets) > 0 {
		// A histogram-only batch still carries data worth publishing.
		accepted = len(batch.Buckets)
	}
	resp, err := s.publish(ctx, model.SignalMetrics, orgID, accepted, rejected, model.MetricsToWire(batch.Metrics, batch.Buckets), "", svc)
	if resp != nil {
		resp.Accepted = len(batch.Metrics)
	}
	return resp, err
}

// labelSets returns the first metric's and first bucket's labels, the places
// a service name is looked for.
func labelSets(batch otlp.MetricBatch) []map[string]any {
	var sets []map[string]any
	if len(batch.Metrics) > 0 {
		sets = append(sets, batch.Metrics[0].Labels)
	}
	if len(batch.Buckets) > 0 {
		sets = append(sets, batch.Buckets[0].Labels)
	}
	return sets
}

// publish wraps a wire payload in an event and sends it. Requests where every
// record was rejected publish nothing.
func (s *Service) publish(ctx context.Context, signal model.Signal, orgID uuid.UUID, accepted, rejected int, payload any, traceID, serviceName string) (*otlp.IngestResponse, error) {
	if accepted > 0 {
		data, err := serializer.MarshalPayload(payload)
		if err != nil {
			return nil, apperr.Internal(err)
		}
		event := model.NewEvent(signal, orgID, accepted, rejected, data)
		event.TraceID = traceID
		event.ServiceName = serviceName
		if err := s.producer.Publish(ctx, event); err != nil {
			return nil, apperr.Internal(err)
		}
	}
	s.log.InfoContext(ctx, "telemetry ingested", "signal", signal, "org_id", orgID, "accepted", accepted, "rejected", rejected)
	return &otlp.IngestResponse{Accepted: accepted, Rejected: rejected}, nil
}

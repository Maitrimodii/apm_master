package services

import (
	"context"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/otlp"
	"github.com/Maitrimodii/apm_master/internal/ingest/producer"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/google/uuid"
)

type Service struct {
	producer producer.Producer
	log      *slog.Logger
}

func NewService(
	producer producer.Producer,
	log *slog.Logger,
) *Service {
	if log == nil {
		log = slog.Default()
	}

	return &Service{
		producer: producer,
		log:      log,
	}
}

func (svc *Service) IngestTraces(
	ctx context.Context,
	orgID uuid.UUID,
	req otlp.IngestRequest,
) (*otlp.IngestResponse, error) {
	spans, rejected := otlp.MapRequest(req, orgID)

	if len(spans) > 0 {
		payload, err := serializer.MarshalPayload(spans)
		if err != nil {
			return nil, server.Internal(err)
		}
		event := model.NewEvent(model.SignalTraces, orgID, len(spans), rejected, payload)
		if len(spans) > 0 {
			event.TraceID = spans[0].TraceID
			event.ServiceName = spans[0].ServiceName
		}
		if err := svc.producer.Publish(ctx, event); err != nil {
			return nil, server.Internal(err)
		}
	}

	svc.log.Info("traces ingested", "org_id", orgID, "accepted", len(spans), "rejected", rejected)

	return &otlp.IngestResponse{
		Accepted: len(spans),
		Rejected: rejected,
	}, nil
}

func (svc *Service) IngestLogs(
	ctx context.Context,
	orgID uuid.UUID,
	req otlp.IngestLogsRequest,
) (*otlp.IngestResponse, error) {
	records, rejected := otlp.MapLogsRequest(req, orgID)

	if len(records) > 0 {
		payload, err := serializer.MarshalPayload(records)
		if err != nil {
			return nil, server.Internal(err)
		}
		event := model.NewEvent(model.SignalLogs, orgID, len(records), rejected, payload)
		if len(records) > 0 {
			if records[0].TraceID != nil {
				event.TraceID = *records[0].TraceID
			}
			event.ServiceName = records[0].ServiceName
		}
		if err := svc.producer.Publish(ctx, event); err != nil {
			return nil, server.Internal(err)
		}
	}

	svc.log.Info("logs ingested", "org_id", orgID, "accepted", len(records), "rejected", rejected)

	return &otlp.IngestResponse{
		Accepted: len(records),
		Rejected: rejected,
	}, nil
}

func (svc *Service) IngestMetrics(
	ctx context.Context,
	orgID uuid.UUID,
	req otlp.IngestMetricsRequest,
) (*otlp.IngestResponse, error) {
	batch, rejected := otlp.MapMetricsRequest(req, orgID)

	if len(batch.Metrics) > 0 ||
		len(batch.Buckets) > 0 {
		payload, err := serializer.MarshalPayload(metric.WriteBatch{
			Metrics: batch.Metrics,
			Buckets: batch.Buckets,
		})
		if err != nil {
			return nil, server.Internal(err)
		}
		event := model.NewEvent(model.SignalMetrics, orgID, len(batch.Metrics), rejected, payload)
		var serviceName string
		if len(batch.Metrics) > 0 {
			if svc, exists := batch.Metrics[0].Labels["service.name"].(string); exists {
				serviceName = svc
			}
		}
		if serviceName == "" && len(batch.Buckets) > 0 {
			if svc, exists := batch.Buckets[0].Labels["service.name"].(string); exists {
				serviceName = svc
			}
		}
		if serviceName == "" {
			serviceName = "unknown"
		}
		event.ServiceName = serviceName

		if err := svc.producer.Publish(ctx, event); err != nil {
			return nil, server.Internal(err)
		}
	}

	svc.log.Info("metrics ingested", "org_id", orgID, "accepted", len(batch.Metrics), "histogram_buckets", len(batch.Buckets), "rejected", rejected)

	return &otlp.IngestResponse{
		Accepted: len(batch.Metrics),
		Rejected: rejected,
	}, nil
}

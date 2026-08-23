package services

import (
	"context"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/otlp"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
)

type recordingProducer struct {
	events []model.Event
}

func (p *recordingProducer) Publish(ctx context.Context, event model.Event) error {
	p.events = append(p.events, event)
	return nil
}

func (p *recordingProducer) Close() error {
	return nil
}

func TestIngestTracesPublishesKafkaEvent(testRunner *testing.T) {
	orgID := uuid.New()
	start := time.Now().UnixNano()
	producer := &recordingProducer{}
	service := NewService(producer, nil)

	resp, err := service.IngestTraces(context.Background(), orgID, otlp.IngestRequest{
		ResourceSpans: []otlp.ResourceSpans{{
			Resource: otlp.Resource{Attributes: map[string]any{"service.name": "checkout"}},
			ScopeSpans: []otlp.ScopeSpans{{
				Spans: []otlp.SpanDTO{{
					TraceID:           "trace-1",
					SpanID:            "span-1",
					Name:              "GET /checkout",
					StartTimeUnixNano: start,
					EndTimeUnixNano:   start + int64(time.Second),
				}},
			}},
		}},
	})
	if err != nil {
		testRunner.Fatalf("IngestTraces() error = %v", err)
	}

	if resp.Accepted != 1 || resp.Rejected != 0 {
		testRunner.Fatalf("response = %+v, want accepted=1 rejected=0", resp)
	}
	if len(producer.events) != 1 {
		testRunner.Fatalf("published events = %d, want 1", len(producer.events))
	}

	event := producer.events[0]
	if event.Signal != model.SignalTraces {
		testRunner.Fatalf("event signal = %q, want %q", event.Signal, model.SignalTraces)
	}
	if event.OrgID != orgID || event.TenantID != orgID {
		testRunner.Fatalf("event org/tenant = %s/%s, want %s", event.OrgID, event.TenantID, orgID)
	}
	if event.TraceID != "trace-1" {
		testRunner.Fatalf("event trace_id = %q, want %q", event.TraceID, "trace-1")
	}
	if event.ServiceName != "checkout" {
		testRunner.Fatalf("event service_name = %q, want %q", event.ServiceName, "checkout")
	}

	spans, err := serializer.UnmarshalPayload[[]trace.Span](event.Payload)
	if err != nil {
		testRunner.Fatalf("unmarshal payload: %v", err)
	}
	if len(spans) != 1 || spans[0].SpanID != "span-1" {
		testRunner.Fatalf("spans = %+v, want span-1", spans)
	}
}

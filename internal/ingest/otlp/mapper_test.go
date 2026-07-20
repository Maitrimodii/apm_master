package otlp

import (
	"math"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/google/uuid"
)

func TestMapRequestMapsValidSpansAndRejectsInvalidSpans(testRunner *testing.T) {
	orgID := uuid.New()
	start := time.Now().UnixNano()
	end := start + int64(time.Second)

	spans, rejected := MapRequest(IngestRequest{
		ResourceSpans: []ResourceSpans{{
			Resource: Resource{Attributes: map[string]any{"service.name": "checkout"}},
			ScopeSpans: []ScopeSpans{{
				Spans: []SpanDTO{
					{
						TraceID:           "trace-1",
						SpanID:            "span-1",
						ParentSpanID:      "parent-1",
						Name:              "POST /checkout",
						Kind:              2,
						StartTimeUnixNano: start,
						EndTimeUnixNano:   end,
						Status:            SpanStatusDTO{Code: 2, Message: "failed"},
						Attributes:        map[string]any{"http.method": "POST"},
					},
					{
						TraceID:           "trace-2",
						SpanID:            "span-2",
						Name:              "bad span",
						StartTimeUnixNano: end,
						EndTimeUnixNano:   start,
					},
				},
			}},
		}},
	}, orgID)

	if rejected != 1 {
		testRunner.Fatalf("rejected = %d, want 1", rejected)
	}
	if len(spans) != 1 {
		testRunner.Fatalf("len(spans) = %d, want 1", len(spans))
	}

	span := spans[0]
	if span.OrgID != orgID {
		testRunner.Fatalf("OrgID = %s, want %s", span.OrgID, orgID)
	}
	if span.ServiceName != "checkout" {
		testRunner.Fatalf("ServiceName = %q, want checkout", span.ServiceName)
	}
	if span.SpanKind != "SERVER" {
		testRunner.Fatalf("SpanKind = %q, want SERVER", span.SpanKind)
	}
	if span.StatusCode != "ERROR" {
		testRunner.Fatalf("StatusCode = %q, want ERROR", span.StatusCode)
	}
	if span.DurationNS != int64(time.Second) {
		testRunner.Fatalf("DurationNS = %d, want %d", span.DurationNS, int64(time.Second))
	}
	if span.ParentID == nil || *span.ParentID != "parent-1" {
		testRunner.Fatalf("ParentID = %v, want parent-1", span.ParentID)
	}
}

func TestMapLogsRequestDefaultsAndRejectsMissingTime(testRunner *testing.T) {
	orgID := uuid.New()
	timestamp := time.Now().UnixNano()

	records, rejected := MapLogsRequest(IngestLogsRequest{
		ResourceLogs: []ResourceLogs{{
			Resource: Resource{Attributes: map[string]any{"service.name": "api"}},
			ScopeLogs: []ScopeLogs{{
				LogRecords: []LogRecordDTO{
					{TimeUnixNano: timestamp, SeverityNumber: 17, Body: "boom", TraceID: "trace-1"},
					{Body: "missing timestamp"},
				},
			}},
		}},
	}, orgID)

	if rejected != 0 {
		testRunner.Fatalf("rejected = %d, want 0", rejected)
	}
	if len(records) != 2 {
		testRunner.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].SeverityText != "ERROR" {
		testRunner.Fatalf("SeverityText = %q, want ERROR", records[0].SeverityText)
	}
	if records[0].TraceID == nil || *records[0].TraceID != "trace-1" {
		testRunner.Fatalf("TraceID = %v, want trace-1", records[0].TraceID)
	}
	if records[1].Time.IsZero() {
		testRunner.Fatalf("expected fallback timestamp to be set, got zero time")
	}
}

func TestMapMetricsRequestMapsGaugeHistogramAndRejectsEmptyMetric(testRunner *testing.T) {
	orgID := uuid.New()
	timestamp := time.Now().UnixNano()

	batch, rejected := MapMetricsRequest(IngestMetricsRequest{
		ResourceMetrics: []ResourceMetrics{{
			Resource: Resource{Attributes: map[string]any{"service.name": "worker", "env": "prod"}},
			ScopeMetrics: []ScopeMetrics{{
				Metrics: []MetricDTO{
					{
						Name: "cpu.usage",
						Gauge: &GaugeDTO{DataPoints: []NumberDataPointDTO{{
							TimeUnixNano: timestamp,
							AsDouble:     floatPtr(42.5),
							Attributes:   map[string]any{"host": "node-1", "env": "canary"},
						}}},
					},
					{
						Name: "cpu.idle",
						Gauge: &GaugeDTO{DataPoints: []NumberDataPointDTO{{
							TimeUnixNano: timestamp,
							AsInt:        intPtr(100),
							Attributes:   map[string]any{"host": "node-2"},
						}}},
					},
					{
						Name: "request.duration",
						Histogram: &HistogramDTO{DataPoints: []HistogramDataPointDTO{{
							TimeUnixNano:   timestamp,
							Sum:            12.5,
							BucketCounts:   []int64{1, 3, 5},
							ExplicitBounds: []float64{10, 50},
						}}},
					},
					{Name: "broken"},
				},
			}},
		}},
	}, orgID)

	if rejected != 1 {
		testRunner.Fatalf("rejected = %d, want 1", rejected)
	}
	if len(batch.Metrics) != 3 {
		testRunner.Fatalf("len(Metrics) = %d, want 3", len(batch.Metrics))
	}
	if len(batch.Buckets) != 3 {
		testRunner.Fatalf("len(Buckets) = %d, want 3", len(batch.Buckets))
	}

	gaugeDouble := batch.Metrics[0]
	if gaugeDouble.MetricType != metric.MetricTypeGauge {
		testRunner.Fatalf("MetricType = %q, want %q", gaugeDouble.MetricType, metric.MetricTypeGauge)
	}
	if gaugeDouble.Value != 42.5 {
		testRunner.Fatalf("Value = %v, want 42.5", gaugeDouble.Value)
	}
	if gaugeDouble.Labels["env"] != "canary" {
		testRunner.Fatalf("label env = %v, want canary", gaugeDouble.Labels["env"])
	}

	gaugeInt := batch.Metrics[1]
	if gaugeInt.MetricType != metric.MetricTypeGauge {
		testRunner.Fatalf("MetricType = %q, want %q", gaugeInt.MetricType, metric.MetricTypeGauge)
	}
	if gaugeInt.Value != 100.0 {
		testRunner.Fatalf("Value = %v, want 100.0", gaugeInt.Value)
	}
	if gaugeInt.Labels["host"] != "node-2" {
		testRunner.Fatalf("label host = %v, want node-2", gaugeInt.Labels["host"])
	}

	if !math.IsInf(batch.Buckets[2].Le, 1) {
		testRunner.Fatalf("last bucket Le = %v, want +Inf", batch.Buckets[2].Le)
	}
}

func floatPtr(f float64) *float64 { return &f }
func intPtr(index int64) *int64   { return &index }

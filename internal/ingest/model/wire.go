package model

import (
	"encoding/json"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

// Wire types are the Kafka payload format (schema version 1). They are kept
// separate from the query modules' API models so changing an API response
// can't silently change the Kafka contract. Field names match the JSON the
// producer wrote before these types existed, so in-flight messages still decode.

type SpanV1 struct {
	SpanID        string         `json:"span_id"`
	TraceID       string         `json:"trace_id"`
	ParentID      *string        `json:"parent_id,omitempty"`
	OrgID         uuid.UUID      `json:"org_id"`
	ServiceName   string         `json:"service_name"`
	SpanName      string         `json:"span_name"`
	SpanKind      string         `json:"span_kind"`
	StartTime     time.Time      `json:"start_time"`
	EndTime       time.Time      `json:"end_time"`
	DurationNS    int64          `json:"duration_ns"`
	StatusCode    string         `json:"status_code"`
	StatusMessage *string        `json:"status_message,omitempty"`
	ResourceAttrs map[string]any `json:"resource_attrs"`
	SpanAttrs     map[string]any `json:"span_attrs"`
	Events        []SpanEventV1  `json:"events"`
	Links         []SpanLinkV1   `json:"links"`
}

type SpanEventV1 struct {
	Name  string         `json:"name"`
	Time  int64          `json:"time_unix_nano"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

type SpanLinkV1 struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	TraceState string         `json:"trace_state,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

type LogRecordV1 struct {
	Time          time.Time      `json:"time"`
	OrgID         uuid.UUID      `json:"org_id"`
	ServiceName   string         `json:"service_name"`
	TraceID       *string        `json:"trace_id,omitempty"`
	SpanID        *string        `json:"span_id,omitempty"`
	SeverityNum   int16          `json:"severity_num"`
	SeverityText  string         `json:"severity_text"`
	Body          string         `json:"body"`
	ResourceAttrs map[string]any `json:"resource_attrs"`
	LogAttrs      map[string]any `json:"log_attrs"`
}

// MetricBatchV1 keeps the untagged field names WriteBatch used to marshal with.
type MetricBatchV1 struct {
	Metrics []MetricV1          `json:"Metrics"`
	Buckets []HistogramBucketV1 `json:"Buckets"`
}

type MetricV1 struct {
	Time       time.Time      `json:"time"`
	OrgID      uuid.UUID      `json:"org_id"`
	MetricName string         `json:"metric_name"`
	MetricType string         `json:"metric_type"`
	Value      float64        `json:"value"`
	Labels     map[string]any `json:"labels"`
}

type HistogramBucketV1 struct {
	Time       time.Time      `json:"time"`
	OrgID      uuid.UUID      `json:"org_id"`
	MetricName string         `json:"metric_name"`
	Labels     map[string]any `json:"labels"`
	Le         float64        `json:"le"`
	Count      int64          `json:"count"`
}

// MarshalJSON writes the +Inf overflow bound as "le": null; JSON has no
// infinity, so encoding/json would otherwise reject the whole batch.
func (b HistogramBucketV1) MarshalJSON() ([]byte, error) {
	type plain HistogramBucketV1
	out := struct {
		plain
		Le *float64 `json:"le"`
	}{plain: plain(b)}
	if !math.IsInf(b.Le, 1) {
		le := b.Le
		out.Le = &le
	}
	return json.Marshal(out)
}

// UnmarshalJSON reads "le": null back as +Inf.
func (b *HistogramBucketV1) UnmarshalJSON(data []byte) error {
	type plain HistogramBucketV1
	in := struct {
		*plain
		Le *float64 `json:"le"`
	}{plain: (*plain)(b)}
	if err := json.Unmarshal(data, &in); err != nil {
		return err
	}
	b.Le = math.Inf(1)
	if in.Le != nil {
		b.Le = *in.Le
	}
	return nil
}

func SpansToWire(spans []traces.Span) []SpanV1 {
	out := make([]SpanV1, len(spans))
	for i, s := range spans {
		w := SpanV1{
			SpanID: s.SpanID, TraceID: s.TraceID, ParentID: s.ParentID, OrgID: s.OrgID,
			ServiceName: s.ServiceName, SpanName: s.SpanName, SpanKind: s.SpanKind,
			StartTime: s.StartTime, EndTime: s.EndTime, DurationNS: s.DurationNS,
			StatusCode: s.StatusCode, StatusMessage: s.StatusMessage,
			ResourceAttrs: s.ResourceAttrs, SpanAttrs: s.SpanAttrs,
		}
		for _, e := range s.Events {
			w.Events = append(w.Events, SpanEventV1{Name: e.Name, Time: e.Time, Attrs: e.Attrs})
		}
		for _, l := range s.Links {
			w.Links = append(w.Links, SpanLinkV1{TraceID: l.TraceID, SpanID: l.SpanID, TraceState: l.TraceState, Attrs: l.Attrs})
		}
		out[i] = w
	}
	return out
}

func SpansFromWire(wire []SpanV1) []traces.Span {
	out := make([]traces.Span, len(wire))
	for i, w := range wire {
		s := traces.Span{
			SpanID: w.SpanID, TraceID: w.TraceID, ParentID: w.ParentID, OrgID: w.OrgID,
			ServiceName: w.ServiceName, SpanName: w.SpanName, SpanKind: w.SpanKind,
			StartTime: w.StartTime, EndTime: w.EndTime, DurationNS: w.DurationNS,
			DurationMS: float64(w.DurationNS) / 1e6,
			StatusCode: w.StatusCode, StatusMessage: w.StatusMessage,
			ResourceAttrs: w.ResourceAttrs, SpanAttrs: w.SpanAttrs,
		}
		for _, e := range w.Events {
			s.Events = append(s.Events, traces.SpanEvent{Name: e.Name, Time: e.Time, Attrs: e.Attrs})
		}
		for _, l := range w.Links {
			s.Links = append(s.Links, traces.SpanLink{TraceID: l.TraceID, SpanID: l.SpanID, TraceState: l.TraceState, Attrs: l.Attrs})
		}
		out[i] = s
	}
	return out
}

func LogsToWire(records []logs.LogRecord) []LogRecordV1 {
	out := make([]LogRecordV1, len(records))
	for i, r := range records {
		out[i] = LogRecordV1{
			Time: r.Time, OrgID: r.OrgID, ServiceName: r.ServiceName, TraceID: r.TraceID, SpanID: r.SpanID,
			SeverityNum: r.SeverityNum, SeverityText: r.SeverityText, Body: r.Body,
			ResourceAttrs: r.ResourceAttrs, LogAttrs: r.LogAttrs,
		}
	}
	return out
}

func LogsFromWire(wire []LogRecordV1) []logs.LogRecord {
	out := make([]logs.LogRecord, len(wire))
	for i, w := range wire {
		out[i] = logs.LogRecord{
			Time: w.Time, OrgID: w.OrgID, ServiceName: w.ServiceName, TraceID: w.TraceID, SpanID: w.SpanID,
			SeverityNum: w.SeverityNum, SeverityText: w.SeverityText, Body: w.Body,
			ResourceAttrs: w.ResourceAttrs, LogAttrs: w.LogAttrs,
		}
	}
	return out
}

func MetricsToWire(points []metrics.Metric, buckets []metrics.HistogramBucket) MetricBatchV1 {
	out := MetricBatchV1{Metrics: make([]MetricV1, len(points)), Buckets: make([]HistogramBucketV1, len(buckets))}
	for i, m := range points {
		out.Metrics[i] = MetricV1{Time: m.Time, OrgID: m.OrgID, MetricName: m.MetricName, MetricType: string(m.MetricType), Value: m.Value, Labels: m.Labels}
	}
	for i, b := range buckets {
		out.Buckets[i] = HistogramBucketV1{Time: b.Time, OrgID: b.OrgID, MetricName: b.MetricName, Labels: b.Labels, Le: b.Le, Count: b.Count}
	}
	return out
}

func MetricsFromWire(w MetricBatchV1) metrics.WriteBatch {
	out := metrics.WriteBatch{Metrics: make([]metrics.Metric, len(w.Metrics)), Buckets: make([]metrics.HistogramBucket, len(w.Buckets))}
	for i, m := range w.Metrics {
		out.Metrics[i] = metrics.Metric{Time: m.Time, OrgID: m.OrgID, MetricName: m.MetricName, MetricType: metrics.MetricType(m.MetricType), Value: m.Value, Labels: m.Labels}
	}
	for i, b := range w.Buckets {
		out.Buckets[i] = metrics.HistogramBucket{Time: b.Time, OrgID: b.OrgID, MetricName: b.MetricName, Labels: b.Labels, Le: b.Le, Count: b.Count}
	}
	return out
}

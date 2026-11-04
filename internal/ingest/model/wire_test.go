package model

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

// Messages written before the wire types existed must still decode.
func TestMetricBatchV1DecodesLegacyPayload(t *testing.T) {
	legacy, err := json.Marshal(metrics.WriteBatch{
		Metrics: []metrics.Metric{{MetricName: "cpu", Value: 1.5, MetricType: metrics.MetricTypeGauge}},
		Buckets: []metrics.HistogramBucket{{MetricName: "lat", Le: 100, Count: 3}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var w MetricBatchV1
	if err := json.Unmarshal(legacy, &w); err != nil {
		t.Fatalf("decode legacy batch: %v", err)
	}
	got := MetricsFromWire(w)
	if len(got.Metrics) != 1 || got.Metrics[0].Value != 1.5 || len(got.Buckets) != 1 || got.Buckets[0].Count != 3 {
		t.Fatalf("MetricsFromWire(legacy) = %+v", got)
	}
}

func TestSpansRoundTrip(t *testing.T) {
	in := []traces.Span{{
		SpanID: "s1", TraceID: "t1", OrgID: uuid.New(), DurationNS: 2_500_000,
		StartTime: time.Unix(10, 0).UTC(), Events: []traces.SpanEvent{{Name: "e", Time: 5}},
	}}
	raw, err := json.Marshal(SpansToWire(in))
	if err != nil {
		t.Fatal(err)
	}
	var w []SpanV1
	if err := json.Unmarshal(raw, &w); err != nil {
		t.Fatal(err)
	}
	out := SpansFromWire(w)
	if out[0].SpanID != "s1" || out[0].DurationMS != 2.5 || len(out[0].Events) != 1 {
		t.Fatalf("round trip = %+v", out[0])
	}
}

func TestHistogramBucketV1RoundTripsInfinity(t *testing.T) {
	buckets := []HistogramBucketV1{{MetricName: "latency", Le: 100, Count: 2}, {MetricName: "latency", Le: math.Inf(1), Count: 1}}

	b, err := json.Marshal(buckets)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var got []HistogramBucketV1
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got[0].Le != 100 || !math.IsInf(got[1].Le, 1) || got[1].Count != 1 || got[1].MetricName != "latency" {
		t.Errorf("round trip = %+v, want le 100 and +Inf", got)
	}
}

package metrics

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestHistogramBucketJSONEncodesInfinityAsNull(t *testing.T) {
	resp := HistogramResponse{MetricName: "latency", Buckets: []HistogramBucket{
		{MetricName: "latency", Le: 100, Count: 3},
		{MetricName: "latency", Le: math.Inf(1), Count: 1},
	}}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `"le":100`) || !strings.Contains(got, `"le":null`) {
		t.Errorf("Marshal() = %s, want le 100 and le null", got)
	}
	if strings.Count(got, `"le"`) != 2 {
		t.Errorf("Marshal() = %s, want exactly one le per bucket", got)
	}
}

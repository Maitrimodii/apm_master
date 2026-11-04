package metrics

import (
	"testing"
	"time"
)

func TestSelectResolution(testRunner *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		to   time.Time
		want Resolution
	}{
		{name: "raw", to: from.Add(time.Hour), want: ResolutionRaw},
		{name: "one minute", to: from.Add(2 * time.Hour), want: Resolution1m},
		{name: "five minute", to: from.Add(48 * time.Hour), want: Resolution5m},
		{name: "one hour", to: from.Add(8 * 24 * time.Hour), want: Resolution1h},
	}

	for _, testCases := range tests {
		testRunner.Run(testCases.name, func(testRunner *testing.T) {
			if got := selectResolution(from, testCases.to); got != testCases.want {
				testRunner.Fatalf("selectResolution() = %q, want %q", got, testCases.want)
			}
		})
	}
}

func TestSeriesSource(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if r, src := seriesSource("", from, from.Add(30*time.Minute)); r != ResolutionRaw || src.table != "metrics" || !src.raw {
		t.Errorf("seriesSource(30m range) = (%q, %+v), want raw metrics", r, src)
	}
	if r, src := seriesSource("5m", from, from.Add(time.Hour)); r != Resolution5m || src.table != "metrics_5m" {
		t.Errorf("seriesSource(interval=5m) = (%q, %+v), want metrics_5m", r, src)
	}
	if _, src := seriesSource("30m", from, from.Add(time.Hour)); src.bucket != "30 minutes" || !src.raw {
		t.Errorf("seriesSource(interval=30m) = %+v, want raw 30-minute buckets", src)
	}
	if r, _ := seriesSource("bogus", from, from.Add(48*time.Hour)); r != Resolution5m {
		t.Errorf("seriesSource(unknown interval, 48h) = %q, want 5m", r)
	}
}

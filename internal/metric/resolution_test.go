package metric

import (
	"testing"
	"time"
)

func TestSelectResolution(testRunner *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		timeout time.Time
		want    Resolution
	}{
		{name: "raw", timeout: from.Add(time.Hour), want: ResolutionRaw},
		{name: "one minute", timeout: from.Add(2 * time.Hour), want: Resolution1m},
		{name: "five minute", timeout: from.Add(48 * time.Hour), want: Resolution5m},
		{name: "one hour", timeout: from.Add(8 * 24 * time.Hour), want: Resolution1h},
	}

	for _, testCases := range tests {
		testRunner.Run(testCases.name, func(testRunner *testing.T) {
			if got := selectResolution(from, testCases.timeout); got != testCases.want {
				testRunner.Fatalf("selectResolution() = %q, want %q", got, testCases.want)
			}
		})
	}
}

func TestResolutionMappings(testRunner *testing.T) {
	if got := resolutionTable(Resolution1m); got != "metrics_1m" {
		testRunner.Fatalf("resolutionTable(1m) = %q, want metrics_1m", got)
	}
	if got := resolutionTable(Resolution("unknown")); got != "metrics" {
		testRunner.Fatalf("resolutionTable(unknown) = %q, want metrics", got)
	}
	if got := resolutionBucket(ResolutionRaw); got != "10 seconds" {
		testRunner.Fatalf("resolutionBucket(raw) = %q, want 10 seconds", got)
	}
	if got := resolutionBucket(Resolution1h); got != "1 hour" {
		testRunner.Fatalf("resolutionBucket(1h) = %q, want 1 hour", got)
	}
}

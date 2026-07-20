package metric

import "time"

// selectResolution chooses the appropriate continuous aggregate table
// based on the requested time range. This mirrors how every production
// time-series platform works — Datadog, Prometheus, InfluxDB all do this.
//
// Thresholds:
//
//	<= 1 hour   → raw metrics table    (full fidelity, short window)
//	<= 24 hours → metrics_1m           (1-minute buckets)
//	<= 7 days   → metrics_5m           (5-minute buckets)
//	>  7 days   → metrics_1h           (1-hour buckets)
func selectResolution(from, timeout time.Time) Resolution {
	d := timeout.Sub(from)
	switch {
	case d <= time.Hour:
		return ResolutionRaw
	case d <= 24*time.Hour:
		return Resolution1m
	case d <= 7*24*time.Hour:
		return Resolution5m
	default:
		return Resolution1h
	}
}

// resolutionTable maps a Resolution to its SQL table or view name.
func resolutionTable(resolution Resolution) string {
	switch resolution {
	case Resolution1m:
		return "metrics_1m"
	case Resolution5m:
		return "metrics_5m"
	case Resolution1h:
		return "metrics_1h"
	default:
		return "metrics"
	}
}

// resolutionBucket returns the time_bucket interval string for a given resolution.
// Used when querying the raw table — aggregates stay pre-bucketed in the views.
func resolutionBucket(resolution Resolution) string {
	switch resolution {
	case Resolution1m:
		return "1 minute"
	case Resolution5m:
		return "5 minutes"
	case Resolution1h:
		return "1 hour"
	default:
		return "10 seconds" // raw: fine-grained bucket for display
	}
}

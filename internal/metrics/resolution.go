package metrics

import "time"

// resolutionSpec is where a resolution's data lives and how it is bucketed.
// Raw data is bucketed on the fly; the others are continuous aggregates.
type resolutionSpec struct {
	table  string
	bucket string
	raw    bool
}

var resolutions = map[Resolution]resolutionSpec{
	ResolutionRaw: {table: "metrics", bucket: "10 seconds", raw: true},
	Resolution1m:  {table: "metrics_1m", bucket: "1 minute"},
	Resolution5m:  {table: "metrics_5m", bucket: "5 minutes"},
	Resolution1h:  {table: "metrics_1h", bucket: "1 hour"},
}

// intervals maps the ?interval= values the API accepts to a source and
// bucket size. Intervals without a matching aggregate re-bucket raw data.
var intervals = map[string]struct {
	resolution Resolution
	spec       resolutionSpec
}{
	"1m":  {Resolution1m, resolutions[Resolution1m]},
	"5m":  {Resolution5m, resolutions[Resolution5m]},
	"15m": {ResolutionRaw, resolutionSpec{table: "metrics", bucket: "15 minutes", raw: true}},
	"30m": {ResolutionRaw, resolutionSpec{table: "metrics", bucket: "30 minutes", raw: true}},
	"1h":  {Resolution1h, resolutions[Resolution1h]},
	"24h": {ResolutionRaw, resolutionSpec{table: "metrics", bucket: "24 hours", raw: true}},
}

// selectResolution picks the coarsest source that still gives a useful
// number of points for the requested range:
//
//	<= 1 hour   → raw metrics table
//	<= 24 hours → metrics_1m
//	<= 7 days   → metrics_5m
//	>  7 days   → metrics_1h
func selectResolution(from, to time.Time) Resolution {
	d := to.Sub(from)
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

// seriesSource resolves the table and bucket for a series query: an explicit
// known interval wins, otherwise the range decides.
func seriesSource(interval string, from, to time.Time) (Resolution, resolutionSpec) {
	if iv, ok := intervals[interval]; ok {
		return iv.resolution, iv.spec
	}
	r := selectResolution(from, to)
	return r, resolutions[r]
}

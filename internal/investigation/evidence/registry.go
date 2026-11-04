package evidence

// Readers are the telemetry stores the executors read from.
type Readers struct {
	Metrics metricAggregator
	Traces  interface {
		traceSummaryLister
		traceGetter
	}
	Logs logLister
}

// NewRegistry registers one executor per query type.
func NewRegistry(r Readers) Registry {
	return Registry{
		MetricBaseline: metricBaseline{metrics: r.Metrics},
		SlowTraces:     slowTraces{traces: r.Traces},
		TraceDetails:   traceDetails{traces: r.Traces},
		CorrelatedLogs: correlatedLogs{logs: r.Logs},
	}
}

package otlp

type IngestRequest struct {
	ResourceSpans []ResourceSpans `json:"resourceSpans" binding:"required,min=1"`
}

type ResourceSpans struct {
	Resource   Resource     `json:"resource"`
	ScopeSpans []ScopeSpans `json:"scopeSpans" binding:"required,min=1"`
}

type Resource struct {
	Attributes map[string]any `json:"attributes"`
}

type ScopeSpans struct {
	Spans []SpanDTO `json:"spans" binding:"required,min=1"`
}

type SpanDTO struct {
	SpanID            string         `json:"spanId"             binding:"required"`
	TraceID           string         `json:"traceId"            binding:"required"`
	ParentSpanID      string         `json:"parentSpanId"`
	Name              string         `json:"name"               binding:"required"`
	Kind              int            `json:"kind"`
	StartTimeUnixNano int64          `json:"startTimeUnixNano"  binding:"required"`
	EndTimeUnixNano   int64          `json:"endTimeUnixNano"    binding:"required"`
	Attributes        map[string]any `json:"attributes"`
	Status            SpanStatusDTO  `json:"status"`
	Events            []SpanEventDTO `json:"events"`
	Links             []SpanLinkDTO  `json:"links"`
}

type SpanStatusDTO struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SpanEventDTO struct {
	Name         string         `json:"name"`
	TimeUnixNano int64          `json:"timeUnixNano"`
	Attributes   map[string]any `json:"attributes"`
}

type SpanLinkDTO struct {
	TraceID    string         `json:"traceId"`
	SpanID     string         `json:"spanId"`
	TraceState string         `json:"traceState"`
	Attributes map[string]any `json:"attributes"`
}

type IngestResponse struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
}

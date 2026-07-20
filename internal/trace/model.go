package trace

import (
	"time"

	"github.com/google/uuid"
)

type Span struct {
	SpanID   string  `json:"span_id"`
	TraceID  string  `json:"trace_id"`
	ParentID *string `json:"parent_id,omitempty"`

	OrgID       uuid.UUID `json:"org_id"`
	ServiceName string    `json:"service_name"`
	SpanName    string    `json:"span_name"`
	SpanKind    string    `json:"span_kind"`

	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	DurationNS int64     `json:"duration_ns"` // store nanoseconds — source of truth
	DurationMS float64   `json:"duration_ms"` // computed, for API consumers

	StatusCode    string  `json:"status_code"`
	StatusMessage *string `json:"status_message,omitempty"`

	ResourceAttrs map[string]any `json:"resource_attrs"`
	SpanAttrs     map[string]any `json:"span_attrs"`
	Events        []SpanEvent    `json:"events"`
	Links         []SpanLink     `json:"links"`
}

type SpanEvent struct {
	Name  string         `json:"name"`
	Time  int64          `json:"time_unix_nano"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

type SpanLink struct {
	TraceID    string         `json:"trace_id"`
	SpanID     string         `json:"span_id"`
	TraceState string         `json:"trace_state,omitempty"`
	Attrs      map[string]any `json:"attrs,omitempty"`
}

type TraceSummary struct {
	TraceID          string  `json:"trace_id"`
	ServiceName      string  `json:"service_name"`
	Endpoint         string  `json:"endpoint"`
	DurationMS       float64 `json:"duration_ms"`
	Status           string  `json:"status"`
	SpanCount        int     `json:"span_count"`
	ErrorCount       int     `json:"error_count"`
	DBTimeMS         float64 `json:"db_time_ms"`
	MiddlewareTimeMS float64 `json:"middleware_time_ms"`
}

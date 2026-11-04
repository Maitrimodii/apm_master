package logs

import (
	"time"

	"github.com/google/uuid"
)

type LogRecord struct {
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

// ListFilter selects log records for one org. Zero From/To mean unbounded.
type ListFilter struct {
	OrgID       uuid.UUID
	From        time.Time
	To          time.Time
	ServiceName string
	MinSeverity int
	TraceID     string
	Search      string
	Limit       int
	Offset      int
}

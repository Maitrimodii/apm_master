package otlp

type IngestLogsRequest struct {
	ResourceLogs []ResourceLogs `json:"resourceLogs" binding:"required,min=1"`
}

type ResourceLogs struct {
	Resource  Resource    `json:"resource"`
	ScopeLogs []ScopeLogs `json:"scopeLogs" binding:"required,min=1"`
}

type ScopeLogs struct {
	LogRecords []LogRecordDTO `json:"logRecords" binding:"required,min=1"`
}

type LogRecordDTO struct {
	TimeUnixNano         int64          `json:"timeUnixNano"`
	ObservedTimeUnixNano int64          `json:"observedTimeUnixNano"`
	SeverityNumber       int16          `json:"severityNumber"`
	SeverityText         string         `json:"severityText"`
	Body                 string         `json:"body"`
	Attributes           map[string]any `json:"attributes"`
	TraceID              string         `json:"traceId"`
	SpanID               string         `json:"spanId"`
}

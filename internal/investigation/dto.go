package investigation

import (
	"time"

	"github.com/google/uuid"
)

// Request & Response DTOs

type CreateInvestigationRequest struct {
	TimeRangeStart *time.Time `json:"time_range_start"`
	TimeRangeEnd   *time.Time `json:"time_range_end"`
	Focus          []string   `json:"focus"`
	IdempotencyKey *string    `json:"idempotency_key"`
}

type EvidenceQueryRequest struct {
	InvestigationID uuid.UUID      `json:"investigation_id" binding:"required"`
	TenantID        uuid.UUID      `json:"tenant_id" binding:"required"`
	QueryType       string         `json:"query_type" binding:"required"`
	TimeRangeStart  time.Time      `json:"time_range_start" binding:"required"`
	TimeRangeEnd    time.Time      `json:"time_range_end" binding:"required"`
	Parameters      map[string]any `json:"parameters"`
}

type EvidenceQueryResponse struct {
	EvidenceID          uuid.UUID      `json:"evidence_id"`
	QueryType           string         `json:"query_type"`
	Result              any            `json:"result"`
	Provenance          map[string]any `json:"provenance"`
	DataQualityWarnings []string       `json:"data_quality_warnings"`
}

type UpdateStatusRequest struct {
	Status         InvestigationStatus `json:"status" binding:"required"`
	FailureCode    *string             `json:"failure_code,omitempty"`
	FailureMessage *string             `json:"failure_message,omitempty"`
}

type SaveReportRequest struct {
	SchemaVersion    string         `json:"schema_version" binding:"required"`
	ReportJSON       map[string]any `json:"report_json" binding:"required"`
	ValidationStatus string         `json:"validation_status" binding:"required"`
}

type AppendEventRequest struct {
	SequenceNumber int            `json:"sequence_number"`
	EventType      string         `json:"event_type" binding:"required"`
	Summary        string         `json:"summary" binding:"required"`
	Payload        map[string]any `json:"payload"`
}

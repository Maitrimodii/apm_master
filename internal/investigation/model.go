package investigation

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTransition     = errors.New("invalid status transition")
	ErrInvestigationNotFound = errors.New("investigation not found")
	ErrTimeWindowInvalid     = errors.New("time window start must be before end")
	ErrTimeWindowTooLarge    = errors.New("time window cannot exceed 24 hours")
	ErrIncidentNotFound      = errors.New("incident not found or access denied")
	ErrCannotCancelTerminal  = errors.New("cannot cancel terminal investigation")
	ErrCannotRetryActive     = errors.New("can only retry investigations in a terminal state")
)

type Investigation struct {
	ID                    uuid.UUID           `json:"id"`
	OrgID                 uuid.UUID           `json:"org_id"`
	IncidentID            uuid.UUID           `json:"incident_id"`
	RequestedBy           uuid.UUID           `json:"requested_by"`
	Status                InvestigationStatus `json:"status"`
	TimeRangeStart        time.Time           `json:"time_range_start"`
	TimeRangeEnd          time.Time           `json:"time_range_end"`
	Focus                 []string            `json:"focus"`
	CorrelationID         uuid.UUID           `json:"correlation_id"`
	IdempotencyKey        *string             `json:"idempotency_key,omitempty"`
	ParentInvestigationID *uuid.UUID          `json:"parent_investigation_id,omitempty"`
	CancelRequestedAt     *time.Time          `json:"cancel_requested_at,omitempty"`
	CreatedAt             time.Time           `json:"created_at"`
	StartedAt             *time.Time          `json:"started_at,omitempty"`
	CompletedAt           *time.Time          `json:"completed_at,omitempty"`
	FailureCode           *string             `json:"failure_code,omitempty"`
	FailureMessage        *string             `json:"failure_message,omitempty"`
}

type OutboxEvent struct {
	ID              uuid.UUID  `json:"id"`
	InvestigationID uuid.UUID  `json:"investigation_id"`
	CorrelationID   uuid.UUID  `json:"correlation_id"`
	Payload         []byte     `json:"payload"`
	CreatedAt       time.Time  `json:"created_at"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
}

type InvestigationJobEvent struct {
	EventID         uuid.UUID `json:"event_id"`
	SchemaVersion   string    `json:"schema_version"`
	EventType       string    `json:"event_type"`
	CorrelationID   uuid.UUID `json:"correlation_id"`
	InvestigationID uuid.UUID `json:"investigation_id"`
	IncidentID      uuid.UUID `json:"incident_id"`
	TenantID        uuid.UUID `json:"tenant_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type InvestigationEvidence struct {
	ID                  uuid.UUID      `json:"id"`
	InvestigationID      uuid.UUID      `json:"investigation_id"`
	EvidenceType        string         `json:"evidence_type"`
	SourceService       string         `json:"source_service"`
	QueryParameters     map[string]any `json:"query_parameters"`
	Summary             string         `json:"summary"`
	Result              any            `json:"result"`
	Provenance          map[string]any `json:"provenance"`
	DataQualityWarnings []string       `json:"data_quality_warnings"`
	CreatedAt           time.Time      `json:"created_at"`
}

type InvestigationReportRecord struct {
	ID               uuid.UUID      `json:"id"`
	InvestigationID  uuid.UUID      `json:"investigation_id"`
	SchemaVersion    string         `json:"schema_version"`
	ReportJSON       map[string]any `json:"report_json"`
	ValidationStatus string         `json:"validation_status"`
	CreatedAt        time.Time      `json:"created_at"`
}

type InvestigationEvent struct {
	ID              uuid.UUID      `json:"id"`
	InvestigationID uuid.UUID      `json:"investigation_id"`
	SequenceNumber  int            `json:"sequence_number"`
	EventType       string         `json:"event_type"`
	Summary         string         `json:"summary"`
	Payload         map[string]any `json:"payload"`
	CreatedAt       time.Time      `json:"created_at"`
}

type IncidentContext struct {
	IncidentID  uuid.UUID  `json:"incident_id"`
	AlertID     uuid.UUID  `json:"alert_id"`
	OrgID       uuid.UUID  `json:"org_id"`
	AlertName   string     `json:"alert_name"`
	Severity    string     `json:"severity"`
	Condition   string     `json:"condition"`
	ServiceName *string    `json:"service_name,omitempty"`
	TriggeredAt time.Time  `json:"triggered_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	Value       float64    `json:"value"`
	Threshold   float64    `json:"threshold"`
	Message     string     `json:"message"`
}

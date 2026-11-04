package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// SourceHTTPIngest marks events produced by the HTTP ingest API.
const SourceHTTPIngest = "http-ingest"

const (
	SchemaVersion = 1

	SignalTraces  Signal = "traces"
	SignalLogs    Signal = "logs"
	SignalMetrics Signal = "metrics"
)

type Signal string

// Event is the Kafka envelope for all telemetry signals.
// Payload contains a signal-specific JSON document so the envelope can evolve
// independently from trace, log, and metric storage models.
type Event struct {
	SchemaVersion int             `json:"schema_version"`
	Signal        Signal          `json:"signal"`
	EventID       uuid.UUID       `json:"event_id"`
	OrgID         uuid.UUID       `json:"org_id"`
	TraceID       string          `json:"trace_id,omitempty"`
	ServiceName   string          `json:"service_name,omitempty"`
	ProducedAt    time.Time       `json:"produced_at"`
	Metadata      Metadata        `json:"metadata,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

type Metadata struct {
	AcceptedRecords int               `json:"accepted_records"`
	RejectedRecords int               `json:"rejected_records"`
	Source          string            `json:"source"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

type DeadLetterEvent struct {
	SchemaVersion int       `json:"schema_version"`
	EventID       uuid.UUID `json:"event_id"`
	Signal        Signal    `json:"signal"`
	OrgID         uuid.UUID `json:"org_id,omitempty"`
	Topic         string    `json:"topic"`
	Partition     int       `json:"partition"`
	Offset        int64     `json:"offset"`
	Reason        string    `json:"reason"`
	Payload       []byte    `json:"payload"`
	FailedAt      time.Time `json:"failed_at"`
}

func NewEvent(signal Signal, orgID uuid.UUID, accepted int, rejected int, payload json.RawMessage) Event {
	return Event{
		SchemaVersion: SchemaVersion,
		Signal:        signal,
		EventID:       uuid.New(),
		OrgID:         orgID,
		ProducedAt:    time.Now().UTC(),
		Metadata: Metadata{
			AcceptedRecords: accepted,
			RejectedRecords: rejected,
			Source:          SourceHTTPIngest,
		},
		Payload: payload,
	}
}

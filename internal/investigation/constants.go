package investigation

import "time"

const (
	// MaxTimeWindow is the maximum permissible analysis window for an investigation.
	MaxTimeWindow = 24 * time.Hour

	// MaxConcurrentPerTenantDefault is the maximum number of concurrent running investigations per tenant.
	MaxConcurrentPerTenantDefault = 5

	// DefaultWindowPadding is the default buffer added on either side of the trigger timestamp.
	DefaultWindowPadding = 15 * time.Minute
)

// Contract with apm-ai-service for the Kafka job event.
const (
	// JobSchemaVersion is the InvestigationJobEvent schema_version.
	JobSchemaVersion = "1.0"
	// JobEventRequested is the event_type of a new or retried investigation.
	JobEventRequested = "investigation.requested"
	// KafkaHeaderCorrelationID carries the investigation's correlation ID.
	KafkaHeaderCorrelationID = "correlation_id"
)

// evidenceQueryTimeout bounds one evidence query against telemetry stores.
const evidenceQueryTimeout = 5 * time.Second

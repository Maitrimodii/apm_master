package notification

import (
	"time"

	"github.com/google/uuid"
)

// Channel is where a notification is delivered. Email jobs name their
// provider by channel.
type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelInApp Channel = "in_app"
)

// Status is the outcome recorded on a notification_events row.
type Status string

const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
)

// JobState is the lifecycle state of a notification_jobs row.
type JobState string

const (
	JobPending    JobState = "pending"
	JobProcessing JobState = "processing"
	JobSent       JobState = "sent"
	JobDeadLetter JobState = "dead_letter"
)

// EventType names what happened. It selects the email layout.
type EventType string

const (
	EventAlertCreated           EventType = "alert_created"
	EventAlertShared            EventType = "alert_shared"
	EventAlertFiring            EventType = "alert_firing"
	EventAlertAcknowledged      EventType = "alert_acknowledged"
	EventAlertResolved          EventType = "alert_resolved"
	EventInviteCreated          EventType = "invite_created"
	EventPasswordResetRequested EventType = "password_reset_requested"
)

// Job priorities. Workers take higher priorities first.
const (
	PriorityNormal = 0  // transactional mail: invites, password resets, shares
	PriorityHigh   = 10 // alert state transitions
)

// DefaultMaxAttempts is how often a job is tried before it is dead-lettered.
const DefaultMaxAttempts = 5

type Event struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	UserID    *uuid.UUID
	EventType EventType
	Channel   Channel
	Recipient string
	Subject   string
	Body      string
	Status    Status
	Error     *string
	Metadata  []byte
	CreatedAt time.Time
	SentAt    *time.Time
}

// Task is a request to deliver one message.
type Task struct {
	OrgID       uuid.UUID
	UserID      *uuid.UUID
	EventType   EventType
	Provider    Channel
	Recipient   string
	Subject     string
	Body        string
	ReferenceID string
	Metadata    map[string]string
	Priority    int
}

// Job is a persisted Task with its delivery state.
type Job struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	UserID      *uuid.UUID
	EventType   EventType
	Provider    Channel
	Recipient   string
	Subject     string
	Body        string
	ReferenceID string
	Metadata    map[string]string
	State       JobState
	Attempts    int
	MaxAttempts int
	RunAt       time.Time
	LastError   *string
	Priority    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

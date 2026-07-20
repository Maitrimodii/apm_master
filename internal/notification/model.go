package notification

import (
	"time"

	"github.com/google/uuid"
)

type Channel string

const (
	ChannelEmail Channel = "email"
	ChannelInApp Channel = "in_app"
)

type Status string

const (
	StatusPending Status = "pending"
	StatusSent    Status = "sent"
	StatusFailed  Status = "failed"
)

type Event struct {
	ID        uuid.UUID
	OrgID     uuid.UUID
	UserID    *uuid.UUID
	EventType string
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

type NotificationJob struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	UserID      *uuid.UUID
	EventType   string
	Provider    string
	Recipient   string
	Subject     string
	Body        string
	ReferenceID string
	Metadata    map[string]string
	State       string
	Attempts    int
	MaxAttempts int
	RunAt       time.Time
	LastError   *string
	Priority    int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

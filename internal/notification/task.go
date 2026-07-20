package notification

import "github.com/google/uuid"

type NotificationTask struct {
	OrgID       uuid.UUID         `json:"org_id"`
	UserID      *uuid.UUID        `json:"user_id,omitempty"`
	EventType   string            `json:"event_type"`
	Provider    string            `json:"provider"`
	Recipient   string            `json:"recipient"`
	Subject     string            `json:"subject"`
	Body        string            `json:"body"`
	ReferenceID string            `json:"reference_id,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Priority    int               `json:"priority,omitempty"`
}

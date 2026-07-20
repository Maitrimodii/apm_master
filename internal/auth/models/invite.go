package models

import (
	"time"

	"github.com/google/uuid"
)

type Invite struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	InvitedBy  uuid.UUID
	FirstName  string
	LastName   string
	Email      string
	TokenHash  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

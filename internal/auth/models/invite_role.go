package models

import (
	"time"

	"github.com/google/uuid"
)

type InviteRole struct {
	RoleID    uuid.UUID
	InviteID  uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

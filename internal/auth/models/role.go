package models

import (
	"time"

	"github.com/google/uuid"
)

type Role struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Name        string
	Description *string
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

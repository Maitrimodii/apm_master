package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID
	OrgID           uuid.UUID
	IsEmailVerified bool
	FirstName       string
	LastName        string
	Email           string
	PasswordHash    *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

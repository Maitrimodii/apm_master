package models

import (
	"time"

	"github.com/google/uuid"
)

type APIKey struct {
	ID         uuid.UUID
	OrgID      uuid.UUID
	Key        string
	Name       string
	KeyPrefix  string
	KeyHash    string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
	UpdatedAt  time.Time
}

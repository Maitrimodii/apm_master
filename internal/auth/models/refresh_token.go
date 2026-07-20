package models

import (
	"net"
	"time"

	"github.com/google/uuid"
)

type RefreshToken struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	UserAgent  *string
	IPAddress  *net.IP
	DeviceName *string
	ExpiresAt  time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
	RevokedAt  *time.Time
	UpdatedAt  time.Time
}

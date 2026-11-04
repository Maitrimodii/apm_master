package auth

import (
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/authz"
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

type InviteRole struct {
	RoleID    uuid.UUID
	InviteID  uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Org struct {
	ID        uuid.UUID
	Name      string
	Slug      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PasswordResetToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

type Permission struct {
	ID          uuid.UUID
	Name        string
	Action      authz.Action
	Description string
}

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

type Role struct {
	ID          uuid.UUID
	OrgID       uuid.UUID
	Name        string
	Description *string
	IsSystem    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type RolePermission struct {
	RoleID       uuid.UUID
	PermissionID uuid.UUID
}

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

type UserRole struct {
	UserID uuid.UUID
	RoleID uuid.UUID
}

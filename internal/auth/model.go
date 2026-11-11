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
	// RevokedAt is set when an administrator cancels the invite.
	RevokedAt *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// InviteStatus is where an invite is in its lifecycle. It is derived from the
// invite's timestamps, never stored.
type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusRevoked  InviteStatus = "revoked"
	InviteStatusExpired  InviteStatus = "expired"
)

// Status derives the invite's status. Accepted and revoked win over expired:
// they are final, while expiry is only time passing.
func (i Invite) Status(now time.Time) InviteStatus {
	switch {
	case i.AcceptedAt != nil:
		return InviteStatusAccepted
	case i.RevokedAt != nil:
		return InviteStatusRevoked
	case !i.ExpiresAt.After(now):
		return InviteStatusExpired
	default:
		return InviteStatusPending
	}
}

// RoleSummary names a role without its permissions.
type RoleSummary struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
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
	// FamilyID is shared by every token of one login session: rotation copies
	// it from the parent token.
	FamilyID uuid.UUID
	// SessionExpiresAt is the session's absolute end. Nil on tokens created
	// before sessions existed; see SessionEnd.
	SessionExpiresAt *time.Time
}

// SessionEnd is when the token's session must end. Tokens without a session
// expiry (created before it existed) end with the token itself.
func (t RefreshToken) SessionEnd() time.Time {
	if t.SessionExpiresAt != nil {
		return *t.SessionExpiresAt
	}
	return t.ExpiresAt
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

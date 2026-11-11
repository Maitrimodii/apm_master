package auth

import (
	"time"

	"github.com/Maitrimodii/apm_master/internal/authz"

	"github.com/google/uuid"
)

type RegisterOrgOwnerRequest struct {
	OrgName   string `json:"org_name" binding:"required"`
	OrgSlug   string `json:"org_slug" binding:"required"`
	FirstName string `json:"first_name" binding:"required"`
	LastName  string `json:"last_name" binding:"required"`
	Email     string `json:"email" binding:"required,email"`
	Password  string `json:"password" binding:"required,min=8"`
}

type RegisterOrgOwnerResponse struct {
	User UserResponse `json:"user"`
	Org  OrgResponse  `json:"org"`
}

type UserResponse struct {
	ID              uuid.UUID `json:"id"`
	OrgID           uuid.UUID `json:"org_id"`
	IsEmailVerified bool      `json:"is_email_verified"`
	FirstName       string    `json:"first_name"`
	LastName        string    `json:"last_name"`
	Email           string    `json:"email"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type OrgResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func NewUserResponse(user User) UserResponse {
	return UserResponse{
		ID:              user.ID,
		OrgID:           user.OrgID,
		IsEmailVerified: user.IsEmailVerified,
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		Email:           user.Email,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}
}

func NewOrgResponse(org Org) OrgResponse {
	return OrgResponse{
		ID:        org.ID,
		Name:      org.Name,
		Slug:      org.Slug,
		CreatedAt: org.CreatedAt,
		UpdatedAt: org.UpdatedAt,
	}
}

type LoginRequest struct {
	OrgSlug   string  `json:"org_slug" binding:"required"`
	Email     string  `json:"email" binding:"required,email"`
	Password  string  `json:"password" binding:"required,min=8"`
	UserAgent *string `json:"-"`
	IPAddress *string `json:"-"`
}

type LoginResponse struct {
	User         UserResponse `json:"user"`
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
}

type RefreshTokenRequest struct {
	RefreshToken string  `json:"refresh_token" binding:"required"`
	UserAgent    *string `json:"-"`
	IPAddress    *string `json:"-"`
}

type LogoutRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type ForgotPasswordRequest struct {
	OrgSlug string `json:"org_slug" binding:"required"`
	Email   string `json:"email" binding:"required,email"`
}

type ForgotPasswordResponse struct {
	Message string `json:"message"`
}

type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

type ResetPasswordResponse struct {
	Message string `json:"message"`
}

type MeRequest struct {
	UserID uuid.UUID `json:"user_id"`
	OrgID  uuid.UUID `json:"org_id"`
}

// MeResponse is the signed-in user and the permissions they hold in their
// org, which the UI uses to decide what to show. The API still checks every
// request itself.
type MeResponse struct {
	User        UserResponse      `json:"user"`
	Permissions []PermissionGrant `json:"permissions"`
}

type InviteUserRequest struct {
	FirstName string      `json:"first_name" binding:"required"`
	LastName  string      `json:"last_name" binding:"required"`
	Email     string      `json:"email" binding:"required,email"`
	RoleIDs   []uuid.UUID `json:"role_ids" binding:"required"`
	UserID    uuid.UUID   `json:"-"`
	OrgID     uuid.UUID   `json:"-"`
}

// InviteUserResponse confirms an invite was sent. It never carries the invite
// token: the token is delivered only by email, so whoever creates or resends
// an invite cannot accept it in the invitee's name.
type InviteUserResponse struct {
	Message   string    `json:"message"`
	InviteID  uuid.UUID `json:"invite_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// InviteInviter identifies who sent an invite.
type InviteInviter struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

// InviteResponse is an invite as its org's administrators see it.
type InviteResponse struct {
	ID         uuid.UUID     `json:"id"`
	Email      string        `json:"email"`
	FirstName  string        `json:"first_name"`
	LastName   string        `json:"last_name"`
	Status     InviteStatus  `json:"status"`
	Roles      []RoleSummary `json:"roles"`
	InvitedBy  InviteInviter `json:"invited_by"`
	CreatedAt  time.Time     `json:"created_at"`
	ExpiresAt  time.Time     `json:"expires_at"`
	AcceptedAt *time.Time    `json:"accepted_at,omitempty"`
	RevokedAt  *time.Time    `json:"revoked_at,omitempty"`
}

func NewInviteResponse(item InviteListItem) InviteResponse {
	return InviteResponse{
		ID:         item.ID,
		Email:      item.Email,
		FirstName:  item.FirstName,
		LastName:   item.LastName,
		Status:     item.Status,
		Roles:      item.Roles,
		InvitedBy:  InviteInviter{ID: item.InvitedBy, Name: item.InvitedByName},
		CreatedAt:  item.CreatedAt,
		ExpiresAt:  item.ExpiresAt,
		AcceptedAt: item.AcceptedAt,
		RevokedAt:  item.RevokedAt,
	}
}

// PermissionGrant names one permission a user holds.
type PermissionGrant struct {
	Name   string       `json:"name"`
	Action authz.Action `json:"action"`
}

type AcceptInviteRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// GetInviteRequest is bound from the query string (?token=), so it needs a
// form tag: without one gin matches the Go field name "Token" and never finds
// the lowercase parameter.
type GetInviteRequest struct {
	Token string `form:"token" json:"token" binding:"required"`
}

type GetInviteResponse struct {
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	OrgName   string    `json:"org_name"`
	OrgSlug   string    `json:"org_slug"`
	ExpiresAt time.Time `json:"expires_at"`
}

type PermissionResponse struct {
	ID          uuid.UUID    `json:"id"`
	Name        string       `json:"name"`
	Action      authz.Action `json:"action"`
	Description string       `json:"description"`
}

type CreateRoleRequest struct {
	Name          string      `json:"name" binding:"required"`
	Description   *string     `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids"`
	OrgID         uuid.UUID   `json:"-"`
	UserID        uuid.UUID   `json:"-"`
}

type RoleResponse struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	IsSystem    bool      `json:"is_system"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GetRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
	OrgID  uuid.UUID `json:"-"`
}

type GetDetailedRoleResponse struct {
	ID          uuid.UUID            `json:"id"`
	Name        string               `json:"name"`
	Description *string              `json:"description,omitempty"`
	IsSystem    bool                 `json:"is_system"`
	Permissions []PermissionResponse `json:"permissions"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

type GetRolesRequest struct {
	OrgID uuid.UUID `json:"-"`
}

type UpdateRoleRequest struct {
	RoleID        uuid.UUID   `json:"-"`
	OrgID         uuid.UUID   `json:"-"`
	UserID        uuid.UUID   `json:"-"`
	Name          string      `json:"name" binding:"required"`
	Description   *string     `json:"description,omitempty"`
	PermissionIDs []uuid.UUID `json:"permission_ids" binding:"required"`
}

func NewRoleResponse(
	role Role,
) RoleResponse {
	return RoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}
}

type CreateAPIKeyRequest struct {
	Name  string    `json:"name" binding:"required"`
	OrgID uuid.UUID `json:"-"`
}

type CreateAPIKeyResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Key       string    `json:"key"`
	Prefix    string    `json:"prefix"`
	CreatedAt time.Time `json:"created_at"`
}

type GetAPIKeysRequest struct {
	OrgID uuid.UUID `json:"-"`
}

type APIKeyResponse struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type GetAPIKeysResponse struct {
	APIKeys []APIKeyResponse `json:"api_keys"`
}

type RevokeAPIKeyRequest struct {
	APIKeyID uuid.UUID `json:"-"`
	OrgID    uuid.UUID `json:"-"`
}

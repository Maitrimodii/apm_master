package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

type InviteService struct {
	repo         *Repository
	cfg          *config.Config
	notification *notification.Service
	issuer       *sessionIssuer
	log          *slog.Logger

	// newToken generates invite tokens. Tokens are only ever emailed, never
	// returned by the API, so tests replace it to learn the token they need.
	newToken func() (string, error)
}

func NewInviteService(repo *Repository, cfg *config.Config, notificationService *notification.Service, log *slog.Logger) *InviteService {
	if log == nil {
		log = slog.Default()
	}

	return &InviteService{
		repo:         repo,
		cfg:          cfg,
		notification: notificationService,
		issuer:       newSessionIssuer(repo, cfg),
		log:          log,
		newToken:     GenerateToken,
	}
}

// CreateInvite invites a person to the caller's org with the given roles.
//
// The roles must belong to the org and the caller must hold every permission
// they carry (see grant.go); both are checked under lock in the same
// transaction that stores the invite. The raw token goes only into the email.
func (s *InviteService) CreateInvite(ctx context.Context, input InviteUserRequest) (*InviteUserResponse, error) {
	roleIDs := uniqueIDs(input.RoleIDs)
	if len(roleIDs) == 0 {
		return nil, apperr.ValidationError("role_ids", "at least one role is required")
	}
	rawToken, err := s.newToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	var created *Invite
	err = s.repo.WithTx(ctx, func(querier Querier) error {
		scope, err := lockGrantScope(ctx, s.repo, querier, input.UserID, input.OrgID, roleIDs)
		if err != nil {
			return err
		}
		granted, err := s.repo.GetPermissionIDsOfRoles(ctx, querier, roleIDs)
		if err != nil {
			return err
		}
		if err := scope.requireGrantable(granted); err != nil {
			return err
		}

		pending, err := s.repo.GetPendingInviteByEmail(ctx, querier, input.Email, input.OrgID)
		if err != nil {
			return err
		}
		if pending != nil {
			return apperr.AlreadyExists(apperr.CodeInviteAlreadyExists, "invite already exists")
		}
		existing, err := s.repo.GetUserByEmail(ctx, querier, input.OrgID, input.Email)
		if err != nil {
			return err
		}
		if existing != nil {
			return apperr.BadRequest(apperr.CodeEmailExists, "user with this email already exists")
		}

		created, err = s.repo.CreateInvite(ctx, querier, Invite{
			OrgID:     input.OrgID,
			InvitedBy: input.UserID,
			FirstName: input.FirstName,
			LastName:  input.LastName,
			Email:     input.Email,
			TokenHash: HashToken(rawToken),
			ExpiresAt: time.Now().Add(s.cfg.Auth.InviteTTL()),
		})
		if err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if err := s.repo.AssignInviteRole(ctx, querier, InviteRole{InviteID: created.ID, RoleID: roleID}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, appError(err)
	}

	s.sendInviteEmail(ctx, created, input.UserID, rawToken)
	s.log.Info("invite created", "org_id", input.OrgID, "invite_id", created.ID, "invited_by", input.UserID)

	return &InviteUserResponse{
		Message:   "Invite created successfully",
		InviteID:  created.ID,
		ExpiresAt: created.ExpiresAt,
	}, nil
}

// GetInviteByToken tells an invitee what they were invited to. Only the
// holder of the emailed token can call it, and it returns nothing beyond what
// that email already said.
func (s *InviteService) GetInviteByToken(ctx context.Context, input GetInviteRequest) (*GetInviteResponse, error) {
	invite, err := s.repo.GetInviteByTokenHash(ctx, s.repo.Query(), HashToken(input.Token))
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if rejection := inviteUnusable(invite, time.Now()); rejection != nil {
		return nil, rejection
	}

	org, err := s.repo.GetOrgByID(ctx, s.repo.Query(), invite.OrgID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if org == nil {
		return nil, apperr.NotFound(apperr.CodeOrgNotFound, "organization not found")
	}

	return &GetInviteResponse{
		Email:     invite.Email,
		FirstName: invite.FirstName,
		LastName:  invite.LastName,
		OrgName:   org.Name,
		OrgSlug:   org.Slug,
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

// AcceptInvite creates the invited user and signs them in.
//
// The token is consumed by a conditional UPDATE inside one transaction that
// also creates the user, assigns the invite's stored roles and issues the
// session, so concurrent acceptances of one invite cannot both succeed and a
// failure at any step leaves the invite usable. The org, email and roles all
// come from the invite row; the request supplies only the token and password.
// The email is therefore marked verified: the invitee proved they received it.
func (s *InviteService) AcceptInvite(ctx context.Context, input AcceptInviteRequest) (*LoginResponse, error) {
	tokenHash := HashToken(input.Token)

	// Reject obviously unusable tokens before spending a bcrypt hash on them.
	// The transaction below makes the real decision.
	precheck, err := s.repo.GetInviteByTokenHash(ctx, s.repo.Query(), tokenHash)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if rejection := inviteUnusable(precheck, time.Now()); rejection != nil {
		return nil, rejection
	}

	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return nil, err
	}

	var (
		resp   *LoginResponse
		invite *Invite
	)
	err = s.repo.WithTx(ctx, func(querier Querier) error {
		invite, err = s.repo.ConsumeInvite(ctx, querier, tokenHash)
		if err != nil {
			return err
		}
		if invite == nil {
			// Accepted, cancelled or expired since the precheck.
			current, err := s.repo.GetInviteByTokenHash(ctx, querier, tokenHash)
			if err != nil {
				return err
			}
			if rejection := inviteUnusable(current, time.Now()); rejection != nil {
				return rejection
			}
			return errInviteExpired()
		}

		existing, err := s.repo.GetUserByEmail(ctx, querier, invite.OrgID, invite.Email)
		if err != nil {
			return err
		}
		if existing != nil {
			return apperr.BadRequest(apperr.CodeEmailExists, "user with this email is already registered in this organization")
		}

		user, err := s.repo.CreateUser(ctx, querier, User{
			OrgID:           invite.OrgID,
			FirstName:       invite.FirstName,
			LastName:        invite.LastName,
			Email:           invite.Email,
			PasswordHash:    &passwordHash,
			IsEmailVerified: true,
		})
		if err != nil {
			return err
		}

		roleIDs, err := s.repo.GetInviteRoleIDsInOrg(ctx, querier, invite.ID, invite.OrgID)
		if err != nil {
			return err
		}
		for _, roleID := range roleIDs {
			if err := s.repo.AssignRoleToUser(ctx, querier, user.ID, roleID); err != nil {
				return err
			}
		}

		resp, err = s.issuer.issue(ctx, querier, *user, clientInfo{}, nil)
		return err
	})
	if err != nil {
		return nil, appError(err)
	}

	s.log.Info("invite accepted", "org_id", invite.OrgID, "user_id", resp.User.ID, "invite_id", invite.ID)
	return resp, nil
}

// ListInvites lists the org's invites, optionally only those in one status.
func (s *InviteService) ListInvites(ctx context.Context, orgID uuid.UUID, status InviteStatus, limit, offset int) ([]InviteResponse, int, error) {
	items, total, err := s.repo.ListInvites(ctx, s.repo.Query(), orgID, status, limit, offset)
	if err != nil {
		return nil, 0, apperr.Internal(err)
	}
	resp := make([]InviteResponse, 0, len(items))
	for _, item := range items {
		resp = append(resp, NewInviteResponse(item))
	}
	return resp, total, nil
}

// ResendInvite emails a pending or expired invite again with a new token and
// a fresh expiry; the previous link stops working. Resending hands out the
// invite's roles again, so the grant policy is checked again for the caller.
func (s *InviteService) ResendInvite(ctx context.Context, orgID, callerID, inviteID uuid.UUID) (*InviteUserResponse, error) {
	rawToken, err := s.newToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	var invite *Invite
	err = s.repo.WithTx(ctx, func(querier Querier) error {
		invite, err = s.lockManageableInvite(ctx, querier, orgID, inviteID)
		if err != nil {
			return err
		}
		roleIDs, err := s.repo.GetInviteRoleIDsInOrg(ctx, querier, invite.ID, orgID)
		if err != nil {
			return err
		}
		scope, err := lockGrantScope(ctx, s.repo, querier, callerID, orgID, roleIDs)
		if err != nil {
			return err
		}
		granted, err := s.repo.GetPermissionIDsOfRoles(ctx, querier, roleIDs)
		if err != nil {
			return err
		}
		if err := scope.requireGrantable(granted); err != nil {
			return err
		}

		invite.TokenHash = HashToken(rawToken)
		invite.ExpiresAt = time.Now().Add(s.cfg.Auth.InviteTTL())
		return s.repo.ReissueInvite(ctx, querier, invite.ID, invite.TokenHash, invite.ExpiresAt)
	})
	if err != nil {
		return nil, appError(err)
	}

	s.sendInviteEmail(ctx, invite, callerID, rawToken)
	s.log.Info("invite resent", "org_id", orgID, "invite_id", invite.ID, "resent_by", callerID)
	return &InviteUserResponse{Message: "Invite sent again", InviteID: invite.ID, ExpiresAt: invite.ExpiresAt}, nil
}

// CancelInvite revokes an invite so its link stops working. Cancelling an
// already-cancelled invite succeeds; an accepted one cannot be cancelled.
func (s *InviteService) CancelInvite(ctx context.Context, orgID, callerID, inviteID uuid.UUID) error {
	err := s.repo.WithTx(ctx, func(querier Querier) error {
		invite, err := s.repo.LockInvite(ctx, querier, orgID, inviteID)
		if err != nil {
			return err
		}
		if invite == nil {
			return errInviteNotFound()
		}
		switch invite.Status(time.Now()) {
		case InviteStatusAccepted:
			return apperr.Conflict(apperr.CodeInviteAlreadyAccepted, "invite already accepted")
		case InviteStatusRevoked:
			return nil
		}
		return s.repo.RevokeInvite(ctx, querier, invite.ID)
	})
	if err != nil {
		return appError(err)
	}
	s.log.Info("invite cancelled", "org_id", orgID, "invite_id", inviteID, "cancelled_by", callerID)
	return nil
}

// lockManageableInvite locks an invite of the org that can still be resent.
func (s *InviteService) lockManageableInvite(ctx context.Context, querier Querier, orgID, inviteID uuid.UUID) (*Invite, error) {
	invite, err := s.repo.LockInvite(ctx, querier, orgID, inviteID)
	if err != nil {
		return nil, err
	}
	if invite == nil {
		return nil, errInviteNotFound()
	}
	switch invite.Status(time.Now()) {
	case InviteStatusAccepted:
		return nil, apperr.Conflict(apperr.CodeInviteAlreadyAccepted, "invite already accepted")
	case InviteStatusRevoked:
		return nil, apperr.Conflict(apperr.CodeInviteRevoked, "invite has been cancelled")
	}
	return invite, nil
}

// sendInviteEmail queues the invite email. Failing to queue it is logged, not
// returned: the invite exists and can be resent.
func (s *InviteService) sendInviteEmail(ctx context.Context, invite *Invite, invitedBy uuid.UUID, rawToken string) {
	if s.notification == nil {
		return
	}
	org, err := s.repo.GetOrgByID(ctx, s.repo.Query(), invite.OrgID)
	if err != nil || org == nil {
		s.log.Warn("invite email not sent: organization lookup failed", "org_id", invite.OrgID, "invite_id", invite.ID, "error", err)
		return
	}

	inviteLink := buildAppLink(s.cfg.App.URL, "/invite/accept", map[string]string{"token": rawToken})
	expires := invite.ExpiresAt.Format(time.RFC1123)
	err = s.notification.EnqueueEmail(ctx, notification.Task{
		OrgID:     invite.OrgID,
		UserID:    &invitedBy,
		EventType: "invite_created",
		Recipient: invite.Email,
		Subject:   fmt.Sprintf("You're invited to %s on APM Master", org.Name),
		Body:      fmt.Sprintf("Hi %s,\n\nYou have been invited to join %s on APM Master.\n\nAccept your invite here:\n%s\n\nThis invite expires on %s.", invite.FirstName, org.Name, inviteLink, expires),
		Metadata: map[string]string{
			"org_id":      invite.OrgID.String(),
			"invited_by":  invitedBy.String(),
			"first_name":  invite.FirstName,
			"org_name":    org.Name,
			"invite_link": inviteLink,
			"expires_at":  expires,
		},
	})
	if err != nil {
		s.log.Warn("invite email enqueue failed", "org_id", invite.OrgID, "invite_id", invite.ID, "error", err)
	}
}

// inviteUnusable is the client error for an invite that cannot be looked up
// or accepted, or nil when it is pending.
func inviteUnusable(invite *Invite, now time.Time) *apperr.Error {
	if invite == nil {
		return errInviteNotFound()
	}
	switch invite.Status(now) {
	case InviteStatusAccepted:
		return apperr.BadRequest(apperr.CodeInviteAlreadyAccepted, "invite already accepted")
	case InviteStatusRevoked:
		return apperr.BadRequest(apperr.CodeInviteRevoked, "invite has been cancelled")
	case InviteStatusExpired:
		return errInviteExpired()
	}
	return nil
}

func errInviteNotFound() *apperr.Error {
	return apperr.NotFound(apperr.CodeNotFound, "invite not found")
}

func errInviteExpired() *apperr.Error {
	return apperr.BadRequest(apperr.CodeInviteExpired, "invite has expired")
}

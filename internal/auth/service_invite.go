package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
)

type InviteService struct {
	repo         *Repository
	cfg          *config.Config
	notification *notification.Service
	log          *slog.Logger
}

func NewInviteService(repo *Repository, cfg *config.Config, notificationService *notification.Service, log *slog.Logger) *InviteService {
	if log == nil {
		log = slog.Default()
	}

	return &InviteService{
		repo:         repo,
		cfg:          cfg,
		notification: notificationService,
		log:          log,
	}
}

func (s *InviteService) CreateInvite(
	ctx context.Context,
	input InviteUserRequest,
) (*InviteUserResponse, error) {
	existingInvite, err :=
		s.repo.GetPendingInviteByEmail(
			ctx,
			s.repo.Query(),
			input.Email,
			input.OrgID,
		)

	if err != nil {
		return nil,
			apperr.Internal(err)
	}

	if existingInvite != nil {
		return nil,
			apperr.AlreadyExists(
				apperr.CodeInviteAlreadyExists,
				"invite already exists",
			)
	}

	rawToken, err := GenerateToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	hashedToken := HashToken(
		rawToken,
	)

	user, err := s.repo.GetUserByEmail(
		ctx,
		s.repo.Query(),
		input.OrgID,
		input.Email,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if user != nil {
		return nil, apperr.BadRequest(
			apperr.CodeEmailExists,
			"user with this email already exists",
		)
	}

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			invite, err := s.repo.CreateInvite(
				ctx,
				querier,
				Invite{
					OrgID: input.OrgID,

					InvitedBy: input.UserID,

					FirstName: input.FirstName,
					LastName:  input.LastName,
					Email:     input.Email,

					TokenHash: hashedToken,

					ExpiresAt: time.Now().Add(
						s.cfg.Auth.InviteTTL(),
					),
				},
			)
			if err != nil {
				return err
			}

			for _, roleID := range input.RoleIDs {

				err := s.repo.AssignInviteRole(
					ctx,
					querier,
					InviteRole{
						InviteID: invite.ID,
						RoleID:   roleID,
					},
				)
				if err != nil {
					return err
				}
			}

			return nil
		},
	)

	if err != nil {
		return nil, apperr.Internal(err)
	}

	org, err := s.repo.GetOrgByID(ctx, s.repo.Query(), input.OrgID)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if org == nil {
		return nil, apperr.NotFound(apperr.CodeOrgNotFound, "organization not found")
	}

	if s.notification != nil {
		inviteLink := buildAppLink(s.cfg.App.URL, "/invite/accept", map[string]string{"token": rawToken})
		err = s.notification.EnqueueEmail(ctx, notification.Task{
			OrgID:     input.OrgID,
			UserID:    &input.UserID,
			EventType: "invite_created",
			Recipient: input.Email,
			Subject:   fmt.Sprintf("You're invited to %s on APM Master", org.Name),
			Body:      fmt.Sprintf("Hi %s,\n\nYou have been invited to join %s on APM Master.\n\nAccept your invite here:\n%s\n\nThis invite expires on %s.", input.FirstName, org.Name, inviteLink, time.Now().Add(s.cfg.Auth.InviteTTL()).Format(time.RFC1123)),
			Metadata: map[string]string{
				"org_id":      input.OrgID.String(),
				"invited_by":  input.UserID.String(),
				"first_name":  input.FirstName,
				"org_name":    org.Name,
				"invite_link": inviteLink,
				"expires_at":  time.Now().Add(s.cfg.Auth.InviteTTL()).Format(time.RFC1123),
			},
		})
		if err != nil {
			s.log.Warn("invite email enqueue failed", "org_id", input.OrgID, "email", input.Email, "error", err)
		}
	}

	s.log.Info("invite created", "org_id", input.OrgID, "email", input.Email, "invited_by", input.UserID)

	return &InviteUserResponse{
		InviteLink: rawToken,
		Message:    "Invite created successfully",
	}, nil
}

func (s *InviteService) GetInviteByToken(
	ctx context.Context,
	input GetInviteRequest,
) (*GetInviteResponse, error) {

	hashedToken := HashToken(
		input.Token,
	)

	invite, err := s.repo.GetInviteByTokenHash(
		ctx,
		s.repo.Query(),
		hashedToken,
	)

	if err != nil {
		return nil, apperr.Internal(err)
	}

	if invite == nil {
		return nil, apperr.NotFound(
			apperr.CodeNotFound,
			"invite not found",
		)
	}

	if invite.AcceptedAt != nil {
		return nil, apperr.BadRequest(
			apperr.CodeInviteAlreadyAccepted,
			"invite already accepted",
		)
	}

	if invite.ExpiresAt.Before(time.Now()) {
		return nil, apperr.BadRequest(
			apperr.CodeInviteExpired,
			"invite has expired",
		)
	}

	return &GetInviteResponse{
		Email:     invite.Email,
		FirstName: invite.FirstName,
		LastName:  invite.LastName,
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func (s *InviteService) AcceptInvite(
	ctx context.Context,
	input AcceptInviteRequest,
) (*LoginResponse, error) {

	hashedToken := HashToken(
		input.Token,
	)

	invite, err := s.repo.GetInviteByTokenHash(
		ctx,
		s.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if invite == nil {
		return nil, apperr.NotFound(
			apperr.CodeNotFound,
			"invite not found",
		)
	}

	if invite.AcceptedAt != nil {
		return nil, apperr.BadRequest(
			apperr.CodeInviteAlreadyAccepted,
			"invite already accepted",
		)
	}

	if invite.ExpiresAt.Before(
		time.Now(),
	) {
		return nil, apperr.BadRequest(
			apperr.CodeInviteExpired,
			"invite has expired",
		)
	}

	user, err := s.repo.GetUserByEmail(
		ctx,
		s.repo.Query(),
		invite.OrgID,
		invite.Email,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if user != nil {
		return nil, apperr.BadRequest(
			apperr.CodeEmailExists,
			"user with this email is already registered in this organization",
		)
	}

	passwordHash, err :=
		bcrypt.GenerateFromPassword(
			[]byte(input.Password),
			bcrypt.DefaultCost,
		)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	roleIDs, err := s.repo.GetInviteRoles(
		ctx,
		s.repo.Query(),
		invite.ID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	var createdUser *User

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			hash := string(
				passwordHash,
			)

			user, err := s.repo.CreateUser(
				ctx,
				querier,
				User{
					OrgID:        invite.OrgID,
					FirstName:    invite.FirstName,
					LastName:     invite.LastName,
					Email:        invite.Email,
					PasswordHash: &hash,
				},
			)
			if err != nil {
				return err
			}

			for _, roleID := range roleIDs {

				err := s.repo.AssignRoleToUser(
					ctx,
					querier,
					user.ID,
					roleID,
				)
				if err != nil {
					return err
				}
			}

			err = s.repo.MarkInviteAccepted(
				ctx,
				querier,
				invite.ID,
			)
			if err != nil {
				return err
			}

			createdUser = user

			return nil
		},
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	accessToken, err :=
		GenerateAccessToken(
			*createdUser,
			s.cfg.Auth.AccessTokenTTL(),
			s.cfg.Auth.JWTSecret,
		)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	refreshToken, err :=
		GenerateToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	hashedRefreshToken :=
		HashToken(
			refreshToken,
		)

	_, err = s.repo.CreateRefreshToken(
		ctx,
		s.repo.Query(),
		RefreshToken{
			UserID:    createdUser.ID,
			TokenHash: hashedRefreshToken,
			ExpiresAt: time.Now().Add(
				s.cfg.Auth.RefreshTokenTTL(),
			),
		},
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("invite accepted", "org_id", invite.OrgID, "user_id", createdUser.ID, "invite_id", invite.ID)

	return &LoginResponse{
		User: NewUserResponse(
			*createdUser,
		),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

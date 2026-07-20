package services

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/auth/dto"
	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/auth/repositories"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"golang.org/x/crypto/bcrypt"
)

type InviteService struct {
	repo         *repositories.Repository
	cfg          *config.Config
	notification *notification.Service
	log          *slog.Logger
}

func NewInviteService(repo *repositories.Repository, cfg *config.Config, notificationService *notification.Service, log *slog.Logger) *InviteService {
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

func (service *InviteService) CreateInvite(
	ctx context.Context,
	input dto.InviteUserRequest,
) (*dto.InviteUserResponse, error) {
	existingInvite, err :=
		service.repo.GetPendingInviteByEmail(
			ctx,
			service.repo.Query(),
			input.Email,
			input.OrgID,
		)

	if err != nil {
		return nil,
			server.Internal(err)
	}

	if existingInvite != nil {
		return nil,
			server.AlreadyExists(
				server.CodeInviteAlreadyExists,
				"invite already exists",
			)
	}

	rawToken, err := GenerateToken()
	if err != nil {
		return nil, server.Internal(err)
	}

	hashedToken := HashToken(
		rawToken,
	)

	user, err := service.repo.GetUserByEmail(
		ctx,
		service.repo.Query(),
		input.OrgID,
		input.Email,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if user != nil {
		return nil, server.BadRequest(
			server.CodeEmailExists,
			"user with this email already exists",
		)
	}

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			invite, err := service.repo.CreateInvite(
				ctx,
				querier,
				models.Invite{
					OrgID: input.OrgID,

					InvitedBy: input.UserID,

					FirstName: input.FirstName,
					LastName:  input.LastName,
					Email:     input.Email,

					TokenHash: hashedToken,

					ExpiresAt: time.Now().Add(
						service.cfg.InviteDuration(),
					),
				},
			)
			if err != nil {
				return err
			}

			for _, roleID := range input.RoleIDs {

				err := service.repo.AssignInviteRole(
					ctx,
					querier,
					models.InviteRole{
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
		return nil, server.Internal(err)
	}

	org, err := service.repo.GetOrgByID(ctx, service.repo.Query(), input.OrgID)
	if err != nil {
		return nil, server.Internal(err)
	}
	if org == nil {
		return nil, server.NotFound(server.CodeOrgNotFound, "organization not found")
	}

	if service.notification != nil {
		inviteLink := buildAppLink(service.cfg.AppUrl, "/invite/accept", map[string]string{"token": rawToken})
		err = service.notification.EnqueueEmail(ctx, notification.NotificationTask{
			OrgID:     input.OrgID,
			UserID:    &input.UserID,
			EventType: "invite_created",
			Recipient: input.Email,
			Subject:   fmt.Sprintf("You're invited to %s on APM Master", org.Name),
			Body:      fmt.Sprintf("Hi %s,\n\nYou have been invited to join %s on APM Master.\n\nAccept your invite here:\n%s\n\nThis invite expires on %s.", input.FirstName, org.Name, inviteLink, time.Now().Add(service.cfg.InviteDuration()).Format(time.RFC1123)),
			Metadata: map[string]string{
				"org_id":      input.OrgID.String(),
				"invited_by":  input.UserID.String(),
				"first_name":  input.FirstName,
				"org_name":    org.Name,
				"invite_link": inviteLink,
				"expires_at":  time.Now().Add(service.cfg.InviteDuration()).Format(time.RFC1123),
			},
		})
		if err != nil {
			service.log.Warn("invite email enqueue failed", "org_id", input.OrgID, "email", input.Email, "error", err)
		}
	}

	service.log.Info("invite created", "org_id", input.OrgID, "email", input.Email, "invited_by", input.UserID)

	return &dto.InviteUserResponse{
		InviteLink: rawToken,
		Message:    "Invite created successfully",
	}, nil
}

func (service *InviteService) GetInviteByToken(
	ctx context.Context,
	input dto.GetInviteRequest,
) (*dto.GetInviteResponse, error) {

	hashedToken := HashToken(
		input.Token,
	)

	invite, err := service.repo.GetInviteByTokenHash(
		ctx,
		service.repo.Query(),
		hashedToken,
	)

	if err != nil {
		return nil, server.Internal(err)
	}

	if invite == nil {
		return nil, server.NotFound(
			server.CodeNotFound,
			"invite not found",
		)
	}

	if invite.AcceptedAt != nil {
		return nil, server.BadRequest(
			server.CodeInviteAlreadyAccepted,
			"invite already accepted",
		)
	}

	if invite.ExpiresAt.Before(time.Now()) {
		return nil, server.BadRequest(
			server.CodeInviteExpired,
			"invite has expired",
		)
	}

	return &dto.GetInviteResponse{
		Email:     invite.Email,
		FirstName: invite.FirstName,
		LastName:  invite.LastName,
		ExpiresAt: invite.ExpiresAt,
	}, nil
}

func (service *InviteService) AcceptInvite(
	ctx context.Context,
	input dto.AcceptInviteRequest,
) (*dto.LoginResponse, error) {

	hashedToken := HashToken(
		input.Token,
	)

	invite, err := service.repo.GetInviteByTokenHash(
		ctx,
		service.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if invite == nil {
		return nil, server.NotFound(
			server.CodeNotFound,
			"invite not found",
		)
	}

	if invite.AcceptedAt != nil {
		return nil, server.BadRequest(
			server.CodeInviteAlreadyAccepted,
			"invite already accepted",
		)
	}

	if invite.ExpiresAt.Before(
		time.Now(),
	) {
		return nil, server.BadRequest(
			server.CodeInviteExpired,
			"invite has expired",
		)
	}

	user, err := service.repo.GetUserByEmail(
		ctx,
		service.repo.Query(),
		invite.OrgID,
		invite.Email,
	)
	if err != nil {
		return nil, server.Internal(err)
	}
	if user != nil {
		return nil, server.BadRequest(
			server.CodeEmailExists,
			"user with this email is already registered in this organization",
		)
	}

	passwordHash, err :=
		bcrypt.GenerateFromPassword(
			[]byte(input.Password),
			bcrypt.DefaultCost,
		)
	if err != nil {
		return nil, server.Internal(err)
	}

	roleIDs, err := service.repo.GetInviteRoles(
		ctx,
		service.repo.Query(),
		invite.ID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	var createdUser *models.User

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			hash := string(
				passwordHash,
			)

			user, err := service.repo.CreateUser(
				ctx,
				querier,
				models.User{
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

				err := service.repo.AssignRoleToUser(
					ctx,
					querier,
					user.ID,
					roleID,
				)
				if err != nil {
					return err
				}
			}

			err = service.repo.MarkInviteAccepted(
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
		return nil, server.Internal(err)
	}

	accessToken, err :=
		GenerateAccessToken(
			*createdUser,
			service.cfg.AccessTokenDuration(),
			service.cfg.JWTSecret,
		)
	if err != nil {
		return nil, server.Internal(err)
	}

	refreshToken, err :=
		GenerateToken()
	if err != nil {
		return nil, server.Internal(err)
	}

	hashedRefreshToken :=
		HashToken(
			refreshToken,
		)

	_, err = service.repo.CreateRefreshToken(
		ctx,
		service.repo.Query(),
		models.RefreshToken{
			UserID:    createdUser.ID,
			TokenHash: hashedRefreshToken,
			ExpiresAt: time.Now().Add(
				service.cfg.RefreshTokenDuration(),
			),
		},
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("invite accepted", "org_id", invite.OrgID, "user_id", createdUser.ID, "invite_id", invite.ID)

	return &dto.LoginResponse{
		User: dto.NewUserResponse(
			*createdUser,
		),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

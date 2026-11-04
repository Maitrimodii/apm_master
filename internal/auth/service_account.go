package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// AccountService registers organizations and manages users' own accounts.
type AccountService struct {
	repo         *Repository
	cfg          *config.Config
	notification *notification.Service
	log          *slog.Logger
}

func NewAccountService(repo *Repository, cfg *config.Config, notification *notification.Service, log *slog.Logger) *AccountService {
	if log == nil {
		log = slog.Default()
	}
	return &AccountService{repo: repo, cfg: cfg, notification: notification, log: log}
}

func (s *AccountService) RegisterOrgOwner(
	ctx context.Context,
	input RegisterOrgOwnerRequest,
) (*RegisterOrgOwnerResponse, error) {

	existingOrg, err := s.repo.GetOrgBySlug(
		ctx,
		s.repo.Query(),
		input.OrgSlug,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if existingOrg != nil {
		return nil, apperr.AlreadyExists(
			apperr.CodeOrgSlugAlreadyExists,
			"organization slug already exists",
		)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(input.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, apperr.Internal(
			fmt.Errorf(
				"hash password: %w",
				err,
			),
		)
	}

	var createdOrg *Org
	var createdUser *User

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			org, err := s.repo.CreateOrg(
				ctx,
				querier,
				Org{
					Name: input.OrgName,
					Slug: input.OrgSlug,
				},
			)
			if err != nil {
				return apperr.Internal(err)
			}

			role, err := s.repo.CreateRole(
				ctx,
				querier,
				Role{
					OrgID:    org.ID,
					Name:     "admin",
					IsSystem: true,
				},
			)
			if err != nil {
				return apperr.Internal(err)
			}

			if err := s.seedAdminPermissions(ctx, querier, role.ID); err != nil {
				return err
			}

			passwordHash := string(
				hashedPassword,
			)

			user, err := s.repo.CreateUser(
				ctx,
				querier,
				User{
					OrgID:        org.ID,
					FirstName:    input.FirstName,
					LastName:     input.LastName,
					Email:        input.Email,
					PasswordHash: &passwordHash,
				},
			)
			if err != nil {
				return apperr.Internal(err)
			}

			err = s.repo.AssignRoleToUser(
				ctx,
				querier,
				user.ID,
				role.ID,
			)
			if err != nil {
				return apperr.Internal(err)
			}

			createdOrg = org
			createdUser = user

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	s.log.Info("organization owner registered", "org_id", createdOrg.ID, "user_id", createdUser.ID)

	return &RegisterOrgOwnerResponse{
		User: NewUserResponse(*createdUser),
		Org:  NewOrgResponse(*createdOrg),
	}, nil
}

func (s *AccountService) seedAdminPermissions(
	ctx context.Context,
	querier Querier,
	roleID uuid.UUID,
) error {
	defaultPermissions := make([]Permission, 0, len(authz.Catalogue))
	for _, p := range authz.Catalogue {
		defaultPermissions = append(defaultPermissions, Permission{Name: string(p.Resource), Action: p.Action, Description: p.Description})
	}

	for _, permission := range defaultPermissions {
		existing, err := s.repo.GetPermission(ctx, querier, permission.Name, permission.Action)
		if err != nil {
			return apperr.Internal(err)
		}

		if existing == nil {
			created, err := s.repo.CreatePermission(ctx, querier, permission)
			if err != nil {
				return apperr.Internal(err)
			}
			existing = created
		}

		if err := s.repo.AssignPermissionToRole(ctx, querier, roleID, existing.ID); err != nil {
			return apperr.Internal(err)
		}
	}

	return nil
}

func (s *AccountService) ForgotPassword(
	ctx context.Context,
	input ForgotPasswordRequest,
) (*ForgotPasswordResponse, error) {
	org, err := s.repo.GetOrgBySlug(ctx, s.repo.Query(), input.OrgSlug)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	response := &ForgotPasswordResponse{Message: "If the account exists, password reset instructions will be sent."}
	if org == nil {
		return response, nil
	}

	user, err := s.repo.GetUserByEmail(ctx, s.repo.Query(), org.ID, input.Email)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if user == nil {
		return response, nil
	}

	rawToken, err := GenerateToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	_, err = s.repo.CreatePasswordResetToken(ctx, s.repo.Query(), PasswordResetToken{
		UserID:    user.ID,
		TokenHash: HashToken(rawToken),
		ExpiresAt: time.Now().Add(s.cfg.Auth.PasswordResetTTL()),
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if s.notification != nil {
		resetLink := buildAppLink(s.cfg.App.URL, "/reset-password", map[string]string{"token": rawToken})
		err = s.notification.EnqueueEmail(ctx, notification.Task{
			OrgID:     org.ID,
			UserID:    &user.ID,
			EventType: "password_reset_requested",
			Recipient: user.Email,
			Subject:   "Reset your APM Master password",
			Body:      fmt.Sprintf("Hi %s,\n\nUse this link to reset your password:\n%s\n\nThis link expires in %d minutes.", user.FirstName, resetLink, s.cfg.Auth.PasswordResetExpiryMins),
			Metadata: map[string]string{
				"user_id":         user.ID.String(),
				"first_name":      user.FirstName,
				"reset_link":      resetLink,
				"expires_in_mins": fmt.Sprintf("%d", s.cfg.Auth.PasswordResetExpiryMins),
			},
		})
		if err != nil {
			s.log.Warn("password reset email enqueue failed", "org_id", org.ID, "user_id", user.ID, "error", err)
		}
	}

	s.log.Info("password reset requested", "org_id", org.ID, "user_id", user.ID)
	return response, nil
}

func (s *AccountService) ResetPassword(
	ctx context.Context,
	input ResetPasswordRequest,
) (*ResetPasswordResponse, error) {
	resetToken, err := s.repo.GetPasswordResetTokenByHash(ctx, s.repo.Query(), HashToken(input.Token))
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if resetToken == nil {
		s.log.WarnContext(ctx, "failed password reset attempt: invalid token", "token_hash", HashToken(input.Token))
		return nil, apperr.BadRequest(apperr.CodeInvalidToken, "invalid password reset token")
	}
	if resetToken.UsedAt != nil || resetToken.ExpiresAt.Before(time.Now()) {
		s.log.WarnContext(ctx, "failed password reset attempt: token expired or already used", "user_id", resetToken.UserID, "token_id", resetToken.ID)
		return nil, apperr.BadRequest(apperr.CodeTokenExpired, "password reset token has expired")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, apperr.Internal(fmt.Errorf("hash password: %w", err))
	}

	err = s.repo.WithTx(ctx, func(querier Querier) error {
		if err := s.repo.UpdateUserPassword(ctx, querier, resetToken.UserID, string(hashedPassword)); err != nil {
			return err
		}
		if err := s.repo.MarkPasswordResetTokenUsed(ctx, querier, resetToken.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("password reset completed", "user_id", resetToken.UserID)
	return &ResetPasswordResponse{Message: "Password reset successfully"}, nil
}

func (s *AccountService) Me(
	ctx context.Context,
	input MeRequest,
) (*MeResponse, error) {

	user, err := s.repo.GetUserByID(
		ctx,
		s.repo.Query(),
		input.UserID,
	)
	if err != nil {
		return nil, apperr.Internal(
			err,
		)
	}

	if user == nil {
		return nil, apperr.NotFound(
			apperr.CodeUserNotFound,
			"user not found",
		)
	}

	return &MeResponse{
		User: NewUserResponse(
			*user,
		),
	}, nil
}

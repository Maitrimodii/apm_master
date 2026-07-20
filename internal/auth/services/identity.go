package services

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/url"

	"time"

	"strings"

	"github.com/Maitrimodii/apm_master/internal/auth/dto"
	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/auth/repositories"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Email  string `json:"email"`

	jwt.RegisteredClaims
}

type IdentityService struct {
	repo         *repositories.Repository
	rdb          *redis.Client
	cfg          *config.Config
	notification *notification.Service
	log          *slog.Logger
}

var _ middleware.PermissionChecker = (*IdentityService)(nil)
var _ middleware.TokenParser = (*IdentityService)(nil)
var _ middleware.APIKeyValidator = (*IdentityService)(nil)

func NewIdentityService(repo *repositories.Repository, rdb *redis.Client, cfg *config.Config, notificationService *notification.Service, log *slog.Logger) *IdentityService {
	if log == nil {
		log = slog.Default()
	}

	return &IdentityService{
		repo:         repo,
		rdb:          rdb,
		cfg:          cfg,
		notification: notificationService,
		log:          log,
	}
}

func (service *IdentityService) RegisterOrgOwner(
	ctx context.Context,
	input dto.RegisterOrgOwnerRequest,
) (*dto.RegisterOrgOwnerResponse, error) {

	existingOrg, err := service.repo.GetOrgBySlug(
		ctx,
		service.repo.Query(),
		input.OrgSlug,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if existingOrg != nil {
		return nil, server.AlreadyExists(
			server.CodeOrgSlugAlreadyExists,
			"organization slug already exists",
		)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(input.Password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return nil, server.Internal(
			fmt.Errorf(
				"hash password: %w",
				err,
			),
		)
	}

	var createdOrg *models.Org
	var createdUser *models.User

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			org, err := service.repo.CreateOrg(
				ctx,
				querier,
				models.Org{
					Name: input.OrgName,
					Slug: input.OrgSlug,
				},
			)
			if err != nil {
				return server.Internal(err)
			}

			role, err := service.repo.CreateRole(
				ctx,
				querier,
				models.Role{
					OrgID:    org.ID,
					Name:     "admin",
					IsSystem: true,
				},
			)
			if err != nil {
				return server.Internal(err)
			}

			if err := service.seedAdminPermissions(ctx, querier, role.ID); err != nil {
				return err
			}

			passwordHash := string(
				hashedPassword,
			)

			user, err := service.repo.CreateUser(
				ctx,
				querier,
				models.User{
					OrgID:        org.ID,
					FirstName:    input.FirstName,
					LastName:     input.LastName,
					Email:        input.Email,
					PasswordHash: &passwordHash,
				},
			)
			if err != nil {
				return server.Internal(err)
			}

			err = service.repo.AssignRoleToUser(
				ctx,
				querier,
				user.ID,
				role.ID,
			)
			if err != nil {
				return server.Internal(err)
			}

			createdOrg = org
			createdUser = user

			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	service.log.Info("organization owner registered", "org_id", createdOrg.ID, "user_id", createdUser.ID)

	return &dto.RegisterOrgOwnerResponse{
		User: dto.NewUserResponse(*createdUser),
		Org:  dto.NewOrgResponse(*createdOrg),
	}, nil
}

func (service *IdentityService) seedAdminPermissions(
	ctx context.Context,
	querier repositories.Querier,
	roleID uuid.UUID,
) error {
	defaultPermissions := []models.Permission{
		{Name: "invite", Action: models.Write, Description: "Invite users to the organization"},
		{Name: "invite", Action: models.Read, Description: "View organization invites"},
		{Name: "permission", Action: models.Read, Description: "View permissions"},
		{Name: "permission", Action: models.Write, Description: "Create permissions"},
		{Name: "permission", Action: models.Update, Description: "Update permissions"},
		{Name: "permission", Action: models.Delete, Description: "Delete permissions"},
		{Name: "role", Action: models.Read, Description: "View roles"},
		{Name: "role", Action: models.Write, Description: "Create roles"},
		{Name: "role", Action: models.Update, Description: "Update roles"},
		{Name: "role", Action: models.Delete, Description: "Delete roles"},
		{Name: "traces", Action: models.Read, Description: "View traces"},
		{Name: "logs", Action: models.Read, Description: "View logs"},
		{Name: "metrics", Action: models.Read, Description: "View metrics"},
		{Name: "alerts", Action: models.Read, Description: "View alerts"},
		{Name: "alerts", Action: models.Write, Description: "Create and share alerts"},
		{Name: "alerts", Action: models.Update, Description: "Update alerts"},
		{Name: "alerts", Action: models.Delete, Description: "Delete alerts"},
		{Name: "api_key", Action: models.Read, Description: "View API keys"},
		{Name: "api_key", Action: models.Write, Description: "Create API keys"},
		{Name: "api_key", Action: models.Delete, Description: "Revoke API keys"},
	}

	for _, permission := range defaultPermissions {
		existing, err := service.repo.GetPermission(ctx, querier, permission.Name, permission.Action)
		if err != nil {
			return server.Internal(err)
		}

		if existing == nil {
			created, err := service.repo.CreatePermission(ctx, querier, permission)
			if err != nil {
				return server.Internal(err)
			}
			existing = created
		}

		if err := service.repo.AssignPermissionToRole(ctx, querier, roleID, existing.ID); err != nil {
			return server.Internal(err)
		}
	}

	return nil
}

func (service *IdentityService) ParseToken(
	tokenString string,
) (
	userID string,
	orgID string,
	email string,
	err error,
) {

	claims := &Claims{}

	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(token *jwt.Token) (any, error) {

			if _, exists := token.Method.(*jwt.SigningMethodHMAC); !exists {
				return nil, fmt.Errorf(
					"unexpected signing method: %v",
					token.Header["alg"],
				)
			}

			return []byte(
				service.cfg.JWTSecret,
			), nil
		},
	)

	if err != nil {
		return "", "", "", fmt.Errorf(
			"parse token: %w",
			err,
		)
	}

	if !token.Valid {
		return "", "", "", fmt.Errorf(
			"invalid token",
		)
	}

	return claims.UserID,
		claims.OrgID,
		claims.Email,
		nil
}

func (service *IdentityService) Login(
	ctx context.Context,
	input dto.LoginRequest,
) (*dto.LoginResponse, error) {
	org, err := service.repo.GetOrgBySlug(
		ctx,
		service.repo.Query(),
		input.OrgSlug,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if org == nil {
		return nil, server.Unauthorized(
			server.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	user, err := service.repo.GetUserByEmail(
		ctx,
		service.repo.Query(),
		org.ID,
		input.Email,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if user == nil {
		service.log.Warn("login failed", "org_id", org.ID, "email", input.Email, "reason", "user_not_found")
		return nil, server.Unauthorized(
			server.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	if user.PasswordHash == nil {
		service.log.Warn("login failed", "org_id", org.ID, "user_id", user.ID, "reason", "password_login_disabled")
		return nil, server.Unauthorized(
			server.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(*user.PasswordHash),
		[]byte(input.Password),
	)

	if err != nil {
		service.log.Warn("login failed", "org_id", org.ID, "user_id", user.ID, "reason", "invalid_password")
		return nil, server.Unauthorized(
			server.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	accessToken, err := GenerateAccessToken(*user, service.cfg.AccessTokenDuration(), service.cfg.JWTSecret)
	if err != nil {
		return nil, server.Internal(err)
	}

	refreshToken, err := GenerateToken()

	if err != nil {
		return nil, server.Internal(err)
	}

	hashedRefreshToken := HashToken(refreshToken)

	var ipAddress *net.IP

	if input.IPAddress != nil {
		parsedIP := net.ParseIP(
			*input.IPAddress,
		)

		if parsedIP != nil {
			ipAddress = &parsedIP
		}
	}

	refreshTokenModel := models.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashedRefreshToken,
		UserAgent: input.UserAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(
			service.cfg.RefreshTokenDuration(),
		),
	}

	_, err = service.repo.CreateRefreshToken(
		ctx,
		service.repo.Query(),
		refreshTokenModel,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("user logged in", "org_id", org.ID, "user_id", user.ID)

	return &dto.LoginResponse{
		User:         dto.NewUserResponse(*user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (service *IdentityService) RefreshToken(
	ctx context.Context,
	input dto.RefreshTokenRequest,
) (*dto.LoginResponse, error) {

	hashedToken := HashToken(
		input.RefreshToken,
	)

	refreshToken, err := service.repo.GetRefreshTokenByHash(
		ctx,
		service.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if refreshToken == nil {
		return nil, server.Unauthorized(
			server.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	if refreshToken.RevokedAt != nil ||
		refreshToken.ExpiresAt.Before(
			time.Now(),
		) {

		return nil, server.Unauthorized(
			server.CodeTokenExpired,
			"invalid refresh token",
		)
	}

	user, err := service.repo.GetUserByID(
		ctx,
		service.repo.Query(),
		refreshToken.UserID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if user == nil {
		return nil, server.Unauthorized(
			server.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	accessToken, err := GenerateAccessToken(
		*user,
		service.cfg.AccessTokenDuration(),
		service.cfg.JWTSecret,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	newRefreshToken, err := GenerateToken()
	if err != nil {
		return nil, server.Internal(err)
	}

	hashedNewRefreshToken := HashToken(
		newRefreshToken,
	)

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			err := service.repo.RevokeRefreshToken(
				ctx,
				querier,
				refreshToken.ID,
			)
			if err != nil {
				return err
			}

			var ipAddress *net.IP

			if input.IPAddress != nil {
				parsedIP := net.ParseIP(
					*input.IPAddress,
				)

				if parsedIP != nil {
					ipAddress = &parsedIP
				}
			}

			_, err = service.repo.CreateRefreshToken(
				ctx,
				querier,
				models.RefreshToken{
					UserID: user.ID,

					TokenHash: hashedNewRefreshToken,

					UserAgent:  input.UserAgent,
					IPAddress:  ipAddress,
					DeviceName: refreshToken.DeviceName,

					ExpiresAt: time.Now().Add(
						service.cfg.RefreshTokenDuration(),
					),
				},
			)

			if err != nil {
				return err
			}

			return nil
		},
	)

	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("refresh token rotated", "user_id", user.ID, "refresh_token_id", refreshToken.ID)

	return &dto.LoginResponse{
		User: dto.NewUserResponse(
			*user,
		),
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (service *IdentityService) Logout(
	ctx context.Context,
	input dto.LogoutRequest,
) error {
	hashedToken := HashToken(
		input.RefreshToken,
	)

	refreshToken, err := service.repo.GetRefreshTokenByHash(
		ctx,
		service.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return server.Internal(err)
	}

	if refreshToken == nil {
		return server.Unauthorized(
			server.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	if refreshToken.RevokedAt != nil {
		return nil
	}

	err = service.repo.RevokeRefreshToken(
		ctx,
		service.repo.Query(),
		refreshToken.ID,
	)
	if err != nil {
		return server.Internal(err)
	}

	service.log.Info("user logged out", "user_id", refreshToken.UserID, "refresh_token_id", refreshToken.ID)

	return nil
}

func (service *IdentityService) ForgotPassword(
	ctx context.Context,
	input dto.ForgotPasswordRequest,
) (*dto.ForgotPasswordResponse, error) {
	org, err := service.repo.GetOrgBySlug(ctx, service.repo.Query(), input.OrgSlug)
	if err != nil {
		return nil, server.Internal(err)
	}

	response := &dto.ForgotPasswordResponse{Message: "If the account exists, password reset instructions will be sent."}
	if org == nil {
		return response, nil
	}

	user, err := service.repo.GetUserByEmail(ctx, service.repo.Query(), org.ID, input.Email)
	if err != nil {
		return nil, server.Internal(err)
	}
	if user == nil {
		return response, nil
	}

	rawToken, err := GenerateToken()
	if err != nil {
		return nil, server.Internal(err)
	}

	_, err = service.repo.CreatePasswordResetToken(ctx, service.repo.Query(), models.PasswordResetToken{
		UserID:    user.ID,
		TokenHash: HashToken(rawToken),
		ExpiresAt: time.Now().Add(service.cfg.PasswordResetDuration()),
	})
	if err != nil {
		return nil, server.Internal(err)
	}

	if service.notification != nil {
		resetLink := buildAppLink(service.cfg.AppUrl, "/reset-password", map[string]string{"token": rawToken})
		err = service.notification.EnqueueEmail(ctx, notification.NotificationTask{
			OrgID:     org.ID,
			UserID:    &user.ID,
			EventType: "password_reset_requested",
			Recipient: user.Email,
			Subject:   "Reset your APM Master password",
			Body:      fmt.Sprintf("Hi %s,\n\nUse this link to reset your password:\n%s\n\nThis link expires in %d minutes.", user.FirstName, resetLink, service.cfg.PasswordResetExpiryMins),
			Metadata: map[string]string{
				"user_id":         user.ID.String(),
				"first_name":      user.FirstName,
				"reset_link":      resetLink,
				"expires_in_mins": fmt.Sprintf("%d", service.cfg.PasswordResetExpiryMins),
			},
		})
		if err != nil {
			service.log.Warn("password reset email enqueue failed", "org_id", org.ID, "user_id", user.ID, "error", err)
		}
	}

	service.log.Info("password reset requested", "org_id", org.ID, "user_id", user.ID)
	return response, nil
}

func (service *IdentityService) ResetPassword(
	ctx context.Context,
	input dto.ResetPasswordRequest,
) (*dto.ResetPasswordResponse, error) {
	resetToken, err := service.repo.GetPasswordResetTokenByHash(ctx, service.repo.Query(), HashToken(input.Token))
	if err != nil {
		return nil, server.Internal(err)
	}
	if resetToken == nil {
		service.log.WarnContext(ctx, "failed password reset attempt: invalid token", "token_hash", HashToken(input.Token))
		return nil, server.BadRequest(server.CodeInvalidToken, "invalid password reset token")
	}
	if resetToken.UsedAt != nil || resetToken.ExpiresAt.Before(time.Now()) {
		service.log.WarnContext(ctx, "failed password reset attempt: token expired or already used", "user_id", resetToken.UserID, "token_id", resetToken.ID)
		return nil, server.BadRequest(server.CodeTokenExpired, "password reset token has expired")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, server.Internal(fmt.Errorf("hash password: %w", err))
	}

	err = service.repo.WithTx(ctx, func(querier repositories.Querier) error {
		if err := service.repo.UpdateUserPassword(ctx, querier, resetToken.UserID, string(hashedPassword)); err != nil {
			return err
		}
		if err := service.repo.MarkPasswordResetTokenUsed(ctx, querier, resetToken.ID); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("password reset completed", "user_id", resetToken.UserID)
	return &dto.ResetPasswordResponse{Message: "Password reset successfully"}, nil
}

func (service *IdentityService) Me(
	ctx context.Context,
	input dto.MeRequest,
) (*dto.MeResponse, error) {

	user, err := service.repo.GetUserByID(
		ctx,
		service.repo.Query(),
		input.UserID,
	)
	if err != nil {
		return nil, server.Internal(
			err,
		)
	}

	if user == nil {
		return nil, server.NotFound(
			server.CodeUserNotFound,
			"user not found",
		)
	}

	return &dto.MeResponse{
		User: dto.NewUserResponse(
			*user,
		),
	}, nil
}

func (service *IdentityService) CheckPermission(
	ctx context.Context,
	userID uuid.UUID,
	orgID uuid.UUID,
	name string,
	action models.PermissionAction,
) (bool, error) {
	return service.repo.HasPermission(ctx, service.repo.Query(), userID, orgID, name, action)
}

func buildAppLink(baseURL, path string, params map[string]string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return baseURL
	}

	parsed.Path = path
	query := parsed.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func (service *IdentityService) ValidateAPIKey(
	ctx context.Context,
	rawToken string,
) (uuid.UUID, error) {

	tokenHash := HashToken(rawToken)
	cacheKey := "apikey:org:" + tokenHash

	// 1. Try to get cached org_id and api_key_id from Redis
	if val, err := service.rdb.Get(ctx, cacheKey).Result(); err == nil {
		parts := strings.Split(val, ":")
		if len(parts) == 2 {
			orgID, parseErr1 := uuid.Parse(parts[0])
			apiKeyID, parseErr2 := uuid.Parse(parts[1])
			if parseErr1 == nil && parseErr2 == nil {
				service.throttleLastUsedUpdate(ctx, apiKeyID)
				return orgID, nil
			}
		}
	}

	// 2. Cache miss -> query DB
	apiKey, err := service.repo.GetAPIKeyByHash(
		ctx,
		service.repo.Query(),
		tokenHash,
	)
	if err != nil {
		return uuid.UUID{}, err
	}

	if apiKey == nil {
		return uuid.UUID{}, server.Unauthorized(
			server.CodeAPIKeyInvalid,
			"api key not found",
		)
	}

	if apiKey.RevokedAt != nil {
		return uuid.UUID{}, server.Unauthorized(
			server.CodeAPIKeyInvalid,
			"api key revoked",
		)
	}

	// 3. Cache the result in Redis for 5 minutes (Format: orgID:apiKeyID)
	cacheValue := fmt.Sprintf("%s:%s", apiKey.OrgID.String(), apiKey.ID.String())
	_ = service.rdb.Set(ctx, cacheKey, cacheValue, 5*time.Minute).Err()

	// 4. Update last used with throttling
	service.throttleLastUsedUpdate(ctx, apiKey.ID)

	service.log.Debug("api key validated", "org_id", apiKey.OrgID, "api_key_id", apiKey.ID)

	return apiKey.OrgID, nil
}

func (service *IdentityService) throttleLastUsedUpdate(ctx context.Context, apiKeyID uuid.UUID) {
	throttleKey := "apikey:lastused_throttle:" + apiKeyID.String()
	// Try to set the throttle key with a 1-minute TTL
	set, err := service.rdb.SetNX(ctx, throttleKey, "1", time.Minute).Result()
	if err == nil && set {
		// Only update the database if we successfully set the key (meaning it wasn't set already)
		if err := service.repo.UpdateAPIKeyLastUsed(ctx, service.repo.Query(), apiKeyID); err != nil {
			service.log.Warn("failed to update api key last used time", "api_key_id", apiKeyID, "error", err)
		}
	}
}

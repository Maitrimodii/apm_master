package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// Claims are the JWT access-token claims.
type Claims struct {
	UserID string `json:"user_id"`
	OrgID  string `json:"org_id"`
	Email  string `json:"email"`

	jwt.RegisteredClaims
}

// SessionService issues, refreshes, revokes and verifies JWT sessions. It
// implements middleware.TokenParser.
type SessionService struct {
	repo *Repository
	cfg  *config.Config
	log  *slog.Logger
}

func NewSessionService(repo *Repository, cfg *config.Config, log *slog.Logger) *SessionService {
	if log == nil {
		log = slog.Default()
	}
	return &SessionService{repo: repo, cfg: cfg, log: log}
}

func (s *SessionService) ParseToken(
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
				s.cfg.Auth.JWTSecret,
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

func (s *SessionService) Login(
	ctx context.Context,
	input LoginRequest,
) (*LoginResponse, error) {
	org, err := s.repo.GetOrgBySlug(
		ctx,
		s.repo.Query(),
		input.OrgSlug,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if org == nil {
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	user, err := s.repo.GetUserByEmail(
		ctx,
		s.repo.Query(),
		org.ID,
		input.Email,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if user == nil {
		s.log.Warn("login failed", "org_id", org.ID, "email", input.Email, "reason", "user_not_found")
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	if user.PasswordHash == nil {
		s.log.Warn("login failed", "org_id", org.ID, "user_id", user.ID, "reason", "password_login_disabled")
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(*user.PasswordHash),
		[]byte(input.Password),
	)

	if err != nil {
		s.log.Warn("login failed", "org_id", org.ID, "user_id", user.ID, "reason", "invalid_password")
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidCredentials,
			"invalid credentials",
		)
	}

	accessToken, err := GenerateAccessToken(*user, s.cfg.Auth.AccessTokenTTL(), s.cfg.Auth.JWTSecret)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	refreshToken, err := GenerateToken()

	if err != nil {
		return nil, apperr.Internal(err)
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

	refreshTokenModel := RefreshToken{
		UserID:    user.ID,
		TokenHash: hashedRefreshToken,
		UserAgent: input.UserAgent,
		IPAddress: ipAddress,
		ExpiresAt: time.Now().Add(
			s.cfg.Auth.RefreshTokenTTL(),
		),
	}

	_, err = s.repo.CreateRefreshToken(
		ctx,
		s.repo.Query(),
		refreshTokenModel,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("user logged in", "org_id", org.ID, "user_id", user.ID)

	return &LoginResponse{
		User:         NewUserResponse(*user),
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *SessionService) RefreshToken(
	ctx context.Context,
	input RefreshTokenRequest,
) (*LoginResponse, error) {

	hashedToken := HashToken(
		input.RefreshToken,
	)

	refreshToken, err := s.repo.GetRefreshTokenByHash(
		ctx,
		s.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if refreshToken == nil {
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	if refreshToken.RevokedAt != nil ||
		refreshToken.ExpiresAt.Before(
			time.Now(),
		) {

		return nil, apperr.Unauthorized(
			apperr.CodeTokenExpired,
			"invalid refresh token",
		)
	}

	user, err := s.repo.GetUserByID(
		ctx,
		s.repo.Query(),
		refreshToken.UserID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if user == nil {
		return nil, apperr.Unauthorized(
			apperr.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	accessToken, err := GenerateAccessToken(
		*user,
		s.cfg.Auth.AccessTokenTTL(),
		s.cfg.Auth.JWTSecret,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	newRefreshToken, err := GenerateToken()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	hashedNewRefreshToken := HashToken(
		newRefreshToken,
	)

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			err := s.repo.RevokeRefreshToken(
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

			_, err = s.repo.CreateRefreshToken(
				ctx,
				querier,
				RefreshToken{
					UserID: user.ID,

					TokenHash: hashedNewRefreshToken,

					UserAgent:  input.UserAgent,
					IPAddress:  ipAddress,
					DeviceName: refreshToken.DeviceName,

					ExpiresAt: time.Now().Add(
						s.cfg.Auth.RefreshTokenTTL(),
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
		return nil, apperr.Internal(err)
	}

	s.log.Info("refresh token rotated", "user_id", user.ID, "refresh_token_id", refreshToken.ID)

	return &LoginResponse{
		User: NewUserResponse(
			*user,
		),
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
	}, nil
}

func (s *SessionService) Logout(
	ctx context.Context,
	input LogoutRequest,
) error {
	hashedToken := HashToken(
		input.RefreshToken,
	)

	refreshToken, err := s.repo.GetRefreshTokenByHash(
		ctx,
		s.repo.Query(),
		hashedToken,
	)
	if err != nil {
		return apperr.Internal(err)
	}

	if refreshToken == nil {
		return apperr.Unauthorized(
			apperr.CodeInvalidToken,
			"invalid refresh token",
		)
	}

	if refreshToken.RevokedAt != nil {
		return nil
	}

	err = s.repo.RevokeRefreshToken(
		ctx,
		s.repo.Query(),
		refreshToken.ID,
	)
	if err != nil {
		return apperr.Internal(err)
	}

	s.log.Info("user logged out", "user_id", refreshToken.UserID, "refresh_token_id", refreshToken.ID)

	return nil
}

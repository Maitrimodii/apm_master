package auth

import (
	"context"
	"fmt"
	"log/slog"
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
	repo   *Repository
	cfg    *config.Config
	issuer *sessionIssuer
	log    *slog.Logger
}

func NewSessionService(repo *Repository, cfg *config.Config, log *slog.Logger) *SessionService {
	if log == nil {
		log = slog.Default()
	}
	return &SessionService{repo: repo, cfg: cfg, issuer: newSessionIssuer(repo, cfg), log: log}
}

// Refresh-token rejections. The codes and message match what the endpoint
// returned before session families existed, so clients see no change.
func errInvalidRefreshToken() *apperr.Error {
	return apperr.Unauthorized(apperr.CodeInvalidToken, "invalid refresh token")
}

func errUnusableRefreshToken() *apperr.Error {
	return apperr.Unauthorized(apperr.CodeTokenExpired, "invalid refresh token")
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

	resp, err := s.issuer.issue(ctx, s.repo.Query(), *user, clientInfo{UserAgent: input.UserAgent, IPAddress: input.IPAddress}, nil)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("user logged in", "org_id", org.ID, "user_id", user.ID)

	return resp, nil
}

// RefreshToken rotates a refresh token: the presented token is consumed and a
// new one in the same session is issued along with a new access token.
//
// Rotation runs in one transaction that locks the user row (shared) and then
// consumes the token with a conditional UPDATE, so of concurrent requests
// presenting the same token exactly one rotates it. The new access token is
// signed and the new refresh token inserted inside that transaction, and the
// pair is returned only after commit: if any step fails the transaction rolls
// back and the presented token stays usable.
//
// A token that cannot be consumed is rejected without rotating anything:
//   - unknown: AUTH003
//   - expired: AUTH004; expiry is the normal end of a token, so its session
//     is left alone
//   - already revoked: AUTH004, and the whole session is revoked. A rotated
//     token being presented again means it was copied (or two clients raced
//     with one token), so the session can no longer be trusted.
func (s *SessionService) RefreshToken(
	ctx context.Context,
	input RefreshTokenRequest,
) (*LoginResponse, error) {
	tokenHash := HashToken(input.RefreshToken)

	// Find the owner first so the transaction can lock the user row before
	// the token row (the lock order every auth transaction follows; see
	// Repository.LockUser). This read decides nothing on its own.
	presented, err := s.repo.GetRefreshTokenByHash(ctx, s.repo.Query(), tokenHash)
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if presented == nil {
		return nil, errInvalidRefreshToken()
	}

	var (
		resp      *LoginResponse
		rejection *apperr.Error
		parentID  = presented.ID
	)
	err = s.repo.WithTx(ctx, func(querier Querier) error {
		user, err := s.repo.LockUser(ctx, querier, presented.UserID, lockUserShared)
		if err != nil {
			return err
		}
		if user == nil {
			rejection = errInvalidRefreshToken()
			return nil
		}

		parent, err := s.repo.ConsumeRefreshToken(ctx, querier, tokenHash)
		if err != nil {
			return err
		}
		if parent == nil {
			// Returning nil (not an error) commits any session revocation
			// rejectUnusableToken made.
			rejection, err = s.rejectUnusableToken(ctx, querier, tokenHash)
			return err
		}

		resp, err = s.issuer.issue(ctx, querier, *user, clientInfo{UserAgent: input.UserAgent, IPAddress: input.IPAddress}, parent)
		return err
	})
	if err != nil {
		return nil, apperr.Internal(err)
	}
	if rejection != nil {
		return nil, rejection
	}

	s.log.Info("refresh token rotated", "user_id", presented.UserID, "family_id", presented.FamilyID, "refresh_token_id", parentID)
	return resp, nil
}

// rejectUnusableToken explains why ConsumeRefreshToken matched nothing and
// applies the reuse policy. It runs in the rotation transaction, after the
// consume, so it sees the token as any concurrent rotation committed it.
func (s *SessionService) rejectUnusableToken(ctx context.Context, querier Querier, tokenHash string) (*apperr.Error, error) {
	token, err := s.repo.GetRefreshTokenByHash(ctx, querier, tokenHash)
	if err != nil {
		return nil, err
	}

	switch {
	case token == nil:
		return errInvalidRefreshToken(), nil

	// Expiry is checked before revocation so an expired token, rotated or
	// not, never counts as reuse. A token that is neither revoked nor expired
	// by this clock missed the consume by a hair of clock difference with the
	// database: treat it as expired too.
	case !token.ExpiresAt.After(time.Now()) || token.RevokedAt == nil:
		return errUnusableRefreshToken(), nil

	default:
		revoked, err := s.repo.RevokeRefreshTokenFamily(ctx, querier, token.FamilyID)
		if err != nil {
			return nil, err
		}
		s.log.WarnContext(ctx, "revoked refresh token presented, session revoked",
			"user_id", token.UserID, "family_id", token.FamilyID, "refresh_token_id", token.ID, "revoked_tokens", revoked)
		return errUnusableRefreshToken(), nil
	}
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

package auth

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/config"
)

// clientInfo describes the client a session is issued to. Both fields are
// optional.
type clientInfo struct {
	UserAgent *string
	IPAddress *string
}

// sessionIssuer creates access/refresh token pairs. Login, invite acceptance
// and refresh-token rotation all issue sessions through it, so the token
// rules (lifetimes, session families, absolute session end) live in one place.
type sessionIssuer struct {
	repo *Repository
	cfg  *config.Config

	// signAccessToken signs the access JWT. It is a field so tests can make
	// signing fail and check that nothing was persisted.
	signAccessToken func(user User) (string, error)
}

func newSessionIssuer(repo *Repository, cfg *config.Config) *sessionIssuer {
	return &sessionIssuer{
		repo: repo,
		cfg:  cfg,
		signAccessToken: func(user User) (string, error) {
			return GenerateAccessToken(user, cfg.Auth.AccessTokenTTL(), cfg.Auth.JWTSecret)
		},
	}
}

// issue signs an access token and stores a new refresh token for user through
// querier.
//
// With parent nil it starts a new session (token family) that ends after
// REFRESH_SESSION_MAX_DAYS. Otherwise the token continues parent's session:
// same family, same absolute end, and its own expiry is capped at that end, so
// rotation can never extend a session.
//
// Everything that can fail (signing, token generation, the insert) happens
// here, before the caller commits. When querier is a transaction, a failure
// rolls the whole operation back, and the caller hands the tokens out only
// after a successful commit.
func (i *sessionIssuer) issue(ctx context.Context, querier Querier, user User, client clientInfo, parent *RefreshToken) (*LoginResponse, error) {
	accessToken, err := i.signAccessToken(user)
	if err != nil {
		return nil, fmt.Errorf("sign access token: %w", err)
	}

	rawRefreshToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	now := time.Now()
	token := RefreshToken{
		UserID:    user.ID,
		TokenHash: HashToken(rawRefreshToken),
		UserAgent: client.UserAgent,
		IPAddress: parseIP(client.IPAddress),
	}

	var sessionEnd time.Time
	if parent == nil {
		token.FamilyID = uuid.New()
		sessionEnd = now.Add(i.cfg.Auth.RefreshSessionMaxTTL())
	} else {
		token.FamilyID = parent.FamilyID
		token.DeviceName = parent.DeviceName
		sessionEnd = parent.SessionEnd()
	}
	token.SessionExpiresAt = &sessionEnd

	token.ExpiresAt = now.Add(i.cfg.Auth.RefreshTokenTTL())
	if token.ExpiresAt.After(sessionEnd) {
		token.ExpiresAt = sessionEnd
	}

	if _, err := i.repo.CreateRefreshToken(ctx, querier, token); err != nil {
		return nil, err
	}

	return &LoginResponse{
		User:         NewUserResponse(user),
		AccessToken:  accessToken,
		RefreshToken: rawRefreshToken,
	}, nil
}

// parseIP returns the parsed address, or nil when it is absent or invalid.
func parseIP(address *string) *net.IP {
	if address == nil {
		return nil
	}
	ip := net.ParseIP(*address)
	if ip == nil {
		return nil
	}
	return &ip
}

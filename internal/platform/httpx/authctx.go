package httpx

import (
	"strings"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// OrgID returns the authenticated org set by the JWT or API-key middleware.
func OrgID(c *gin.Context) (uuid.UUID, *apperr.Error) {
	id, err := uuid.Parse(c.GetString(CtxKeyOrgID))
	if err != nil {
		return uuid.Nil, apperr.Unauthorized(apperr.CodeInvalidToken, "missing or invalid org_id")
	}
	return id, nil
}

// UserID returns the authenticated user set by the JWT middleware.
func UserID(c *gin.Context) (uuid.UUID, *apperr.Error) {
	id, err := uuid.Parse(c.GetString(CtxKeyUserID))
	if err != nil {
		return uuid.Nil, apperr.Unauthorized(apperr.CodeInvalidToken, "missing or invalid user_id")
	}
	return id, nil
}

// OrgAndUser returns both authenticated IDs.
func OrgAndUser(c *gin.Context) (orgID, userID uuid.UUID, apiErr *apperr.Error) {
	if orgID, apiErr = OrgID(c); apiErr != nil {
		return uuid.Nil, uuid.Nil, apiErr
	}
	if userID, apiErr = UserID(c); apiErr != nil {
		return uuid.Nil, uuid.Nil, apiErr
	}
	return orgID, userID, nil
}

// BearerToken extracts the token from an "Authorization: Bearer <token>" header.
func BearerToken(c *gin.Context) (string, *apperr.Error) {
	header := c.GetHeader(HeaderAuthorization)
	if header == "" {
		return "", apperr.Unauthorized(apperr.CodeInvalidToken, "missing authorization header")
	}
	scheme, token, ok := strings.Cut(header, " ")
	if !ok || scheme != BearerScheme || token == "" {
		return "", apperr.Unauthorized(apperr.CodeInvalidToken, "invalid authorization format, use Bearer <token>")
	}
	return token, nil
}

// Abort writes err as the response and stops the handler chain.
func Abort(c *gin.Context, err error) {
	Handle(c, err)
	c.Abort()
}

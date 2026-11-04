package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/ctxkeys"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// TokenParser is implemented by the auth session service.
type TokenParser interface {
	ParseToken(tokenString string) (userID, orgID, email string, err error)
}

// JWTAuth validates the bearer access token on protected routes and stores
// the caller's identity on both the Gin and the request context.
func JWTAuth(parser TokenParser) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, apiErr := httpx.BearerToken(c)
		if apiErr != nil {
			httpx.Abort(c, apiErr)
			return
		}
		userID, orgID, email, err := parser.ParseToken(token)
		if err != nil {
			httpx.Abort(c, apperr.Unauthorized(apperr.CodeInvalidToken, "invalid or expired token"))
			return
		}

		c.Set(httpx.CtxKeyUserID, userID)
		c.Set(httpx.CtxKeyOrgID, orgID)
		c.Set(httpx.CtxKeyEmail, email)
		ctx := ctxkeys.WithUserID(ctxkeys.WithOrgID(c.Request.Context(), orgID), userID)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

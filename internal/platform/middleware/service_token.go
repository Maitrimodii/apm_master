package middleware

import (
	"crypto/subtle"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// ServiceTokenAuth checks "Authorization: Bearer <token>" against the shared
// service token. It protects the /internal/v1 routes used by apm-ai-service.
func ServiceTokenAuth(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, apiErr := httpx.BearerToken(c)
		if apiErr != nil {
			httpx.Abort(c, apiErr)
			return
		}
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			httpx.Abort(c, apperr.Unauthorized(apperr.CodeInvalidToken, "invalid service token"))
			return
		}
		c.Next()
	}
}

package middleware

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/ctxkeys"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// APIKeyValidator is implemented by the auth API-key service.
type APIKeyValidator interface {
	ValidateAPIKey(ctx context.Context, rawToken string) (orgID uuid.UUID, err error)
}

// APIKeyAuth validates the X-API-Key header for telemetry ingestion routes.
func APIKeyAuth(validator APIKeyValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader(httpx.HeaderAPIKey)
		if apiKey == "" {
			httpx.Abort(c, apperr.Unauthorized(apperr.CodeInvalidToken, "missing X-API-Key header"))
			return
		}
		orgID, err := validator.ValidateAPIKey(c.Request.Context(), apiKey)
		if err != nil {
			httpx.Abort(c, apperr.Unauthorized(apperr.CodeAPIKeyInvalid, "invalid or revoked API key"))
			return
		}
		c.Set(httpx.CtxKeyOrgID, orgID.String())
		c.Request = c.Request.WithContext(ctxkeys.WithOrgID(c.Request.Context(), orgID.String()))
		c.Next()
	}
}

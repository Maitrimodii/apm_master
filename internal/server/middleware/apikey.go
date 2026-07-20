package middleware

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// APIKeyValidator is implemented by auth.Service
type APIKeyValidator interface {
	ValidateAPIKey(ctx context.Context, rawToken string) (orgID uuid.UUID, err error)
}

// APIKeyAuth validates the X-API-Key header for telemetry ingestion routes.
func APIKeyAuth(validator APIKeyValidator) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		apiKey := ginContext.GetHeader("X-API-Key")
		if apiKey == "" {
			server.Handle(ginContext, server.Unauthorized(
				server.CodeInvalidToken,
				"missing X-API-Key header",
			))
			ginContext.Abort()
			return
		}

		orgID, err := validator.ValidateAPIKey(ginContext.Request.Context(), apiKey)
		if err != nil {
			server.Handle(ginContext, server.Unauthorized(
				server.CodeInvalidToken,
				"invalid or revoked API key",
			))
			ginContext.Abort()
			return
		}

		// Inject the OrgID so the ingestion handlers know where to save the spans
		ginContext.Set("org_id", orgID.String())
		ginContext.Next()
	}
}

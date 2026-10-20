package middleware

import (
	"context"
	"crypto/subtle"
	"strings"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ServiceTokenAuth returns a Gin middleware that validates
// Authorization: Bearer <token> against cfg.AIServiceToken.
// Returns 401 on missing/wrong token. Used on /internal/v1/ routes only.
func ServiceTokenAuth(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract or generate correlation ID
		correlationIDStr := c.GetHeader("X-Correlation-ID")
		var correlationID uuid.UUID
		if correlationIDStr != "" {
			parsed, err := uuid.Parse(correlationIDStr)
			if err == nil {
				correlationID = parsed
			}
		}
		if correlationID == uuid.Nil {
			correlationID = uuid.New()
		}

		c.Set("correlation_id", correlationID.String())
		c.Header("X-Correlation-ID", correlationID.String())
		ctx := context.WithValue(c.Request.Context(), "correlation_id", correlationID.String())
		c.Request = c.Request.WithContext(ctx)

		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			server.Handle(c, server.Unauthorized(
				server.CodeInvalidToken,
				"missing authorization header",
			))
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			server.Handle(c, server.Unauthorized(
				server.CodeInvalidToken,
				"invalid authorization format, use Bearer <token>",
			))
			c.Abort()
			return
		}

		token := parts[1]
		expected := cfg.AIServiceToken
		if subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
			server.Handle(c, server.Unauthorized(
				server.CodeInvalidToken,
				"invalid service token",
			))
			c.Abort()
			return
		}

		c.Next()
	}
}

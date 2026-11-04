package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/ctxkeys"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// CorrelationID propagates the X-Correlation-ID header used to tie an AI
// investigation's calls together, generating one when it is missing or invalid.
func CorrelationID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := uuid.Parse(c.GetHeader(httpx.HeaderCorrelationID))
		if err != nil {
			id = uuid.New()
		}
		value := id.String()
		c.Set(httpx.CtxKeyCorrelationID, value)
		c.Header(httpx.HeaderCorrelationID, value)
		c.Request = c.Request.WithContext(ctxkeys.WithCorrelationID(c.Request.Context(), value))
		c.Next()
	}
}

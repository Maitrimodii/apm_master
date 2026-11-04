package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/ctxkeys"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// RequestID ensures every request has a correlation ID before any logging happens.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(httpx.HeaderRequestID)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Header(httpx.HeaderRequestID, requestID)
		c.Set(httpx.CtxKeyRequestID, requestID)
		c.Request = c.Request.WithContext(ctxkeys.WithRequestID(c.Request.Context(), requestID))
		c.Next()
	}
}

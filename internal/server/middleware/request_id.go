package middleware

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

// RequestID ensures every request has a correlation ID before any logging happens.
func RequestID() gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		requestID := ginContext.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		ginContext.Header(RequestIDHeader, requestID)
		ginContext.Set("request_id", requestID)

		ctx := logger.WithRequestID(ginContext.Request.Context(), requestID)
		// Maintain raw string key for third-party handler compatibility
		ctx = context.WithValue(ctx, "request_id", requestID)
		ginContext.Request = ginContext.Request.WithContext(ctx)

		ginContext.Next()
	}
}

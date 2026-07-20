package middleware

import (
	"context"

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

		ctx := context.WithValue(ginContext.Request.Context(), "request_id", requestID)
		ginContext.Request = ginContext.Request.WithContext(ctx)

		ginContext.Next()
	}
}

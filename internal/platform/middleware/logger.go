package middleware

import (
	"log/slog"
	"time"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"

	"github.com/gin-gonic/gin"
)

// Logger logs method, path, status, duration for every request.
func Logger(log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}

	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		status := c.Writer.Status()
		attrs := []any{
			"method",
			c.Request.Method,
			"path",
			c.Request.URL.Path,
			"status",
			status,
			"duration_ms",
			time.Since(start).Milliseconds(),
			"client_ip",
			c.ClientIP(),
			"user_agent",
			c.Request.UserAgent(),
			"request_id",
			c.Writer.Header().Get(httpx.HeaderRequestID),
		}

		if len(c.Errors) > 0 {
			attrs = append(attrs, "gin_errors", c.Errors.String())
		}

		if errorType := c.GetString(httpx.CtxKeyErrorType); errorType != "" {
			attrs = append(attrs, "error_type", errorType)
		}

		if errorCode := c.GetString(httpx.CtxKeyErrorCode); errorCode != "" {
			attrs = append(attrs, "error_code", errorCode)
		}

		switch {
		case status >= 500:
			log.Error("http request completed", attrs...)
		case status >= 400:
			log.Warn("http request completed", attrs...)
		default:
			log.Info("http request completed", attrs...)
		}
	}
}

package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// Logger logs method, path, status, duration for every request.
func Logger(log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}

	return func(ginContext *gin.Context) {
		start := time.Now()
		ginContext.Next()

		status := ginContext.Writer.Status()
		attrs := []any{
			"method",
			ginContext.Request.Method,
			"path",
			ginContext.Request.URL.Path,
			"status",
			status,
			"duration_ms",
			time.Since(start).Milliseconds(),
			"client_ip",
			ginContext.ClientIP(),
			"user_agent",
			ginContext.Request.UserAgent(),
			"request_id",
			ginContext.Writer.Header().Get(RequestIDHeader),
		}

		if len(ginContext.Errors) > 0 {
			attrs = append(attrs, "gin_errors", ginContext.Errors.String())
		}

		if errorType := ginContext.GetString("error_type"); errorType != "" {
			attrs = append(attrs, "error_type", errorType)
		}

		if errorCode := ginContext.GetString("error_code"); errorCode != "" {
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

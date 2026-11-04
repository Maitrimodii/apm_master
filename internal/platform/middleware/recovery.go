package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// Recovery catches panics and returns a clean 500 response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}

	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		log.Error(
			"panic recovered",
			"panic", recovered,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"client_ip", c.ClientIP(),
			"request_id", c.Writer.Header().Get(httpx.HeaderRequestID),
			"stack", string(debug.Stack()),
		)

		httpx.Handle(c, apperr.Internal(fmt.Errorf("panic recovered")))
		c.Abort()
	})
}

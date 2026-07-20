package middleware

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/gin-gonic/gin"
)

// Recovery catches panics and returns a clean 500 response.
func Recovery(log *slog.Logger) gin.HandlerFunc {
	if log == nil {
		log = slog.Default()
	}

	return gin.CustomRecovery(func(ginContext *gin.Context, recovered any) {
		log.Error(
			"panic recovered",
			"panic", recovered,
			"method", ginContext.Request.Method,
			"path", ginContext.Request.URL.Path,
			"client_ip", ginContext.ClientIP(),
			"request_id", ginContext.Writer.Header().Get(RequestIDHeader),
			"stack", string(debug.Stack()),
		)

		server.Handle(ginContext, server.Internal(fmt.Errorf("panic recovered")))
		ginContext.Abort()
	})
}

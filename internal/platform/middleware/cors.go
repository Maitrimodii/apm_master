package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

var (
	corsAllowMethods  = strings.Join([]string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead}, ", ")
	corsAllowHeaders  = strings.Join([]string{"Origin", "Content-Type", "Content-Length", "Accept", httpx.HeaderAuthorization, "X-Requested-With", httpx.HeaderRequestID, httpx.HeaderAPIKey, httpx.HeaderCorrelationID}, ", ")
	corsExposeHeaders = strings.Join([]string{"Content-Length", httpx.HeaderRequestID, httpx.HeaderCorrelationID}, ", ")
)

// CORS answers cross-origin requests only for allowed origins. Requests from
// any other origin get no CORS headers, so browsers block them.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(allowedOrigins))
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(strings.TrimSpace(o), "/")] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Methods", corsAllowMethods)
			c.Header("Access-Control-Allow-Headers", corsAllowHeaders)
			c.Header("Access-Control-Expose-Headers", corsExposeHeaders)
			c.Header("Vary", "Origin")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

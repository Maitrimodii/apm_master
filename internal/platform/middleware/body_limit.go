package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps request bodies before Gin attempts to bind JSON.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes > 0 && c.Request.Body != nil {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}

		c.Next()
	}
}

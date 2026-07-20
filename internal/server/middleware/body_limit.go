package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps request bodies before Gin attempts to bind JSON.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		if maxBytes > 0 && ginContext.Request.Body != nil {
			ginContext.Request.Body = http.MaxBytesReader(ginContext.Writer, ginContext.Request.Body, maxBytes)
		}

		ginContext.Next()
	}
}

package middleware

import (
	"context"
	"strings"

	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/gin-gonic/gin"
)

// TokenParser is implemented by auth.Service.
type TokenParser interface {
	ParseToken(tokenString string) (userID, orgID, email string, err error)
}

// JWTAuth validates JWT token on protected routes.
func JWTAuth(parser TokenParser) gin.HandlerFunc {
	return func(ginContext *gin.Context) {
		authHeader := ginContext.GetHeader("Authorization")
		if authHeader == "" {
			server.Handle(ginContext, server.Unauthorized(
				server.CodeInvalidToken,
				"missing authorization header",
			))
			ginContext.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			server.Handle(ginContext, server.Unauthorized(
				server.CodeInvalidToken,
				"invalid authorization format, use Bearer <token>",
			))
			ginContext.Abort()
			return
		}

		userID, orgID, email, err := parser.ParseToken(parts[1])
		if err != nil {
			server.Handle(ginContext, server.Unauthorized(
				server.CodeInvalidToken,
				"invalid or expired token",
			))
			ginContext.Abort()
			return
		}

		ginContext.Set("user_id", userID)
		ginContext.Set("org_id", orgID)
		ginContext.Set("email", email)

		ctx := ginContext.Request.Context()
		ctx = context.WithValue(ctx, "user_id", userID)
		ctx = context.WithValue(ctx, "org_id", orgID)
		ginContext.Request = ginContext.Request.WithContext(ctx)

		ginContext.Next()
	}
}

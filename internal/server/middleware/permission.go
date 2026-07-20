package middleware

import (
	"context"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type PermissionChecker interface {
	CheckPermission(
		ctx context.Context,
		userID uuid.UUID,
		orgID uuid.UUID,
		name string,
		action models.PermissionAction,
	) (bool, error)
}

func RequirePermission(
	checker PermissionChecker,
	name string,
	action models.PermissionAction,
) gin.HandlerFunc {
	return func(ginContext *gin.Context) {

		userIDStr := ginContext.GetString(
			"user_id",
		)

		userID, err := uuid.Parse(
			userIDStr,
		)
		if err != nil {
			server.Handle(
				ginContext,
				server.Unauthorized(
					server.CodeInvalidToken,
					"invalid token",
				),
			)
			ginContext.Abort()
			return
		}

		orgIDStr := ginContext.GetString(
			"org_id",
		)

		orgID, err := uuid.Parse(
			orgIDStr,
		)
		if err != nil {
			server.Handle(
				ginContext,
				server.Unauthorized(
					server.CodeInvalidToken,
					"invalid token",
				),
			)
			ginContext.Abort()
			return
		}

		has, err := checker.CheckPermission(
			ginContext.Request.Context(),
			userID,
			orgID,
			name,
			action,
		)
		if err != nil {
			server.Handle(
				ginContext,
				server.Internal(err),
			)
			ginContext.Abort()
			return
		}

		if !has {
			server.Handle(
				ginContext,
				server.Forbidden(
					server.CodeForbidden,
					"permission denied",
				),
			)
			ginContext.Abort()
			return
		}

		ginContext.Next()
	}
}

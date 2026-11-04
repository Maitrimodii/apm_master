package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

// RequirePermission rejects the request unless the authenticated user may
// perform action on resource. It must run after JWTAuth.
func RequirePermission(checker authz.Checker, resource authz.Resource, action authz.Action) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, userID, apiErr := httpx.OrgAndUser(c)
		if apiErr != nil {
			httpx.Abort(c, apiErr)
			return
		}
		allowed, err := checker.CheckPermission(c.Request.Context(), userID, orgID, resource, action)
		if err != nil {
			httpx.Abort(c, apperr.Internal(err))
			return
		}
		if !allowed {
			httpx.Abort(c, apperr.Forbidden(apperr.CodePermissionDenied, "permission denied"))
			return
		}
		c.Next()
	}
}

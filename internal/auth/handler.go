package auth

import (
	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// Services are the auth services the HTTP handler calls.
type Services struct {
	Sessions *SessionService
	Accounts *AccountService
	Invites  *InviteService
	Access   *AccessService
	APIKeys  *APIKeyService
	Authz    *AuthzService
}

// Handler serves /api/v1/auth. Endpoints live in handler_session.go,
// handler_invites.go, handler_roles.go and handler_apikeys.go.
type Handler struct {
	sessions  *SessionService
	accounts  *AccountService
	invites   *InviteService
	access    *AccessService
	apiKeys   *APIKeyService
	authz     *AuthzService
	rateLimit gin.HandlerFunc
}

func NewHandler(svc Services, rateLimit gin.HandlerFunc) *Handler {
	return &Handler{
		sessions:  svc.Sessions,
		accounts:  svc.Accounts,
		invites:   svc.Invites,
		access:    svc.Access,
		apiKeys:   svc.APIKeys,
		authz:     svc.Authz,
		rateLimit: rateLimit,
	}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	public := rg.Group("/auth")
	public.POST("/register", h.RegisterOrgOwner)
	public.POST("/login", h.Login)
	public.POST("/refresh-token", h.RefreshToken)
	public.POST("/logout", h.Logout)
	public.POST("/forgot-password", h.ForgotPassword)
	public.POST("/reset-password", h.ResetPassword)
	public.GET("/invite", h.GetInviteByToken)
	public.POST("/invite/accept", h.AcceptInvite)

	protected := public.Group("", middleware.JWTAuth(h.sessions))
	if h.rateLimit != nil {
		protected.Use(h.rateLimit)
	}
	can := func(r authz.Resource, a authz.Action) gin.HandlerFunc {
		return middleware.RequirePermission(h.authz, r, a)
	}

	protected.GET("/me", h.Me)
	protected.POST("/invite", can(authz.Invites, authz.Write), h.CreateInvite)
	protected.GET("/invites", can(authz.Invites, authz.Read), h.ListInvites)
	protected.POST("/invites/:id/resend", can(authz.Invites, authz.Write), h.ResendInvite)
	protected.DELETE("/invites/:id", can(authz.Invites, authz.Delete), h.CancelInvite)
	protected.GET("/permissions", can(authz.Permissions, authz.Read), h.GetPermissions)
	protected.POST("/roles", can(authz.Roles, authz.Write), h.CreateRole)
	protected.GET("/roles", can(authz.Roles, authz.Read), h.GetRoles)
	protected.GET("/roles/:id", can(authz.Roles, authz.Read), h.GetRole)
	protected.PUT("/roles/:id", can(authz.Roles, authz.Update), h.UpdateRole)
	protected.POST("/api-keys", can(authz.APIKeys, authz.Write), h.CreateAPIKey)
	protected.GET("/api-keys", can(authz.APIKeys, authz.Read), h.GetAPIKeys)
	protected.DELETE("/api-keys/:id", can(authz.APIKeys, authz.Delete), h.RevokeAPIKey)
}

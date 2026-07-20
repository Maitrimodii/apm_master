package transport

import (
	"github.com/Maitrimodii/apm_master/internal/auth/dto"
	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/auth/services"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	IdentityService *services.IdentityService
	InviteService   *services.InviteService
	AccessService   *services.AccessService
}

func NewHandler(identityService *services.IdentityService, inviteService *services.InviteService, accessService *services.AccessService) *Handler {
	return &Handler{
		IdentityService: identityService,
		InviteService:   inviteService,
		AccessService:   accessService,
	}
}

func (handler *Handler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	authGroup := routerGroup.Group("/auth")
	authGroup.POST("/register", handler.RegisterOrgOwner)
	authGroup.POST("/login", handler.Login)
	authGroup.POST("/refresh-token", handler.RefreshToken)
	authGroup.POST("/logout", handler.Logout)
	authGroup.POST("/forgot-password", handler.ForgotPassword)
	authGroup.POST("/reset-password", handler.ResetPassword)
	authGroup.GET("/invite", handler.GetInviteByToken)
	authGroup.POST("/invite/accept", handler.AcceptInvite)

	protected := authGroup.Group("")
	protected.Use(
		middleware.JWTAuth(
			handler.IdentityService,
		),
	)

	protected.GET("/me", handler.Me)
	protected.POST("/invite",
		middleware.RequirePermission(
			handler.IdentityService,
			"invite",
			models.Write,
		),
		handler.CreateInvite,
	)
	protected.GET(
		"/permissions",
		middleware.RequirePermission(
			handler.IdentityService,
			"permission",
			models.Read,
		),
		handler.GetPermissions,
	)
	protected.POST(
		"/roles",
		middleware.RequirePermission(
			handler.IdentityService,
			"role",
			models.Write,
		),
		handler.CreateRole,
	)
	protected.GET(
		"/roles",
		middleware.RequirePermission(
			handler.IdentityService,
			"role",
			models.Read,
		),
		handler.GetRoles,
	)
	protected.GET(
		"/roles/:id",
		middleware.RequirePermission(
			handler.IdentityService,
			"role",
			models.Read,
		),
		handler.GetRole,
	)
	protected.PUT(
		"/roles/:id",
		middleware.RequirePermission(
			handler.IdentityService,
			"role",
			models.Update,
		),
		handler.UpdateRole,
	)
	protected.POST(
		"/api-keys",
		middleware.RequirePermission(
			handler.IdentityService,
			"api_key",
			models.Write,
		),
		handler.CreateAPIKey,
	)
	protected.GET(
		"/api-keys",
		middleware.RequirePermission(
			handler.IdentityService,
			"api_key",
			models.Read,
		),
		handler.GetAPIKeys,
	)
	protected.DELETE(
		"/api-keys/:id",
		middleware.RequirePermission(
			handler.IdentityService,
			"api_key",
			models.Delete,
		),
		handler.RevokeAPIKey,
	)
}

func (handler *Handler) RegisterOrgOwner(ginContext *gin.Context) {
	var req dto.RegisterOrgOwnerRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := handler.IdentityService.RegisterOrgOwner(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.Created(
		ginContext,
		resp,
	)
}

func (handler *Handler) Login(ginContext *gin.Context) {
	var req dto.LoginRequest

	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	userAgent := ginContext.Request.UserAgent()
	ipAddress := ginContext.ClientIP()

	req.UserAgent = &userAgent
	req.IPAddress = &ipAddress

	resp, err := handler.IdentityService.Login(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) RefreshToken(ginContext *gin.Context) {
	var req dto.RefreshTokenRequest

	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	userAgent := ginContext.Request.UserAgent()
	ipAddress := ginContext.ClientIP()

	req.UserAgent = &userAgent
	req.IPAddress = &ipAddress

	resp, err := handler.IdentityService.RefreshToken(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) Logout(ginContext *gin.Context) {
	var req dto.LogoutRequest

	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	err := handler.IdentityService.Logout(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(
		ginContext,
		"logged out successfully",
	)
}

func (handler *Handler) ForgotPassword(ginContext *gin.Context) {
	var req dto.ForgotPasswordRequest

	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := handler.IdentityService.ForgotPassword(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) ResetPassword(ginContext *gin.Context) {
	var req dto.ResetPasswordRequest

	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := handler.IdentityService.ResetPassword(ginContext.Request.Context(), req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) Me(
	ginContext *gin.Context,
) {
	userIDStr := ginContext.GetString(
		"user_id",
	)

	orgIDStr := ginContext.GetString(
		"org_id",
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
		return
	}

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
		return
	}

	resp, err := handler.IdentityService.Me(
		ginContext.Request.Context(),
		dto.MeRequest{
			UserID: userID,
			OrgID:  orgID,
		},
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) CreateInvite(
	ginContext *gin.Context,
) {
	var req dto.InviteUserRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	userID, err := uuid.Parse(
		ginContext.GetString(
			"user_id",
		),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString(
			"org_id",
		),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	req.UserID = userID
	req.OrgID = orgID

	resp, err := handler.InviteService.CreateInvite(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.Created(
		ginContext,
		resp,
	)
}

func (handler *Handler) GetInviteByToken(
	ginContext *gin.Context,
) {
	var req dto.GetInviteRequest

	if err := ginContext.ShouldBindQuery(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request query",
			),
		)
		return
	}

	resp, err := handler.InviteService.GetInviteByToken(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) AcceptInvite(
	ginContext *gin.Context,
) {
	var req dto.AcceptInviteRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	resp, err := handler.InviteService.AcceptInvite(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) GetPermissions(
	ginContext *gin.Context,
) {
	resp, err := handler.AccessService.GetPermissions(
		ginContext.Request.Context(),
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) CreateRole(
	ginContext *gin.Context,
) {
	var req dto.CreateRoleRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	req.OrgID = orgID

	resp, err := handler.AccessService.CreateRole(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.Created(
		ginContext,
		resp,
	)

}

func (handler *Handler) GetRoles(
	ginContext *gin.Context,
) {
	OrgID, err := uuid.Parse(
		ginContext.GetString(
			"org_id",
		),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	resp, err := handler.AccessService.GetRoles(
		ginContext.Request.Context(),
		dto.GetRolesRequest{
			OrgID: OrgID,
		},
	)

	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) GetRole(
	ginContext *gin.Context,
) {
	roleID, err := uuid.Parse(
		ginContext.Param("id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid role id",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	resp, err := handler.AccessService.GetRole(
		ginContext.Request.Context(),
		dto.GetRoleRequest{
			RoleID: roleID,
			OrgID:  orgID,
		},
	)

	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) UpdateRole(
	ginContext *gin.Context,
) {
	var req dto.UpdateRoleRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	roleID, err := uuid.Parse(
		ginContext.Param("id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid role id",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	req.RoleID = roleID
	req.OrgID = orgID

	resp, err := handler.AccessService.UpdateRole(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) CreateAPIKey(
	ginContext *gin.Context,
) {
	var req dto.CreateAPIKeyRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	req.OrgID = orgID

	resp, err := handler.AccessService.CreateAPIKey(
		ginContext.Request.Context(),
		req,
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.Created(
		ginContext,
		resp,
	)
}

func (handler *Handler) GetAPIKeys(
	ginContext *gin.Context,
) {
	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	resp, err := handler.AccessService.GetAPIKeys(
		ginContext.Request.Context(),
		dto.GetAPIKeysRequest{
			OrgID: orgID,
		},
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		resp,
	)
}

func (handler *Handler) RevokeAPIKey(
	ginContext *gin.Context,
) {

	apiKeyID, err := uuid.Parse(
		ginContext.Param("id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeValidation,
				"invalid api key id",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		ginContext.GetString("org_id"),
	)
	if err != nil {
		server.Handle(
			ginContext,
			server.Unauthorized(
				server.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	err = handler.AccessService.RevokeAPIKey(
		ginContext.Request.Context(),
		dto.RevokeAPIKeyRequest{
			APIKeyID: apiKeyID,
			OrgID:    orgID,
		},
	)
	if err != nil {
		server.Handle(
			ginContext,
			err,
		)
		return
	}

	server.OK(
		ginContext,
		"api key revoked successfully",
	)
}

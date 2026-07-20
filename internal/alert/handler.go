package alert

import (
	"strconv"
	"time"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service           *Service
	permissionChecker middleware.PermissionChecker
	tokenParser       middleware.TokenParser
}

func NewHandler(service *Service, permissionChecker middleware.PermissionChecker, tokenParser middleware.TokenParser) *Handler {
	return &Handler{service: service, permissionChecker: permissionChecker, tokenParser: tokenParser}
}

func (handler *Handler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	g := routerGroup.Group("/alerts")
	g.Use(middleware.JWTAuth(handler.tokenParser))

	g.POST("", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Write), handler.Create)
	g.GET("", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Read), handler.List)
	g.GET("/:id", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Read), handler.Get)
	g.PUT("/:id", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Update), handler.Update)
	g.DELETE("/:id", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Delete), handler.Delete)
	g.POST("/:id/share", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Write), handler.Share)
	g.POST("/:id/acknowledge", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Update), handler.Acknowledge)

	group := routerGroup.Group("/alert-instances")
	group.Use(middleware.JWTAuth(handler.tokenParser))
	group.GET("", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Read), handler.ListInstances)
	group.GET("/:id", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Read), handler.GetInstanceDetails)
	group.POST("/:id/acknowledge", middleware.RequirePermission(handler.permissionChecker, "alerts", models.Update), handler.AcknowledgeInstance)
}

func (handler *Handler) Create(ginContext *gin.Context) {
	var req CreateAlertRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	orgID, userID, err := authContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	resp, err := handler.service.Create(ginContext.Request.Context(), orgID, userID, req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.Created(ginContext, resp)
}

func (handler *Handler) List(ginContext *gin.Context) {
	orgID, _, err := authContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	page, limit := pagination(ginContext)
	resp, total, err := handler.service.List(ginContext.Request.Context(), orgID, page, limit)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.List(ginContext, resp, total, page, limit)
}

func (handler *Handler) Get(ginContext *gin.Context) {
	orgID, _, alertID, err := routeContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	resp, err := handler.service.Get(ginContext.Request.Context(), orgID, alertID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) Update(ginContext *gin.Context) {
	var req UpdateAlertRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	orgID, _, alertID, err := routeContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	resp, err := handler.service.Update(ginContext.Request.Context(), orgID, alertID, req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) Delete(ginContext *gin.Context) {
	orgID, _, alertID, err := routeContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	if err := handler.service.Delete(ginContext.Request.Context(), orgID, alertID); err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.NoContent(ginContext)
}

func (handler *Handler) Share(ginContext *gin.Context) {
	var req ShareAlertRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	orgID, userID, alertID, err := routeContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	resp, err := handler.service.Share(ginContext.Request.Context(), orgID, userID, alertID, req)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) Acknowledge(ginContext *gin.Context) {
	orgID, _, alertID, err := routeContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	resp, err := handler.service.Acknowledge(ginContext.Request.Context(), orgID, alertID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func routeContext(ginContext *gin.Context) (uuid.UUID, uuid.UUID, uuid.UUID, error) {
	orgID, userID, err := authContext(ginContext)
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, err
	}

	alertID, err := uuid.Parse(ginContext.Param("id"))
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, server.BadRequest(server.CodeValidation, "invalid alert id")
	}

	return orgID, userID, alertID, nil
}

func authContext(ginContext *gin.Context) (uuid.UUID, uuid.UUID, error) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, server.Unauthorized(server.CodeInvalidToken, "invalid token")
	}

	userID, err := uuid.Parse(ginContext.GetString("user_id"))
	if err != nil {
		return uuid.UUID{}, uuid.UUID{}, server.Unauthorized(server.CodeInvalidToken, "invalid token")
	}

	return orgID, userID, nil
}

func pagination(ginContext *gin.Context) (int, int) {
	page, _ := strconv.Atoi(ginContext.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ginContext.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return page, limit
}

func (handler *Handler) ListInstances(ginContext *gin.Context) {
	orgID, _, err := authContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	state := ginContext.Query("state")
	severity := ginContext.Query("severity")

	var alertID uuid.UUID
	if alertIDStr := ginContext.Query("alert_id"); alertIDStr != "" {
		alertID, err = uuid.Parse(alertIDStr)
		if err != nil {
			server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid alert_id format"))
			return
		}
	}

	var fromTime *time.Time
	if fromStr := ginContext.Query("from"); fromStr != "" {
		parsedTime, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid from time format, must be RFC3339"))
			return
		}
		fromTime = &parsedTime
	}

	var toTime *time.Time
	if toStr := ginContext.Query("to"); toStr != "" {
		parsedTime, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid to time format, must be RFC3339"))
			return
		}
		toTime = &parsedTime
	}

	page, _ := strconv.Atoi(ginContext.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ginContext.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	resp, total, err := handler.service.ListInstances(ginContext.Request.Context(), orgID, state, severity, alertID, fromTime, toTime, page, pageSize)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.List(ginContext, resp, total, page, pageSize)
}

func (handler *Handler) GetInstanceDetails(ginContext *gin.Context) {
	orgID, _, err := authContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	instanceID, err := uuid.Parse(ginContext.Param("id"))
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid alert instance id"))
		return
	}

	resp, err := handler.service.GetInstanceDetails(ginContext.Request.Context(), orgID, instanceID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) AcknowledgeInstance(ginContext *gin.Context) {
	orgID, _, err := authContext(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	instanceID, err := uuid.Parse(ginContext.Param("id"))
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "invalid alert instance id"))
		return
	}

	resp, err := handler.service.AcknowledgeInstance(ginContext.Request.Context(), orgID, instanceID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

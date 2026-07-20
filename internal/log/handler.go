package log

import (
	"fmt"
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
	IdentityService   middleware.PermissionChecker
	IdentityValidator middleware.TokenParser
}

func NewHandler(service *Service, identityService middleware.PermissionChecker, identityValidator middleware.TokenParser) *Handler {
	return &Handler{service: service, IdentityService: identityService, IdentityValidator: identityValidator}
}

func (handler *Handler) RegisterRoutes(router *gin.RouterGroup) {
	logGroup := router.Group("/logs")

	logGroup.Use(
		middleware.JWTAuth(
			handler.IdentityValidator,
		),
	)

	logGroup.GET(
		"",
		middleware.RequirePermission(
			handler.IdentityService,
			"logs",
			models.Read,
		),
		handler.handleListLogs,
	)

	logGroup.GET(
		"/trace/:traceId",
		middleware.RequirePermission(
			handler.IdentityService,
			"logs",
			models.Read,
		),
		handler.handleGetTraceLogs,
	)
}

func (handler *Handler) handleListLogs(ginContext *gin.Context) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))

	if err != nil {
		server.Handle(ginContext, server.Unauthorized(server.CodeInvalidToken, "missing or invalid org_id"))
		return
	}

	params, err := parseListParams(ginContext)
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, err.Error()))
		return
	}

	records, total, err := handler.service.ListLogs(ginContext.Request.Context(), orgID.String(), params)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.List(ginContext, records, total, params.Page, params.Limit)
}

func (handler *Handler) handleGetTraceLogs(ginContext *gin.Context) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))
	if err != nil {
		server.Handle(ginContext, server.Unauthorized(server.CodeInvalidToken, "missing or invalid org_id"))
		return
	}

	traceID := ginContext.Param("traceId")
	if traceID == "" {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "missing trace_id"))
		return
	}

	records, err := handler.service.GetTraceLogs(ginContext.Request.Context(), orgID.String(), traceID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, records)
}
func parseListParams(ginContext *gin.Context) (ListParams, error) {
	now := time.Now().UTC()
	fromDefault := now.Add(-1 * time.Hour)

	fromStr := ginContext.DefaultQuery("from", fromDefault.Format(time.RFC3339))
	toStr := ginContext.DefaultQuery("to", now.Format(time.RFC3339))

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return ListParams{}, fmt.Errorf("invalid 'from': %s", fromStr)
	}
	endTime, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return ListParams{}, fmt.Errorf("invalid 'to': %s", toStr)
	}
	if endTime.Before(from) {
		return ListParams{}, fmt.Errorf("'to' must be after 'from'")
	}

	page, _ := strconv.Atoi(ginContext.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ginContext.DefaultQuery("limit", "100"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	minSev, _ := strconv.Atoi(ginContext.DefaultQuery("min_severity", "0"))

	return ListParams{
		From:        &from,
		To:          &endTime,
		ServiceName: ginContext.Query("service"),
		MinSeverity: minSev,
		TraceID:     ginContext.Query("trace_id"),
		Search:      ginContext.Query("q"),
		Page:        page,
		Offset:      (page - 1) * limit,
		Limit:       limit,
	}, nil
}

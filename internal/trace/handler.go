package trace

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
	identityService   middleware.PermissionChecker
	identityValidator middleware.TokenParser
}

func NewHandler(service *Service, identityService middleware.PermissionChecker, identityValidator middleware.TokenParser) *Handler {
	return &Handler{service: service, identityService: identityService, identityValidator: identityValidator}
}

func (handler *Handler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	g := routerGroup.Group("/traces")

	g.Use(
		middleware.JWTAuth(
			handler.identityValidator,
		),
	)

	g.GET(
		"",
		middleware.RequirePermission(
			handler.identityService,
			"traces",
			models.Read,
		),
		handler.ListSpans,
	)

	g.GET(
		"/:traceID",
		middleware.RequirePermission(
			handler.identityService,
			"traces",
			models.Read,
		),
		handler.GetTrace,
	)
}

func (handler *Handler) GetTrace(ginContext *gin.Context) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))
	if err != nil {
		server.Handle(ginContext, server.Unauthorized(server.CodeInvalidToken, "missing or invalid org_id"))
		return
	}

	traceID := ginContext.Param("traceID")
	if traceID == "" {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "trace_id is required"))
		return
	}

	spans, err := handler.service.GetTrace(ginContext.Request.Context(), orgID, traceID)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, spans)
}

func (handler *Handler) ListSpans(ginContext *gin.Context) {
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

	summaries, total, err := handler.service.ListSpans(ginContext.Request.Context(), orgID, params)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.List(ginContext, summaries, total, params.Page, params.Limit)
}

func parseListParams(ginContext *gin.Context) (ListParams, error) {
	now := time.Now().UTC()

	fromStr := ginContext.DefaultQuery("from", now.Add(-1*time.Hour).Format(time.RFC3339))
	toStr := ginContext.DefaultQuery("to", now.Format(time.RFC3339))

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return ListParams{}, fmt.Errorf("invalid 'from' timestamp: %s", fromStr)
	}

	endTime, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return ListParams{}, fmt.Errorf("invalid 'to' timestamp: %s", toStr)
	}

	if endTime.Before(from) {
		return ListParams{}, fmt.Errorf("'to' must be after 'from'")
	}

	page, _ := strconv.Atoi(ginContext.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(ginContext.DefaultQuery("limit", "50"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 1000 {
		limit = 50
	}

	minDuration := 0.0
	if minDurationStr := ginContext.Query("min_duration"); minDurationStr != "" {
		minDuration, _ = strconv.ParseFloat(minDurationStr, 64)
	}
	maxDuration := 0.0
	if maxDurationStr := ginContext.Query("max_duration"); maxDurationStr != "" {
		maxDuration, _ = strconv.ParseFloat(maxDurationStr, 64)
	}

	return ListParams{
		From:          from,
		To:            endTime,
		ServiceName:   ginContext.Query("service"),
		TraceID:       ginContext.Query("trace_id"),
		Endpoint:      ginContext.Query("endpoint"),
		StatusCode:    ginContext.Query("status"),
		MinDurationMS: minDuration,
		MaxDurationMS: maxDuration,
		Page:          page,
		Offset:        (page - 1) * limit,
		Limit:         limit,
	}, nil
}

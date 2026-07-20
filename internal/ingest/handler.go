package ingest

import (
	"github.com/Maitrimodii/apm_master/internal/ingest/otlp"
	"github.com/Maitrimodii/apm_master/internal/ingest/services"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler receives all OTel signals over HTTP.
// Each signal has its own pipeline — they are independently buffered,
// independently flushed, and independently writable.
type Handler struct {
	service         *services.Service
	identityService middleware.APIKeyValidator
}

func NewHandler(
	service *services.Service,
	identityService middleware.APIKeyValidator,
) *Handler {
	return &Handler{
		service:         service,
		identityService: identityService,
	}
}

func (handler *Handler) RegisterRoutes(
	routerGroup *gin.RouterGroup,
) {
	ingest := routerGroup.Group("/ingest")

	ingest.Use(
		middleware.APIKeyAuth(
			handler.identityService,
		),
	)

	ingest.POST(
		"/traces",
		handler.Traces,
	)

	ingest.POST(
		"/logs",
		handler.Logs,
	)

	ingest.POST(
		"/metrics",
		handler.Metrics,
	)
}

func (handler *Handler) Traces(
	ginContext *gin.Context,
) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	var req otlp.IngestRequest

	if err := ginContext.ShouldBindJSON(
		&req,
	); err != nil {

		server.Handle(
			ginContext,
			server.BadRequest(
				server.CodeInvalidPayload,
				"invalid request body",
			),
		)
		return
	}

	resp, err := handler.service.IngestTraces(
		ginContext.Request.Context(),
		orgID,
		req,
	)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) Logs(
	ginContext *gin.Context,
) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	var req otlp.IngestLogsRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeInvalidPayload, "invalid request body"))
		return
	}

	res, err := handler.service.IngestLogs(
		ginContext.Request.Context(),
		orgID,
		req,
	)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, res)
}

func (handler *Handler) Metrics(
	ginContext *gin.Context,
) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	var req otlp.IngestMetricsRequest
	if err := ginContext.ShouldBindJSON(&req); err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeInvalidPayload, "invalid request body"))
		return
	}

	resp, err := handler.service.IngestMetrics(
		ginContext.Request.Context(),
		orgID,
		req,
	)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func parseOrgID(ginContext *gin.Context) (uuid.UUID, error) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))
	if err != nil {
		return uuid.UUID{}, server.Unauthorized(server.CodeInvalidToken, "missing or invalid org_id")
	}
	return orgID, nil
}

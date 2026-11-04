package ingest

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/ingest/otlp"
	"github.com/Maitrimodii/apm_master/internal/ingest/services"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// Handler accepts OTLP-style JSON telemetry authenticated by API key.
type Handler struct {
	service *services.Service
	apiKeys middleware.APIKeyValidator
}

func NewHandler(service *services.Service, apiKeys middleware.APIKeyValidator) *Handler {
	return &Handler{service: service, apiKeys: apiKeys}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("/ingest", middleware.APIKeyAuth(h.apiKeys))
	g.POST("/traces", h.Traces)
	g.POST("/logs", h.Logs)
	g.POST("/metrics", h.Metrics)
}

func (h *Handler) Traces(c *gin.Context) {
	var req otlp.IngestRequest
	ingest(c, &req, func(org uuid.UUID) (*otlp.IngestResponse, error) {
		return h.service.IngestTraces(c.Request.Context(), org, req)
	})
}

func (h *Handler) Logs(c *gin.Context) {
	var req otlp.IngestLogsRequest
	ingest(c, &req, func(org uuid.UUID) (*otlp.IngestResponse, error) {
		return h.service.IngestLogs(c.Request.Context(), org, req)
	})
}

func (h *Handler) Metrics(c *gin.Context) {
	var req otlp.IngestMetricsRequest
	ingest(c, &req, func(org uuid.UUID) (*otlp.IngestResponse, error) {
		return h.service.IngestMetrics(c.Request.Context(), org, req)
	})
}

// ingest binds the request body into req, then runs the signal's ingest call
// for the authenticated org.
func ingest(c *gin.Context, req any, run func(org uuid.UUID) (*otlp.IngestResponse, error)) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	if err := c.ShouldBindJSON(req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeInvalidPayload, "invalid request body"))
		return
	}
	resp, err := run(orgID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

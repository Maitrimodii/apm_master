package investigation

import (
	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// InternalHandler serves the service-to-service API under /internal/v1 that
// apm-ai-service calls while running an investigation.
type InternalHandler struct {
	service      *Service
	evidence     *EvidenceService
	serviceToken string
}

func NewInternalHandler(service *Service, evidence *EvidenceService, serviceToken string) *InternalHandler {
	return &InternalHandler{service: service, evidence: evidence, serviceToken: serviceToken}
}

func (h *InternalHandler) RegisterRoutes(rg *gin.RouterGroup) {
	g := rg.Group("", middleware.CorrelationID(), middleware.ServiceTokenAuth(h.serviceToken))
	g.GET("/incidents/:incident_id/context", h.GetIncidentContext)
	g.POST("/investigation-evidence/query", h.QueryEvidence)
	g.PUT("/investigations/:id/status", h.UpdateStatus)
	g.POST("/investigations/:id/report", h.SaveReport)
	g.POST("/investigations/:id/events", h.AppendEvent)
}

func (h *InternalHandler) GetIncidentContext(c *gin.Context) {
	incidentID, apiErr := httpx.PathUUID(c, "incident_id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	ic, err := h.service.IncidentContext(c.Request.Context(), incidentID)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, ic)
}

func (h *InternalHandler) QueryEvidence(c *gin.Context) {
	var req EvidenceQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload: "+err.Error()))
		return
	}
	resp, err := h.evidence.Query(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, resp)
}

func (h *InternalHandler) UpdateStatus(c *gin.Context) {
	id, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	if err := h.service.ApplyStatus(c.Request.Context(), id, req.Status, req.FailureCode, req.FailureMessage); err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.NoContent(c)
}

func (h *InternalHandler) SaveReport(c *gin.Context) {
	id, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req SaveReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	if err := h.service.SaveReport(c.Request.Context(), id, req); err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.NoContent(c)
}

func (h *InternalHandler) AppendEvent(c *gin.Context) {
	id, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req AppendEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	if err := h.service.AppendEvent(c.Request.Context(), id, req); err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.NoContent(c)
}

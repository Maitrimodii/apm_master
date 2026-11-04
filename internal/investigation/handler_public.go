package investigation

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// defaultPageSize is the investigations-per-incident page size.
const defaultPageSize = 20

// Handler serves the public investigation API under /api/v1.
type Handler struct {
	service     *Service
	checker     authz.Checker
	tokenParser middleware.TokenParser
	rateLimit   gin.HandlerFunc
}

func NewHandler(service *Service, checker authz.Checker, tokenParser middleware.TokenParser, rateLimit gin.HandlerFunc) *Handler {
	return &Handler{service: service, checker: checker, tokenParser: tokenParser, rateLimit: rateLimit}
}

func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	auth := []gin.HandlerFunc{middleware.JWTAuth(h.tokenParser), middleware.CorrelationID()}
	if h.rateLimit != nil {
		auth = append(auth, h.rateLimit)
	}
	canRead := middleware.RequirePermission(h.checker, authz.Investigations, authz.Read)
	canWrite := middleware.RequirePermission(h.checker, authz.Investigations, authz.Write)

	incidents := rg.Group("/incidents", auth...)
	incidents.POST("/:incident_id/investigations", canWrite, h.Create)
	incidents.GET("/:incident_id/investigations", canRead, h.ListByIncident)

	investigations := rg.Group("/investigations", auth...)
	investigations.GET("/:id", canRead, h.Get)
	investigations.GET("/:id/events", canRead, h.ListEvents)
	investigations.GET("/:id/report", canRead, h.GetReport)
	investigations.GET("/:id/evidence/:evidence_id", canRead, h.GetEvidence)
	investigations.POST("/:id/cancel", canWrite, h.Cancel)
	investigations.POST("/:id/retry", canWrite, h.Retry)
}

func (h *Handler) Create(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	incidentID, apiErr := httpx.PathUUID(c, "incident_id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req CreateInvestigationRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
			return
		}
	}
	inv, err := h.service.Create(c.Request.Context(), orgID, userID, incidentID, correlationID(c), req)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.Accepted(c, inv)
}

func (h *Handler) ListByIncident(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	incidentID, apiErr := httpx.PathUUID(c, "incident_id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	page, apiErr := httpx.ParsePage(c, defaultPageSize)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	list, total, err := h.service.List(c.Request.Context(), orgID, incidentID, page.Size, page.Offset())
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.List(c, list, total, page.Number, page.Size)
}

func (h *Handler) Get(c *gin.Context) {
	orgID, id, apiErr := orgAndID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	inv, err := h.service.Get(c.Request.Context(), orgID, id)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, inv)
}

func (h *Handler) ListEvents(c *gin.Context) {
	orgID, id, apiErr := orgAndID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	events, err := h.service.ListEvents(c.Request.Context(), orgID, id)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	if events == nil {
		events = []InvestigationEvent{}
	}
	httpx.OK(c, events)
}

func (h *Handler) GetReport(c *gin.Context) {
	orgID, id, apiErr := orgAndID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	report, err := h.service.GetReport(c.Request.Context(), orgID, id)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, report)
}

func (h *Handler) GetEvidence(c *gin.Context) {
	orgID, id, apiErr := orgAndID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	evidenceID, apiErr := httpx.PathUUID(c, "evidence_id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	ev, err := h.service.GetEvidence(c.Request.Context(), orgID, id, evidenceID)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, ev)
}

func (h *Handler) Cancel(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	id, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	inv, err := h.service.Cancel(c.Request.Context(), orgID, userID, id)
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.OK(c, inv)
}

func (h *Handler) Retry(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	id, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	inv, err := h.service.Retry(c.Request.Context(), orgID, userID, id, correlationID(c))
	if err != nil {
		httpx.Handle(c, apiErrors.Wrap(err))
		return
	}
	httpx.Accepted(c, inv)
}

func orgAndID(c *gin.Context) (uuid.UUID, uuid.UUID, *apperr.Error) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		return uuid.Nil, uuid.Nil, apiErr
	}
	id, apiErr := httpx.PathUUID(c, "id")
	return orgID, id, apiErr
}

// correlationID returns the request's correlation ID set by the
// CorrelationID middleware.
func correlationID(c *gin.Context) uuid.UUID {
	id, _ := uuid.Parse(c.GetString(httpx.CtxKeyCorrelationID))
	return id
}

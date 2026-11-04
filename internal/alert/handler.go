package alert

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

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
	auth := []gin.HandlerFunc{middleware.JWTAuth(h.tokenParser)}
	if h.rateLimit != nil {
		auth = append(auth, h.rateLimit)
	}
	can := func(a authz.Action) gin.HandlerFunc { return middleware.RequirePermission(h.checker, authz.Alerts, a) }

	alerts := rg.Group("/alerts", auth...)
	alerts.POST("", can(authz.Write), h.Create)
	alerts.GET("", can(authz.Read), h.List)
	alerts.GET("/:id", can(authz.Read), h.Get)
	alerts.PUT("/:id", can(authz.Update), h.Update)
	alerts.DELETE("/:id", can(authz.Delete), h.Delete)
	alerts.POST("/:id/share", can(authz.Write), h.Share)
	alerts.POST("/:id/acknowledge", can(authz.Update), h.Acknowledge)

	instances := rg.Group("/alert-instances", auth...)
	instances.GET("", can(authz.Read), h.ListInstances)
	instances.GET("/:id", can(authz.Read), h.GetInstanceDetails)
	instances.POST("/:id/acknowledge", can(authz.Update), h.AcknowledgeInstance)
}

func (h *Handler) Create(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req CreateAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	resp, err := h.service.Create(c.Request.Context(), orgID, userID, req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.Created(c, resp)
}

func (h *Handler) List(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	page, apiErr := httpx.ParsePage(c, defaultPageSize)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, total, err := h.service.List(c.Request.Context(), orgID, page.Size, page.Offset())
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, resp, total, page.Number, page.Size)
}

func (h *Handler) Get(c *gin.Context) {
	orgID, alertID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.Get(c.Request.Context(), orgID, alertID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Update(c *gin.Context) {
	orgID, alertID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req UpdateAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	resp, err := h.service.Update(c.Request.Context(), orgID, alertID, req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Delete(c *gin.Context) {
	orgID, alertID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	if err := h.service.Delete(c.Request.Context(), orgID, alertID); err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.NoContent(c)
}

func (h *Handler) Share(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	alertID, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	var req ShareAlertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}
	resp, err := h.service.Share(c.Request.Context(), orgID, userID, alertID, req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) Acknowledge(c *gin.Context) {
	orgID, alertID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.Acknowledge(c.Request.Context(), orgID, alertID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) ListInstances(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	filter, page, apiErr := parseInstanceFilter(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	filter.OrgID = orgID
	resp, total, err := h.service.ListInstances(c.Request.Context(), filter)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, resp, total, page.Number, page.Size)
}

func (h *Handler) GetInstanceDetails(c *gin.Context) {
	orgID, instanceID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.GetInstanceDetails(c.Request.Context(), orgID, instanceID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) AcknowledgeInstance(c *gin.Context) {
	orgID, instanceID, apiErr := orgAndPathID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.AcknowledgeInstance(c.Request.Context(), orgID, instanceID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func orgAndPathID(c *gin.Context) (uuid.UUID, uuid.UUID, *apperr.Error) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		return uuid.Nil, uuid.Nil, apiErr
	}
	id, apiErr := httpx.PathUUID(c, "id")
	return orgID, id, apiErr
}

func parseInstanceFilter(c *gin.Context) (InstanceFilter, httpx.Page, *apperr.Error) {
	page, apiErr := httpx.ParsePage(c, defaultPageSize)
	if apiErr != nil {
		return InstanceFilter{}, httpx.Page{}, apiErr
	}
	f := InstanceFilter{
		State:  AlertState(c.Query("state")),
		Limit:  page.Size,
		Offset: page.Offset(),
	}
	if v := c.Query("severity"); v != "" {
		sev, ok := ParseSeverity(v)
		if !ok {
			return InstanceFilter{}, httpx.Page{}, apperr.ValidationError("severity", "must be one of low, medium, high, critical")
		}
		f.Severity = sev
	}
	if v := c.Query("alert_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return InstanceFilter{}, httpx.Page{}, apperr.ValidationError("alert_id", "must be a UUID")
		}
		f.AlertID = id
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := c.Query(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return InstanceFilter{}, httpx.Page{}, apperr.ValidationError(name, "must be an RFC3339 timestamp")
			}
			*dst = &t
		}
	}
	return f, page, nil
}

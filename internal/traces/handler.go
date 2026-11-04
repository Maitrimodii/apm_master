package traces

import (
	"github.com/gin-gonic/gin"

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
	g := rg.Group("/traces", middleware.JWTAuth(h.tokenParser))
	if h.rateLimit != nil {
		g.Use(h.rateLimit)
	}
	canRead := middleware.RequirePermission(h.checker, authz.Traces, authz.Read)
	g.GET("", canRead, h.ListTraces)
	g.GET("/:traceID", canRead, h.GetTrace)
}

func (h *Handler) GetTrace(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	spans, err := h.service.GetTrace(c.Request.Context(), orgID, c.Param("traceID"))
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, spans)
}

func (h *Handler) ListTraces(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	filter, page, apiErr := parseListFilter(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	filter.OrgID = orgID
	summaries, total, err := h.service.ListTraceSummaries(c.Request.Context(), filter)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, summaries, total, page.Number, page.Size)
}

func parseListFilter(c *gin.Context) (ListFilter, httpx.Page, *apperr.Error) {
	tr, apiErr := httpx.ParseTimeRange(c)
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	page, apiErr := httpx.ParsePage(c, httpx.DefaultPageSize)
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	minDuration, apiErr := httpx.QueryFloat(c, "min_duration")
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	maxDuration, apiErr := httpx.QueryFloat(c, "max_duration")
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	return ListFilter{
		From:          tr.From,
		To:            tr.To,
		ServiceName:   c.Query("service"),
		TraceID:       c.Query("trace_id"),
		Endpoint:      c.Query("endpoint"),
		StatusCode:    c.Query("status"),
		MinDurationMS: minDuration,
		MaxDurationMS: maxDuration,
		Limit:         page.Size,
		Offset:        page.Offset(),
	}, page, nil
}

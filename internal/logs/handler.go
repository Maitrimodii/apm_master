package logs

import (
	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// defaultPageSize is larger than the shared default: log views scroll.
const defaultPageSize = 100

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
	g := rg.Group("/logs", middleware.JWTAuth(h.tokenParser))
	if h.rateLimit != nil {
		g.Use(h.rateLimit)
	}
	canRead := middleware.RequirePermission(h.checker, authz.Logs, authz.Read)
	g.GET("", canRead, h.ListLogs)
	g.GET("/trace/:traceID", canRead, h.GetTraceLogs)
}

func (h *Handler) ListLogs(c *gin.Context) {
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
	records, total, err := h.service.ListLogs(c.Request.Context(), filter)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, records, total, page.Number, page.Size)
}

func (h *Handler) GetTraceLogs(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	records, err := h.service.GetTraceLogs(c.Request.Context(), orgID, c.Param("traceID"))
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, records)
}

func parseListFilter(c *gin.Context) (ListFilter, httpx.Page, *apperr.Error) {
	tr, apiErr := httpx.ParseTimeRange(c)
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	page, apiErr := httpx.ParsePage(c, defaultPageSize)
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	minSeverity, apiErr := httpx.QueryInt(c, "min_severity", 0)
	if apiErr != nil {
		return ListFilter{}, httpx.Page{}, apiErr
	}
	return ListFilter{
		From:        tr.From,
		To:          tr.To,
		ServiceName: c.Query("service"),
		MinSeverity: minSeverity,
		TraceID:     c.Query("trace_id"),
		Search:      c.Query("q"),
		Limit:       page.Size,
		Offset:      page.Offset(),
	}, page, nil
}

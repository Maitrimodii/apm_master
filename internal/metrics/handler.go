package metrics

import (
	"strings"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// defaultPageSize is the metric picker's page size.
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
	g := rg.Group("/metrics", middleware.JWTAuth(h.tokenParser))
	if h.rateLimit != nil {
		g.Use(h.rateLimit)
	}
	canRead := middleware.RequirePermission(h.checker, authz.Metrics, authz.Read)
	g.GET("", canRead, h.ListMetricNames)
	g.GET("/:metricName/values", canRead, h.ListMetricValues)
	g.GET("/:metricName/series", canRead, h.GetSeries)
	g.GET("/:metricName/aggregations", canRead, h.GetAggregations)
	g.GET("/:metricName/histogram", canRead, h.GetHistogram)
}

// ListMetricNames lists metric names for the dashboard picker.
func (h *Handler) ListMetricNames(c *gin.Context) {
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
	names, total, err := h.service.ListMetricNames(c.Request.Context(), ListMetricsQuery{
		OrgID:       orgID,
		ServiceName: c.Query("service"),
		Limit:       page.Size,
		Offset:      page.Offset(),
	})
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, names, total, page.Number, page.Size)
}

// ListMetricValues lists raw samples of one metric, newest first.
func (h *Handler) ListMetricValues(c *gin.Context) {
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
	values, total, err := h.service.ListMetricValues(c.Request.Context(), ListMetricValuesQuery{
		OrgID:       orgID,
		MetricName:  c.Param("metricName"),
		ServiceName: c.Query("service"),
		Limit:       page.Size,
		Offset:      page.Offset(),
	})
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, values, total, page.Number, page.Size)
}

func (h *Handler) GetSeries(c *gin.Context) {
	orgID, tr, apiErr := orgAndRange(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.GetSeries(c.Request.Context(), SeriesQuery{
		OrgID:      orgID,
		MetricName: c.Param("metricName"),
		From:       tr.From,
		To:         tr.To,
		Labels:     parseLabelFilters(c),
		Interval:   c.Query("interval"),
	})
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) GetAggregations(c *gin.Context) {
	orgID, tr, apiErr := orgAndRange(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	labels := parseLabelFilters(c)
	if svc := c.Query("service"); svc != "" {
		labels[serviceNameLabel] = svc
	}
	resp, err := h.service.GetAggregations(c.Request.Context(), AggregationQuery{
		OrgID:      orgID,
		MetricName: c.Param("metricName"),
		From:       tr.From,
		To:         tr.To,
		Labels:     labels,
	})
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

func (h *Handler) GetHistogram(c *gin.Context) {
	orgID, tr, apiErr := orgAndRange(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.service.GetHistogram(c.Request.Context(), orgID, c.Param("metricName"), tr)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

// serviceNameLabel is the OTel resource attribute naming the emitting service.
const serviceNameLabel = "service.name"

func orgAndRange(c *gin.Context) (uuid.UUID, httpx.TimeRange, *apperr.Error) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		return uuid.Nil, httpx.TimeRange{}, apiErr
	}
	tr, apiErr := httpx.ParseTimeRange(c)
	return orgID, tr, apiErr
}

// parseLabelFilters reads ?label[key]=value matchers.
func parseLabelFilters(c *gin.Context) map[string]string {
	labels := map[string]string{}
	for key, values := range c.Request.URL.Query() {
		name, ok := strings.CutPrefix(key, "label[")
		if !ok || !strings.HasSuffix(name, "]") || len(values) == 0 {
			continue
		}
		labels[strings.TrimSuffix(name, "]")] = values[0]
	}
	return labels
}

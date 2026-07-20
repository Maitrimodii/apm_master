package metric

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

func (handler *Handler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	g := routerGroup.Group("/metrics")
	// List all metric names for this org — used by dashboard metric picker

	g.Use(
		middleware.JWTAuth(
			handler.IdentityValidator,
		),
	)

	g.GET(
		"",
		middleware.RequirePermission(
			handler.IdentityService,
			"metrics",
			models.Read,
		),
		handler.ListMetricNames,
	)
	// Time series for a single metric — the main chart data endpoint
	g.GET("/:metricName/series",
		middleware.RequirePermission(
			handler.IdentityService,
			"metrics",
			models.Read,
		),
		handler.GetSeries,
	)
	// Aggregations for a metric — count, avg, min, max, percentiles
	g.GET("/:metricName/aggregations",
		middleware.RequirePermission(
			handler.IdentityService,
			"metrics",
			models.Read,
		),
		handler.GetAggregations,
	)
	// Histogram bucket distribution — for latency heatmaps
	g.GET("/:metricName/histogram",
		middleware.RequirePermission(
			handler.IdentityService,
			"metrics",
			models.Read,
		),
		handler.GetHistogramBuckets,
	)
}

func (handler *Handler) ListMetricNames(ginContext *gin.Context) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	page, limit := parsePagination(ginContext)
	metricName := ginContext.Query("name")
	serviceName := ginContext.Query("service")

	if metricName != "" {
		values, total, err := handler.service.ListMetricValues(ginContext.Request.Context(), orgID, ListMetricValuesParams{
			MetricName:  metricName,
			ServiceName: serviceName,
			Limit:       limit,
			Offset:      (page - 1) * limit,
		})
		if err != nil {
			server.Handle(ginContext, err)
			return
		}
		server.List(ginContext, values, total, page, limit)
		return
	}

	names, total, err := handler.service.ListMetricNames(ginContext.Request.Context(), orgID, ListParams{
		ServiceName: serviceName,
		Limit:       limit,
		Offset:      (page - 1) * limit,
	})
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.List(ginContext, names, total, page, limit)
}

func (handler *Handler) GetSeries(ginContext *gin.Context) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	metricName := ginContext.Param("metricName")
	if metricName == "" {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "metric_name is required"))
		return
	}

	timeRange, err := parseTimeRange(ginContext)
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, err.Error()))
		return
	}

	labels := parseLabelFilters(ginContext)

	resp, err := handler.service.GetSeries(ginContext.Request.Context(), orgID, SeriesParams{
		MetricName: metricName,
		From:       timeRange.from,
		To:         timeRange.endTime,
		Labels:     labels,
		Interval:   ginContext.Query("interval"),
	})
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) GetAggregations(ginContext *gin.Context) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	metricName := ginContext.Param("metricName")
	if metricName == "" {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "metric_name is required"))
		return
	}

	timeRange, err := parseTimeRange(ginContext)
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, err.Error()))
		return
	}

	resp, err := handler.service.GetAggregations(ginContext.Request.Context(), orgID, AggregationParams{
		MetricName:  metricName,
		From:        timeRange.from,
		To:          timeRange.endTime,
		Labels:      parseLabelFilters(ginContext),
		ServiceName: ginContext.Query("service"),
	})
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, resp)
}

func (handler *Handler) GetHistogramBuckets(ginContext *gin.Context) {
	orgID, err := parseOrgID(ginContext)
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	metricName := ginContext.Param("metricName")
	if metricName == "" {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, "metric_name is required"))
		return
	}

	timeRange, err := parseTimeRange(ginContext)
	if err != nil {
		server.Handle(ginContext, server.BadRequest(server.CodeValidation, err.Error()))
		return
	}

	buckets, err := handler.service.GetHistogramBuckets(ginContext.Request.Context(), orgID, HistogramParams{
		MetricName: metricName,
		From:       timeRange.from,
		To:         timeRange.endTime,
	})
	if err != nil {
		server.Handle(ginContext, err)
		return
	}

	server.OK(ginContext, buckets)
}

// ── helpers ───────────────────────────────────────────────────────────────────

type parsedTimeRange struct {
	from    time.Time
	endTime time.Time
}

func parseTimeRange(ginContext *gin.Context) (parsedTimeRange, error) {
	now := time.Now().UTC()

	fromStr := ginContext.DefaultQuery("from", now.Add(-1*time.Hour).Format(time.RFC3339))
	toStr := ginContext.DefaultQuery("to", now.Format(time.RFC3339))

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return parsedTimeRange{}, fmt.Errorf("invalid 'from': %s", fromStr)
	}

	endTime, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return parsedTimeRange{}, fmt.Errorf("invalid 'to': %s", toStr)
	}

	if endTime.Before(from) {
		return parsedTimeRange{}, fmt.Errorf("'to' must be after 'from'")
	}

	return parsedTimeRange{from: from, endTime: endTime}, nil
}

func parseLabelFilters(ginContext *gin.Context) map[string]string {
	labels := map[string]string{}
	for key, valuesList := range ginContext.Request.URL.Query() {
		if len(key) > 6 && key[:6] == "label[" && key[len(key)-1] == ']' {
			labelKey := key[6 : len(key)-1]
			if len(valuesList) > 0 {
				labels[labelKey] = valuesList[0]
			}
		}
	}
	return labels
}

func parseOrgID(ginContext *gin.Context) (uuid.UUID, error) {
	orgID, err := uuid.Parse(ginContext.GetString("org_id"))
	if err != nil {
		return uuid.UUID{}, server.Unauthorized(server.CodeInvalidToken, "missing or invalid org_id")
	}
	return orgID, nil
}

func parsePagination(ginContext *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(ginContext.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(ginContext.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}
	return page, limit
}

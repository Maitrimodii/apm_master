package investigation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	MaxSlowTracesCap = 50
	MaxLogsCap       = 100
	DefaultTimeout   = 5 * time.Second
)

type EvidenceHandler struct {
	store       *Store
	metricStore *metric.Store
	traceStore  *trace.Store
	logStore    *log.Store
	cfg         *config.Config
	logger      *slog.Logger
}

func NewEvidenceHandler(
	store *Store,
	metricStore *metric.Store,
	traceStore *trace.Store,
	logStore *log.Store,
	cfg *config.Config,
	logger *slog.Logger,
) *EvidenceHandler {
	return &EvidenceHandler{
		store:       store,
		metricStore: metricStore,
		traceStore:  traceStore,
		logStore:    logStore,
		cfg:         cfg,
		logger:      logger,
	}
}

func (h *EvidenceHandler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	// Protected by ServiceTokenAuth middleware only
	routerGroup.Use(middleware.ServiceTokenAuth(h.cfg))

	routerGroup.GET("/incidents/:incident_id/context", h.GetIncidentContext)
	routerGroup.POST("/investigation-evidence/query", h.QueryEvidence)
	routerGroup.PUT("/investigations/:id/status", h.UpdateStatus)
	routerGroup.POST("/investigations/:id/report", h.SaveReport)
	routerGroup.POST("/investigations/:id/events", h.AppendEvent)
}

func (h *EvidenceHandler) GetIncidentContext(c *gin.Context) {
	incidentID, err := uuid.Parse(c.Param("incident_id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid incident_id"))
		return
	}

	ic, err := h.store.GetIncidentContext(c.Request.Context(), incidentID)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}
	if ic == nil {
		server.Handle(c, server.NotFound(server.CodeNotFound, "incident not found"))
		return
	}

	c.JSON(http.StatusOK, ic)
}

func (h *EvidenceHandler) QueryEvidence(c *gin.Context) {
	var req EvidenceQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, fmt.Sprintf("invalid request payload: %v", err)))
		return
	}

	// 1. Time window validation
	if req.TimeRangeStart.After(req.TimeRangeEnd) {
		server.Handle(c, server.BadRequest(server.CodeValidation, "time_range_start must be before time_range_end"))
		return
	}
	if req.TimeRangeEnd.Sub(req.TimeRangeStart) > MaxTimeWindow {
		server.Handle(c, server.BadRequest(server.CodeValidation, "time window cannot exceed 24 hours"))
		return
	}

	// 2. Tenant isolation verification: check investigation exists and matches tenant_id
	inv, err := h.store.GetByID(c.Request.Context(), req.InvestigationID)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}
	if inv == nil {
		server.Handle(c, server.NotFound(server.CodeNotFound, "investigation not found"))
		return
	}
	if inv.OrgID != req.TenantID {
		server.Handle(c, server.Forbidden(server.CodeForbidden, "cross-tenant access denied"))
		return
	}

	// 3. Dispatch to executor with per-request timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), DefaultTimeout)
	defer cancel()

	evidenceID := uuid.New()
	var (
		result   any
		warnings []string
		srcSvc   string
		summary  string
	)

	switch req.QueryType {
	case "metric_baseline":
		res, warn, svc, sum, qErr := h.executeMetricBaseline(ctx, req)
		if qErr != nil {
			server.Handle(c, server.BadRequest(server.CodeValidation, qErr.Error()))
			return
		}
		result = res
		warnings = warn
		srcSvc = svc
		summary = sum

	case "slow_traces":
		res, warn, svc, sum, qErr := h.executeSlowTraces(ctx, req)
		if qErr != nil {
			server.Handle(c, server.BadRequest(server.CodeValidation, qErr.Error()))
			return
		}
		result = res
		warnings = warn
		srcSvc = svc
		summary = sum

	case "trace_details":
		res, warn, svc, sum, qErr := h.executeTraceDetails(ctx, req)
		if qErr != nil {
			server.Handle(c, server.BadRequest(server.CodeValidation, qErr.Error()))
			return
		}
		result = res
		warnings = warn
		srcSvc = svc
		summary = sum

	case "correlated_logs":
		res, warn, svc, sum, qErr := h.executeCorrelatedLogs(ctx, req)
		if qErr != nil {
			server.Handle(c, server.BadRequest(server.CodeValidation, qErr.Error()))
			return
		}
		result = res
		warnings = warn
		srcSvc = svc
		summary = sum

	default:
		server.Handle(c, server.BadRequest(server.CodeValidation, fmt.Sprintf("unsupported query_type: %q", req.QueryType)))
		return
	}

	provenance := map[string]any{
		"source":           "apm_master",
		"query_type":       req.QueryType,
		"queried_at":       time.Now().UTC().Format(time.RFC3339),
		"tenant_id":        req.TenantID.String(),
		"investigation_id": req.InvestigationID.String(),
	}

	// 4. Persist evidence record
	evidenceRecord := &InvestigationEvidence{
		ID:                  evidenceID,
		InvestigationID:      req.InvestigationID,
		EvidenceType:        req.QueryType,
		SourceService:       srcSvc,
		QueryParameters:     req.Parameters,
		Summary:             summary,
		Result:              result,
		Provenance:          provenance,
		DataQualityWarnings: warnings,
	}

	if err := h.store.SaveEvidence(c.Request.Context(), evidenceRecord); err != nil {
		h.logger.Warn("failed to persist evidence in Go database", "error", err)
	}

	resp := EvidenceQueryResponse{
		EvidenceID:          evidenceID,
		QueryType:           req.QueryType,
		Result:              result,
		Provenance:          provenance,
		DataQualityWarnings: warnings,
	}

	c.JSON(http.StatusOK, resp)
}

func (h *EvidenceHandler) executeMetricBaseline(
	ctx context.Context,
	req EvidenceQueryRequest,
) (result any, warnings []string, serviceName string, summary string, err error) {
	metricName, _ := req.Parameters["metric_name"].(string)
	if metricName == "" {
		return nil, nil, "", "", errors.New("metric_name is required for metric_baseline query")
	}

	svcName, _ := req.Parameters["service_name"].(string)
	labels := make(map[string]string)
	if svcName != "" {
		labels["service.name"] = svcName
	}

	// Baseline window
	var baselineFrom, baselineTo time.Time
	if bFromStr, ok := req.Parameters["baseline_from"].(string); ok && bFromStr != "" {
		baselineFrom, _ = time.Parse(time.RFC3339, bFromStr)
	}
	if bToStr, ok := req.Parameters["baseline_to"].(string); ok && bToStr != "" {
		baselineTo, _ = time.Parse(time.RFC3339, bToStr)
	}

	windowDuration := req.TimeRangeEnd.Sub(req.TimeRangeStart)
	if baselineFrom.IsZero() || baselineTo.IsZero() {
		baselineTo = req.TimeRangeStart
		baselineFrom = baselineTo.Add(-windowDuration)
	}

	incidentAgg, err := h.metricStore.GetAggregations(ctx, metric.AggregationQuery{
		OrgID:      req.TenantID,
		MetricName: metricName,
		From:       req.TimeRangeStart,
		To:         req.TimeRangeEnd,
		Labels:     labels,
	})
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("aggregate incident metric: %w", err)
	}

	baselineAgg, err := h.metricStore.GetAggregations(ctx, metric.AggregationQuery{
		OrgID:      req.TenantID,
		MetricName: metricName,
		From:       baselineFrom,
		To:         baselineTo,
		Labels:     labels,
	})
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("aggregate baseline metric: %w", err)
	}

	var relChange float64
	if baselineAgg.Avg > 0 {
		relChange = math.Round(((incidentAgg.Avg-baselineAgg.Avg)/baselineAgg.Avg)*10000) / 100
	} else if incidentAgg.Avg > 0 {
		relChange = 100.0
	}

	if incidentAgg.Count == 0 {
		warnings = append(warnings, "No telemetry metric data points found in incident window")
	}
	if baselineAgg.Count == 0 {
		warnings = append(warnings, "No baseline telemetry metric data points found in baseline window")
	}

	comparison := map[string]any{
		"metric_name": metricName,
		"incident_window": map[string]any{
			"from":  req.TimeRangeStart.Format(time.RFC3339),
			"to":    req.TimeRangeEnd.Format(time.RFC3339),
			"avg":   incidentAgg.Avg,
			"p95":   incidentAgg.P95,
			"p99":   incidentAgg.P99,
			"min":   incidentAgg.Min,
			"max":   incidentAgg.Max,
			"count": incidentAgg.Count,
			"unit":  incidentAgg.Unit,
		},
		"baseline_window": map[string]any{
			"from":  baselineFrom.Format(time.RFC3339),
			"to":    baselineTo.Format(time.RFC3339),
			"avg":   baselineAgg.Avg,
			"p95":   baselineAgg.P95,
			"p99":   baselineAgg.P99,
			"min":   baselineAgg.Min,
			"max":   baselineAgg.Max,
			"count": baselineAgg.Count,
			"unit":  baselineAgg.Unit,
		},
		"relative_change_pct": relChange,
	}

	summary = fmt.Sprintf("Metric %s shifted by %.1f%% compared to baseline", metricName, relChange)
	return comparison, warnings, svcName, summary, nil
}

func (h *EvidenceHandler) executeSlowTraces(
	ctx context.Context,
	req EvidenceQueryRequest,
) (result any, warnings []string, serviceName string, summary string, err error) {
	svcName, _ := req.Parameters["service_name"].(string)

	limit := 10
	if limVal, ok := req.Parameters["limit"]; ok {
		switch v := limVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	if limit > MaxSlowTracesCap {
		limit = MaxSlowTracesCap
		warnings = append(warnings, fmt.Sprintf("Limit capped at maximum allowed: %d", MaxSlowTracesCap))
	}
	if limit <= 0 {
		limit = 10
	}

	var minDuration float64
	if durVal, ok := req.Parameters["min_duration_ms"]; ok {
		switch v := durVal.(type) {
		case float64:
			minDuration = v
		case int:
			minDuration = float64(v)
		}
	}

	summaries, total, err := h.traceStore.ListTraceSummaries(ctx, trace.ListSpansQuery{
		OrgID:         req.TenantID,
		From:          req.TimeRangeStart,
		To:            req.TimeRangeEnd,
		ServiceName:   svcName,
		MinDurationMS: minDuration,
		Limit:         limit,
		Offset:        0,
	})
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("list trace summaries: %w", err)
	}

	if total == 0 {
		warnings = append(warnings, "No slow traces found matching criteria in incident window")
	}

	var tracesList []map[string]any
	for _, s := range summaries {
		tracesList = append(tracesList, map[string]any{
			"trace_id":          s.TraceID,
			"service_name":      s.ServiceName,
			"endpoint":          s.Endpoint,
			"duration_ms":       s.DurationMS,
			"status":             s.Status,
			"span_count":         s.SpanCount,
			"error_count":        s.ErrorCount,
			"db_time_ms":         s.DBTimeMS,
			"middleware_time_ms": s.MiddlewareTimeMS,
		})
	}

	summary = fmt.Sprintf("Retrieved %d slow traces (total %d matching)", len(tracesList), total)
	return tracesList, warnings, svcName, summary, nil
}

func (h *EvidenceHandler) executeTraceDetails(
	ctx context.Context,
	req EvidenceQueryRequest,
) (result any, warnings []string, serviceName string, summary string, err error) {
	traceID, _ := req.Parameters["trace_id"].(string)
	if traceID == "" {
		return nil, nil, "", "", errors.New("trace_id is required for trace_details query")
	}

	spans, err := h.traceStore.GetTrace(ctx, req.TenantID, traceID)
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("get trace spans: %w", err)
	}

	if len(spans) == 0 {
		warnings = append(warnings, fmt.Sprintf("No spans found for trace ID %s", traceID))
	}

	var spansList []map[string]any
	for _, sp := range spans {
		if serviceName == "" && sp.ServiceName != "" {
			serviceName = sp.ServiceName
		}
		spansList = append(spansList, map[string]any{
			"span_id":        sp.SpanID,
			"trace_id":       sp.TraceID,
			"parent_id":      sp.ParentID,
			"service_name":   sp.ServiceName,
			"span_name":      sp.SpanName,
			"span_kind":      sp.SpanKind,
			"start_time":     sp.StartTime.Format(time.RFC3339Nano),
			"end_time":       sp.EndTime.Format(time.RFC3339Nano),
			"duration_ms":    sp.DurationMS,
			"status_code":    sp.StatusCode,
			"status_message": sp.StatusMessage,
			"span_attrs":     sp.SpanAttrs,
		})
	}

	summary = fmt.Sprintf("Retrieved trace %s with %d spans", traceID, len(spansList))
	return spansList, warnings, serviceName, summary, nil
}

func (h *EvidenceHandler) executeCorrelatedLogs(
	ctx context.Context,
	req EvidenceQueryRequest,
) (result any, warnings []string, serviceName string, summary string, err error) {
	svcName, _ := req.Parameters["service_name"].(string)
	traceID, _ := req.Parameters["trace_id"].(string)
	search, _ := req.Parameters["search"].(string)

	minSeverity := 0
	if sevVal, ok := req.Parameters["min_severity"]; ok {
		switch v := sevVal.(type) {
		case float64:
			minSeverity = int(v)
		case int:
			minSeverity = v
		}
	}

	limit := 50
	if limVal, ok := req.Parameters["limit"]; ok {
		switch v := limVal.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	if limit > MaxLogsCap {
		limit = MaxLogsCap
		warnings = append(warnings, fmt.Sprintf("Limit capped at maximum allowed: %d", MaxLogsCap))
	}
	if limit <= 0 {
		limit = 50
	}

	logs, total, err := h.logStore.ListLogs(ctx, log.ListLogsQuery{
		OrgID:       req.TenantID.String(),
		From:        &req.TimeRangeStart,
		To:          &req.TimeRangeEnd,
		ServiceName: svcName,
		MinSeverity: minSeverity,
		TraceID:     traceID,
		Search:      search,
		Limit:       limit,
		Offset:      0,
	})
	if err != nil {
		return nil, nil, "", "", fmt.Errorf("list logs: %w", err)
	}

	if total == 0 {
		warnings = append(warnings, "No correlated log records found matching query")
	}

	var logsList []map[string]any
	for _, l := range logs {
		logsList = append(logsList, map[string]any{
			"time":          l.Time.Format(time.RFC3339Nano),
			"service_name":  l.ServiceName,
			"trace_id":      l.TraceID,
			"span_id":       l.SpanID,
			"severity_num":  l.SeverityNum,
			"severity_text": l.SeverityText,
			"body":          l.Body,
			"log_attrs":     l.LogAttrs,
		})
	}

	summary = fmt.Sprintf("Retrieved %d correlated log records (total %d matching)", len(logsList), total)
	return logsList, warnings, svcName, summary, nil
}

func (h *EvidenceHandler) UpdateStatus(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	err = h.store.UpdateStatusDirect(c.Request.Context(), id, req.Status, req.FailureCode, req.FailureMessage)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *EvidenceHandler) SaveReport(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	var req SaveReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	report := &InvestigationReportRecord{
		InvestigationID:  id,
		SchemaVersion:    req.SchemaVersion,
		ReportJSON:       req.ReportJSON,
		ValidationStatus: req.ValidationStatus,
	}

	if err := h.store.SaveReport(c.Request.Context(), report); err != nil {
		server.Handle(c, server.Internal(err))
		return
	}

	// Update status to completed or inconclusive
	newStatus := StatusCompleted
	if req.ValidationStatus != "valid" {
		newStatus = StatusInconclusive
	}
	_ = h.store.UpdateStatusDirect(c.Request.Context(), id, newStatus, nil, nil)

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h *EvidenceHandler) AppendEvent(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	var req AppendEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid request payload"))
		return
	}

	ev := &InvestigationEvent{
		InvestigationID: id,
		SequenceNumber:  req.SequenceNumber,
		EventType:       req.EventType,
		Summary:         req.Summary,
		Payload:         req.Payload,
	}

	if err := h.store.AppendEvent(c.Request.Context(), ev); err != nil {
		server.Handle(c, server.Internal(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

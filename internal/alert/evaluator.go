package alert

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
)

// Evaluator evaluates an alert definition against telemetry data.
type Evaluator interface {
	Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error)
}

// EvaluationResult captures the outcome of one evaluation cycle.
type EvaluationResult struct {
	Fired      bool
	Value      float64
	Threshold  float64
	Message    string
	Evaluator  string
	Window     time.Duration
	Comparison string
}

type metricSeriesReader interface {
	GetSeries(ctx context.Context, query metric.SeriesQuery) ([]metric.MetricPoint, metric.Resolution, error)
	GetAggregations(ctx context.Context, query metric.AggregationQuery) (metric.MetricAggregation, error)
}

type logsSeriesReader interface {
	ListLogs(ctx context.Context, query log.ListLogsQuery) ([]log.LogRecord, int, error)
}

type tracesSeriesReader interface {
	ListSpans(ctx context.Context, query trace.ListSpansQuery) ([]trace.Span, int, error)
	ListTraceSummaries(ctx context.Context, query trace.ListSpansQuery) ([]trace.TraceSummary, int, error)
}

type parsedCondition struct {
	agg       string
	target    string // evaluator.g. "cpu_usage", "logs{service_name=\"checkout\"}"
	operator  string
	threshold float64
	window    time.Duration
	raw       string
}

var conditionPattern = regexp.MustCompile(`(?i)^\s*(avg|p50|p95|p99|min|max|count|sum|error_rate)\s*\(([^)]+)\)\s*(>=|<=|>|<|==|!=)\s*([0-9.]+(?:ms|s|m|h|%)?)\s*(?:for\s+([0-9]+(?:ms|s|m|h)))?\s*$`)

func parseCondition(condition string) (*parsedCondition, error) {
	match := conditionPattern.FindStringSubmatch(strings.TrimSpace(condition))
	if match == nil {
		return nil, fmt.Errorf("unsupported condition format: %q", condition)
	}

	agg := strings.ToLower(strings.TrimSpace(match[1]))
	target := strings.TrimSpace(match[2])
	operator := match[3]
	threshold, err := parseThreshold(match[4])
	if err != nil {
		return nil, err
	}

	window := 5 * time.Minute
	if len(match) > 5 && match[5] != "" {
		window, err = parseDuration(match[5])
		if err != nil {
			return nil, err
		}
	}

	return &parsedCondition{
		agg:       agg,
		target:    target,
		operator:  operator,
		threshold: threshold,
		window:    window,
		raw:       condition,
	}, nil
}

func parseTargetFilters(target string) (base string, filters map[string]string) {
	target = strings.TrimSpace(target)
	filters = make(map[string]string)

	idx := strings.Index(target, "{")
	if idx == -1 {
		return target, filters
	}

	base = strings.TrimSpace(target[:idx])
	endIdx := strings.LastIndex(target, "}")
	if endIdx == -1 || endIdx <= idx {
		return base, filters
	}

	filterStr := target[idx+1 : endIdx]
	parts := strings.Split(filterStr, ",")
	for _, part := range parts {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) != 2 {
			continue
		}
		key := strings.TrimSpace(keyValue[0])
		value := strings.TrimSpace(keyValue[1])
		value = strings.Trim(value, `"' `)
		filters[key] = value
	}

	return base, filters
}

// MetricEvaluator evaluates metric-based alert conditions.
type MetricEvaluator struct {
	reader metricSeriesReader
	log    *slog.Logger
}

func NewMetricEvaluator(reader metricSeriesReader, log *slog.Logger) *MetricEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &MetricEvaluator{reader: reader, log: log}
}

func (evaluator *MetricEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	parsed, err := parseCondition(alert.Condition)
	if err != nil {
		return nil, err
	}
	return evaluator.EvaluateSubCondition(ctx, alert.OrgID, parsed)
}

func (evaluator *MetricEvaluator) EvaluateSubCondition(ctx context.Context, orgID uuid.UUID, parsed *parsedCondition) (*EvaluationResult, error) {
	metricName, filters := parseTargetFilters(parsed.target)

	agg, err := evaluator.reader.GetAggregations(ctx, metric.AggregationQuery{
		OrgID:      orgID,
		MetricName: metricName,
		From:       time.Now().Add(-parsed.window),
		To:         time.Now(),
		Labels:     filters,
	})
	if err != nil {
		return nil, fmt.Errorf("read metric aggregations: %w", err)
	}

	if agg.Count == 0 {
		return &EvaluationResult{
			Fired:      false,
			Value:      0,
			Threshold:  parsed.threshold,
			Message:    fmt.Sprintf("no telemetry samples found for %s", metricName),
			Evaluator:  "metric",
			Window:     parsed.window,
			Comparison: parsed.operator,
		}, nil
	}

	var value float64
	switch strings.ToLower(parsed.agg) {
	case "avg":
		value = agg.Avg
	case "min":
		value = agg.Min
	case "max":
		value = agg.Max
	case "p50":
		value = agg.P50
	case "p95":
		value = agg.P95
	case "p99":
		value = agg.P99
	case "count":
		value = float64(agg.Count)
	case "sum":
		value = agg.Total
	default:
		value = agg.Avg
	}

	triggered := compareValue(value, parsed.operator, parsed.threshold)
	message := fmt.Sprintf("%s %s %.2f %s %.2f", parsed.agg, metricName, value, parsed.operator, parsed.threshold)

	return &EvaluationResult{
		Fired:      triggered,
		Value:      value,
		Threshold:  parsed.threshold,
		Message:    message,
		Evaluator:  "metric",
		Window:     parsed.window,
		Comparison: parsed.operator,
	}, nil
}

// LogEvaluator evaluates log-based alert conditions.
type LogEvaluator struct {
	reader logsSeriesReader
	log    *slog.Logger
}

func NewLogEvaluator(reader logsSeriesReader, log *slog.Logger) *LogEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &LogEvaluator{reader: reader, log: log}
}

func (evaluator *LogEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	parsed, err := parseCondition(alert.Condition)
	if err != nil {
		return nil, err
	}
	return evaluator.EvaluateSubCondition(ctx, alert.OrgID, parsed)
}

func (evaluator *LogEvaluator) EvaluateSubCondition(ctx context.Context, orgID uuid.UUID, parsed *parsedCondition) (*EvaluationResult, error) {
	_, filters := parseTargetFilters(parsed.target)

	fromTime := time.Now().Add(-parsed.window)
	toTime := time.Now()

	query := log.ListLogsQuery{
		OrgID: orgID.String(),
		From:  &fromTime,
		To:    &toTime,
		Limit: 1,
	}

	if svc, exists := filters["service_name"]; exists {
		query.ServiceName = svc
	}
	if sev, exists := filters["severity"]; exists {
		query.MinSeverity = int(severityToNum(sev))
	}
	if search, exists := filters["search"]; exists {
		query.Search = search
	}

	_, total, err := evaluator.reader.ListLogs(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read log count: %w", err)
	}

	value := float64(total)
	triggered := compareValue(value, parsed.operator, parsed.threshold)
	message := fmt.Sprintf("logs count %d %s %.2f", total, parsed.operator, parsed.threshold)

	return &EvaluationResult{
		Fired:      triggered,
		Value:      value,
		Threshold:  parsed.threshold,
		Message:    message,
		Evaluator:  "log",
		Window:     parsed.window,
		Comparison: parsed.operator,
	}, nil
}

func severityToNum(sev string) int16 {
	switch strings.ToUpper(strings.TrimSpace(sev)) {
	case "TRACE":
		return 1
	case "DEBUG":
		return 5
	case "INFO":
		return 9
	case "WARN", "WARNING":
		return 13
	case "ERROR":
		return 17
	case "FATAL":
		return 21
	default:
		return 0
	}
}

// TraceEvaluator evaluates trace-based alert conditions.
type TraceEvaluator struct {
	reader tracesSeriesReader
	log    *slog.Logger
}

func NewTraceEvaluator(reader tracesSeriesReader, log *slog.Logger) *TraceEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &TraceEvaluator{reader: reader, log: log}
}

func (evaluator *TraceEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	parsed, err := parseCondition(alert.Condition)
	if err != nil {
		return nil, err
	}
	return evaluator.EvaluateSubCondition(ctx, alert.OrgID, parsed)
}

func (evaluator *TraceEvaluator) EvaluateSubCondition(ctx context.Context, orgID uuid.UUID, parsed *parsedCondition) (*EvaluationResult, error) {
	base, filters := parseTargetFilters(parsed.target)

	query := trace.ListSpansQuery{
		OrgID: orgID,
		From:  time.Now().Add(-parsed.window),
		To:    time.Now(),
	}

	if svc, exists := filters["service_name"]; exists {
		query.ServiceName = svc
	}

	summaries, _, err := evaluator.reader.ListTraceSummaries(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("read trace summaries: %w", err)
	}

	var value float64
	aggLower := strings.ToLower(parsed.agg)

	if len(summaries) == 0 {
		return &EvaluationResult{
			Fired:      false,
			Value:      0,
			Threshold:  parsed.threshold,
			Message:    "no traces found in window",
			Evaluator:  "trace",
			Window:     parsed.window,
			Comparison: parsed.operator,
		}, nil
	}

	switch aggLower {
	case "count":
		if strings.Contains(base, "duration") {
			value = float64(len(summaries))
		} else {
			var spansCount int64
			for _, summary := range summaries {
				spansCount += int64(summary.SpanCount)
			}
			value = float64(spansCount)
		}
	case "avg":
		var sum float64
		for _, summary := range summaries {
			sum += summary.DurationMS
		}
		value = sum / float64(len(summaries))
	case "max":
		var max float64
		for _, summary := range summaries {
			if summary.DurationMS > max {
				max = summary.DurationMS
			}
		}
		value = max
	case "min":
		min := math.MaxFloat64
		for _, summary := range summaries {
			if summary.DurationMS < min {
				min = summary.DurationMS
			}
		}
		value = min
	case "error_rate":
		var errorTraces int
		for _, summary := range summaries {
			if summary.Status == "ERROR" {
				errorTraces++
			}
		}
		value = (float64(errorTraces) / float64(len(summaries))) * 100
	default:
		return nil, fmt.Errorf("unsupported trace aggregation function: %s", parsed.agg)
	}

	triggered := compareValue(value, parsed.operator, parsed.threshold)
	message := fmt.Sprintf("%s trace/span value %.2f %s %.2f", parsed.agg, value, parsed.operator, parsed.threshold)

	return &EvaluationResult{
		Fired:      triggered,
		Value:      value,
		Threshold:  parsed.threshold,
		Message:    message,
		Evaluator:  "trace",
		Window:     parsed.window,
		Comparison: parsed.operator,
	}, nil
}

// CompoundEvaluator evaluates compound alert conditions containing AND/OR logical composition.
type CompoundEvaluator struct {
	metricsStore metric.MetricStoreInterface
	logsStore    log.LogStoreInterface
	tracesStore  trace.TraceStoreInterface
	log          *slog.Logger
}

func NewCompoundEvaluator(
	metricsStore metric.MetricStoreInterface,
	logsStore log.LogStoreInterface,
	tracesStore trace.TraceStoreInterface,
	log *slog.Logger,
) *CompoundEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &CompoundEvaluator{
		metricsStore: metricsStore,
		logsStore:    logsStore,
		tracesStore:  tracesStore,
		log:          log,
	}
}

func (evaluator *CompoundEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	cond := alert.Condition

	orParts := splitLogicalOperator(cond, "OR")

	var fired bool
	var orResults []*EvaluationResult
	var combinedMsgs []string

	for _, orPart := range orParts {
		andParts := splitLogicalOperator(orPart, "AND")

		var andFired = true
		var andResults []*EvaluationResult
		var andMsgs []string

		for _, andPart := range andParts {
			res, err := evaluator.evaluateSingleCondition(ctx, alert.OrgID, andPart)
			if err != nil {
				return nil, fmt.Errorf("evaluate sub-condition %q: %w", andPart, err)
			}
			andResults = append(andResults, res)
			andMsgs = append(andMsgs, res.Message)
			if !res.Fired {
				andFired = false
			}
		}

		if andFired && len(andParts) > 0 {
			fired = true
		}

		orResultMsg := "(" + strings.Join(andMsgs, " AND ") + ")"
		combinedMsgs = append(combinedMsgs, orResultMsg)
		orResults = append(orResults, &EvaluationResult{
			Fired:   andFired,
			Message: orResultMsg,
		})
	}

	finalMsg := strings.Join(combinedMsgs, " OR ")

	var finalWindow time.Duration
	if len(orResults) > 0 && len(orParts) > 0 {
		finalWindow = 5 * time.Minute
	}

	return &EvaluationResult{
		Fired:     fired,
		Value:     0,
		Threshold: 0,
		Message:   finalMsg,
		Evaluator: "compound",
		Window:    finalWindow,
	}, nil
}

func isInsideBracesOrQuotes(expr string, index int) bool {
	inBraces := false
	inQuotes := false
	var quoteChar rune

	runes := []rune(expr)
	for i := 0; i < index && i < len(runes); i++ {
		r := runes[i]
		if inQuotes {
			if r == quoteChar {
				if i > 0 && runes[i-1] == '\\' {
					// escaped quote, keep inside quotes
				} else {
					inQuotes = false
				}
			}
		} else {
			if r == '"' || r == '\'' {
				inQuotes = true
				quoteChar = r
			} else if r == '{' {
				inBraces = true
			} else if r == '}' {
				inBraces = false
			}
		}
	}
	return inBraces || inQuotes
}

func splitLogicalOperator(expr string, op string) []string {
	var parts []string
	opToken := " " + strings.ToUpper(op) + " "

	lowerExpr := strings.ToLower(expr)
	lowerOpToken := strings.ToLower(opToken)

	start := 0
	searchStart := 0
	for {
		idx := strings.Index(lowerExpr[searchStart:], lowerOpToken)
		if idx == -1 {
			parts = append(parts, strings.TrimSpace(expr[start:]))
			break
		}

		matchIdx := searchStart + idx
		if isInsideBracesOrQuotes(expr, matchIdx) {
			searchStart = matchIdx + 1
			continue
		}

		parts = append(parts, strings.TrimSpace(expr[start:matchIdx]))
		start = matchIdx + len(opToken)
		searchStart = start
	}

	return parts
}

func (evaluator *CompoundEvaluator) evaluateSingleCondition(ctx context.Context, orgID uuid.UUID, cond string) (*EvaluationResult, error) {
	parsed, err := parseCondition(cond)
	if err != nil {
		return nil, err
	}

	targetBase, _ := parseTargetFilters(parsed.target)
	targetLower := strings.ToLower(targetBase)

	switch {
	case strings.Contains(targetLower, "logs"):
		eval := NewLogEvaluator(evaluator.logsStore, evaluator.log)
		return eval.EvaluateSubCondition(ctx, orgID, parsed)
	case strings.Contains(targetLower, "spans") || strings.Contains(targetLower, "duration"):
		eval := NewTraceEvaluator(evaluator.tracesStore, evaluator.log)
		return eval.EvaluateSubCondition(ctx, orgID, parsed)
	default:
		eval := NewMetricEvaluator(evaluator.metricsStore, evaluator.log)
		return eval.EvaluateSubCondition(ctx, orgID, parsed)
	}
}

func parseThreshold(value string) (float64, error) {
	value = strings.TrimSpace(value)
	unit := ""
	if strings.HasSuffix(value, "%") {
		unit = "%"
		value = strings.TrimSuffix(value, "%")
	} else if strings.HasSuffix(value, "ms") {
		unit = "ms"
		value = strings.TrimSuffix(value, "ms")
	} else if strings.HasSuffix(value, "s") {
		unit = "s"
		value = strings.TrimSuffix(value, "s")
	} else if strings.HasSuffix(value, "m") {
		unit = "m"
		value = strings.TrimSuffix(value, "m")
	} else if strings.HasSuffix(value, "h") {
		unit = "h"
		value = strings.TrimSuffix(value, "h")
	}

	threshold, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid threshold %q: %w", value, err)
	}

	switch unit {
	case "ms":
		return threshold, nil
	case "s":
		return threshold * 1000, nil
	case "m":
		return threshold * 60 * 1000, nil
	case "h":
		return threshold * 60 * 60 * 1000, nil
	default:
		return threshold, nil
	}
}

func parseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(value, "ms") {
		return time.Duration(mustParseFloat(strings.TrimSuffix(value, "ms"))) * time.Millisecond, nil
	}
	if strings.HasSuffix(value, "s") {
		return time.Duration(mustParseFloat(strings.TrimSuffix(value, "s"))) * time.Second, nil
	}
	if strings.HasSuffix(value, "m") {
		return time.Duration(mustParseFloat(strings.TrimSuffix(value, "m"))) * time.Minute, nil
	}
	if strings.HasSuffix(value, "h") {
		return time.Duration(mustParseFloat(strings.TrimSuffix(value, "h"))) * time.Hour, nil
	}
	return 0, fmt.Errorf("unsupported duration %q", value)
}

func mustParseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func compareValue(value float64, operator string, threshold float64) bool {
	switch operator {
	case ">":
		return value > threshold
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	case "==":
		return math.Abs(value-threshold) < 1e-9
	case "!=":
		return math.Abs(value-threshold) >= 1e-9
	default:
		return false
	}
}

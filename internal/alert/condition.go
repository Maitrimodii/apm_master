package alert

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Source is the telemetry signal a condition reads.
type Source string

const (
	SourceMetric Source = "metric"
	SourceLog    Source = "log"
	SourceTrace  Source = "trace"
)

// DefaultWindow applies when a condition has no "for <duration>" clause.
const DefaultWindow = 5 * time.Minute

// Aggregations each source supports.
var sourceAggregations = map[Source]map[string]bool{
	SourceMetric: {"avg": true, "min": true, "max": true, "p50": true, "p90": true, "p95": true, "p99": true, "count": true, "sum": true},
	SourceLog:    {"count": true},
	SourceTrace:  {"count": true, "avg": true, "min": true, "max": true, "error_rate": true},
}

var operators = map[string]func(value, threshold float64) bool{
	">":  func(v, t float64) bool { return v > t },
	">=": func(v, t float64) bool { return v >= t },
	"<":  func(v, t float64) bool { return v < t },
	"<=": func(v, t float64) bool { return v <= t },
	"==": func(v, t float64) bool { return math.Abs(v-t) < 1e-9 },
	"!=": func(v, t float64) bool { return math.Abs(v-t) >= 1e-9 },
}

// Condition is one parsed comparison such as
// `p95(http.latency{service="api"}) > 500ms for 5m`.
// It is stored as JSON next to the alert text and sent to the AI service.
type Condition struct {
	Source        Source            `json:"source"`
	Aggregation   string            `json:"aggregation"`
	Target        string            `json:"target"`
	Filters       map[string]string `json:"filters,omitempty"`
	Operator      string            `json:"operator"`
	Threshold     float64           `json:"threshold"`
	WindowSeconds int64             `json:"window_seconds"`
}

// Window is how far back the condition looks.
func (c Condition) Window() time.Duration {
	return time.Duration(c.WindowSeconds) * time.Second
}

// Compare reports whether value meets the condition.
func (c Condition) Compare(value float64) bool {
	cmp, ok := operators[c.Operator]
	return ok && cmp(value, c.Threshold)
}

// Expression is an OR of AND groups: any group whose conditions all hold
// makes the alert fire.
type Expression struct {
	AnyOf [][]Condition `json:"any_of"`
}

// Window is the longest window of any condition; an instance must stay in
// breach this long before it moves from PENDING to FIRING.
func (e Expression) Window() time.Duration {
	var longest time.Duration
	for _, group := range e.AnyOf {
		for _, c := range group {
			longest = max(longest, c.Window())
		}
	}
	return longest
}

// Conditions lists every condition in the expression, in order.
func (e Expression) Conditions() []Condition {
	var out []Condition
	for _, group := range e.AnyOf {
		out = append(out, group...)
	}
	return out
}

// ParseExpression parses alert condition text, e.g.
// `avg(cpu_usage) > 80 for 5m OR count(logs{severity="ERROR"}) > 10`.
func ParseExpression(text string) (Expression, error) {
	if strings.TrimSpace(text) == "" {
		return Expression{}, errors.New("condition is empty")
	}
	var expr Expression
	for _, orPart := range splitLogicalOperator(text, "OR") {
		var group []Condition
		for _, andPart := range splitLogicalOperator(orPart, "AND") {
			c, err := parseCondition(andPart)
			if err != nil {
				return Expression{}, err
			}
			group = append(group, c)
		}
		expr.AnyOf = append(expr.AnyOf, group)
	}
	return expr, nil
}

var conditionPattern = regexp.MustCompile(`(?i)^\s*(\w+)\s*\(([^)]+)\)\s*(>=|<=|>|<|==|!=)\s*([0-9.]+(?:ms|s|m|h|%)?)\s*(?:for\s+([0-9.]+(?:ms|s|m|h)))?\s*$`)

func parseCondition(text string) (Condition, error) {
	match := conditionPattern.FindStringSubmatch(strings.TrimSpace(text))
	if match == nil {
		return Condition{}, fmt.Errorf("unsupported condition format %q: want agg(target) <op> <threshold> [for <duration>]", text)
	}

	target, filters := parseTargetFilters(match[2])
	c := Condition{
		Source:        sourceFor(target),
		Aggregation:   strings.ToLower(match[1]),
		Target:        target,
		Filters:       filters,
		Operator:      match[3],
		WindowSeconds: int64(DefaultWindow / time.Second),
	}
	if !sourceAggregations[c.Source][c.Aggregation] {
		return Condition{}, fmt.Errorf("aggregation %q is not supported for %s conditions", c.Aggregation, c.Source)
	}

	threshold, err := parseThreshold(match[4])
	if err != nil {
		return Condition{}, err
	}
	c.Threshold = threshold

	if match[5] != "" {
		window, err := parseDuration(match[5])
		if err != nil {
			return Condition{}, err
		}
		c.WindowSeconds = int64(window / time.Second)
	}
	return c, nil
}

// sourceFor routes a target to its signal by its base name: "logs…" reads
// logs, "spans…"/"traces…" read traces, anything else is a metric name.
func sourceFor(target string) Source {
	lower := strings.ToLower(target)
	switch {
	case lower == "logs" || strings.HasPrefix(lower, "logs."):
		return SourceLog
	case lower == "spans" || lower == "traces" || strings.HasPrefix(lower, "spans.") || strings.HasPrefix(lower, "traces."):
		return SourceTrace
	default:
		return SourceMetric
	}
}

// parseTargetFilters splits `name{k="v",k2="v2"}` into the name and filters.
func parseTargetFilters(target string) (string, map[string]string) {
	target = strings.TrimSpace(target)
	open := strings.Index(target, "{")
	if open == -1 {
		return target, nil
	}
	base := strings.TrimSpace(target[:open])
	end := strings.LastIndex(target, "}")
	if end <= open {
		return base, nil
	}
	filters := map[string]string{}
	for _, part := range strings.Split(target[open+1:end], ",") {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		filters[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return base, filters
}

// thresholdUnits converts a threshold suffix to milliseconds. "%" and no
// suffix leave the number as-is.
var thresholdUnits = []struct {
	suffix string
	factor float64
}{
	{"ms", 1}, {"s", 1000}, {"m", 60 * 1000}, {"h", 60 * 60 * 1000}, {"%", 1},
}

func parseThreshold(value string) (float64, error) {
	value = strings.TrimSpace(value)
	factor := 1.0
	for _, u := range thresholdUnits {
		if strings.HasSuffix(value, u.suffix) {
			value, factor = strings.TrimSuffix(value, u.suffix), u.factor
			break
		}
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid threshold %q: %w", value, err)
	}
	return n * factor, nil
}

var durationUnits = []struct {
	suffix string
	unit   time.Duration
}{
	{"ms", time.Millisecond}, {"s", time.Second}, {"m", time.Minute}, {"h", time.Hour},
}

func parseDuration(value string) (time.Duration, error) {
	value = strings.TrimSpace(value)
	for _, u := range durationUnits {
		if num, ok := strings.CutSuffix(value, u.suffix); ok {
			n, err := strconv.ParseFloat(num, 64)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid duration %q", value)
			}
			return time.Duration(n * float64(u.unit)), nil
		}
	}
	return 0, fmt.Errorf("invalid duration %q: want a number followed by ms, s, m or h", value)
}

// splitLogicalOperator splits expr on a case-insensitive " OR " / " AND ",
// ignoring operators inside {...} filters or quoted strings.
func splitLogicalOperator(expr string, op string) []string {
	token := " " + strings.ToLower(op) + " "
	lower := strings.ToLower(expr)
	var parts []string
	start, search := 0, 0
	for {
		idx := strings.Index(lower[search:], token)
		if idx == -1 {
			return append(parts, strings.TrimSpace(expr[start:]))
		}
		at := search + idx
		if isInsideBracesOrQuotes(expr, at) {
			search = at + 1
			continue
		}
		parts = append(parts, strings.TrimSpace(expr[start:at]))
		start = at + len(token)
		search = start
	}
}

func isInsideBracesOrQuotes(expr string, index int) bool {
	inBraces, inQuotes := false, false
	var quote rune
	runes := []rune(expr)
	for i := 0; i < index && i < len(runes); i++ {
		r := runes[i]
		switch {
		case inQuotes:
			if r == quote && (i == 0 || runes[i-1] != '\\') {
				inQuotes = false
			}
		case r == '"' || r == '\'':
			inQuotes, quote = true, r
		case r == '{':
			inBraces = true
		case r == '}':
			inBraces = false
		}
	}
	return inBraces || inQuotes
}

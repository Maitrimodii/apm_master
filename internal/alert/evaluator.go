package alert

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// EvaluationResult captures the outcome of one evaluation cycle.
type EvaluationResult struct {
	Fired      bool
	Value      float64
	Threshold  float64
	Message    string
	Evaluator  Source
	Window     time.Duration
	Comparison string
}

// conditionEvaluator evaluates one condition for one org.
type conditionEvaluator interface {
	evaluate(ctx context.Context, orgID uuid.UUID, c Condition) (*EvaluationResult, error)
}

// evaluateAlert parses an alert's condition and evaluates its first
// condition. It backs the per-source evaluators' Evaluate methods.
func evaluateAlert(ctx context.Context, e conditionEvaluator, alert Alert) (*EvaluationResult, error) {
	expr, err := alert.ParsedCondition()
	if err != nil {
		return nil, err
	}
	conds := expr.Conditions()
	if len(conds) == 0 {
		return nil, fmt.Errorf("alert %s has no conditions", alert.ID)
	}
	return e.evaluate(ctx, alert.OrgID, conds[0])
}

func newResult(c Condition, value float64, message string) *EvaluationResult {
	return &EvaluationResult{
		Fired:      c.Compare(value),
		Value:      value,
		Threshold:  c.Threshold,
		Message:    message,
		Evaluator:  c.Source,
		Window:     c.Window(),
		Comparison: c.Operator,
	}
}

func noData(c Condition, message string) *EvaluationResult {
	r := newResult(c, 0, message)
	r.Fired = false
	return r
}

// ExpressionEvaluator evaluates a full AND/OR expression by routing each
// condition to the evaluator for its source.
type ExpressionEvaluator struct {
	bySource map[Source]conditionEvaluator
}

func NewExpressionEvaluator(metric *MetricEvaluator, log *LogEvaluator, trace *TraceEvaluator) *ExpressionEvaluator {
	return &ExpressionEvaluator{bySource: map[Source]conditionEvaluator{
		SourceMetric: metric,
		SourceLog:    log,
		SourceTrace:  trace,
	}}
}

// Evaluate fires when every condition in any OR group holds. A single
// condition reports its own value and threshold; compound expressions report
// the first condition that fired (or the first evaluated).
func (e *ExpressionEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	expr, err := alert.ParsedCondition()
	if err != nil {
		return nil, err
	}

	var first, firstFired *EvaluationResult
	var groupMsgs []string
	fired := false
	for _, group := range expr.AnyOf {
		groupFired := len(group) > 0
		var msgs []string
		for _, c := range group {
			ev, ok := e.bySource[c.Source]
			if !ok || ev == nil {
				return nil, fmt.Errorf("no evaluator for %s conditions", c.Source)
			}
			res, err := ev.evaluate(ctx, alert.OrgID, c)
			if err != nil {
				return nil, fmt.Errorf("evaluate %s(%s): %w", c.Aggregation, c.Target, err)
			}
			if first == nil {
				first = res
			}
			if res.Fired && firstFired == nil {
				firstFired = res
			}
			groupFired = groupFired && res.Fired
			msgs = append(msgs, res.Message)
		}
		fired = fired || groupFired
		groupMsgs = append(groupMsgs, "("+strings.Join(msgs, " AND ")+")")
	}
	if first == nil {
		return nil, fmt.Errorf("alert %s has no conditions", alert.ID)
	}

	if len(expr.Conditions()) == 1 {
		return first, nil
	}
	report := first
	if firstFired != nil {
		report = firstFired
	}
	return &EvaluationResult{
		Fired:      fired,
		Value:      report.Value,
		Threshold:  report.Threshold,
		Message:    strings.Join(groupMsgs, " OR "),
		Evaluator:  report.Evaluator,
		Window:     expr.Window(),
		Comparison: report.Comparison,
	}, nil
}

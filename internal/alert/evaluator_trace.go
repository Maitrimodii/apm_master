package alert

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/traces"
)

// traceStatusError is the status a trace summary reports when any span failed.
const traceStatusError = "ERROR"

type traceReader interface {
	ListTraceSummaries(ctx context.Context, filter traces.ListFilter) ([]traces.TraceSummary, int, error)
}

// TraceEvaluator evaluates conditions over trace summaries in the window:
// count, avg/min/max duration (ms) and error_rate (% of traces).
type TraceEvaluator struct {
	reader traceReader
	log    *slog.Logger
}

func NewTraceEvaluator(reader traceReader, log *slog.Logger) *TraceEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &TraceEvaluator{reader: reader, log: log}
}

func (e *TraceEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	return evaluateAlert(ctx, e, alert)
}

func (e *TraceEvaluator) evaluate(ctx context.Context, orgID uuid.UUID, c Condition) (*EvaluationResult, error) {
	now := time.Now()
	summaries, _, err := e.reader.ListTraceSummaries(ctx, traces.ListFilter{
		OrgID:       orgID,
		From:        now.Add(-c.Window()),
		To:          now,
		ServiceName: c.Filters[logFilterService],
	})
	if err != nil {
		return nil, fmt.Errorf("read trace summaries: %w", err)
	}
	if len(summaries) == 0 {
		return noData(c, "no traces found in window"), nil
	}

	var value float64
	switch c.Aggregation {
	case "count":
		// count(spans.duration) counts traces; count(spans) counts spans.
		if strings.Contains(strings.ToLower(c.Target), "duration") {
			value = float64(len(summaries))
		} else {
			for _, s := range summaries {
				value += float64(s.SpanCount)
			}
		}
	case "avg":
		for _, s := range summaries {
			value += s.DurationMS
		}
		value /= float64(len(summaries))
	case "max":
		for _, s := range summaries {
			value = math.Max(value, s.DurationMS)
		}
	case "min":
		value = math.MaxFloat64
		for _, s := range summaries {
			value = math.Min(value, s.DurationMS)
		}
	case "error_rate":
		var failed int
		for _, s := range summaries {
			if s.Status == traceStatusError {
				failed++
			}
		}
		value = float64(failed) / float64(len(summaries)) * 100
	default:
		return nil, fmt.Errorf("unsupported trace aggregation %q", c.Aggregation)
	}
	return newResult(c, value, fmt.Sprintf("%s trace/span value %.2f %s %.2f", c.Aggregation, value, c.Operator, c.Threshold)), nil
}

package alert

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/logs"
)

// Log filter keys recognised in `count(logs{...})` conditions.
const (
	logFilterService  = "service_name"
	logFilterSeverity = "severity"
	logFilterSearch   = "search"
)

// otelSeverityNumbers maps severity names to the lowest OpenTelemetry
// SeverityNumber of that level.
var otelSeverityNumbers = map[string]int{
	"TRACE": 1, "DEBUG": 5, "INFO": 9, "WARN": 13, "WARNING": 13, "ERROR": 17, "FATAL": 21,
}

type logReader interface {
	ListLogs(ctx context.Context, filter logs.ListFilter) ([]logs.LogRecord, int, error)
}

// LogEvaluator evaluates `count(logs{...})` conditions.
type LogEvaluator struct {
	reader logReader
	log    *slog.Logger
}

func NewLogEvaluator(reader logReader, log *slog.Logger) *LogEvaluator {
	if log == nil {
		log = slog.Default()
	}
	return &LogEvaluator{reader: reader, log: log}
}

func (e *LogEvaluator) Evaluate(ctx context.Context, alert Alert) (*EvaluationResult, error) {
	return evaluateAlert(ctx, e, alert)
}

func (e *LogEvaluator) evaluate(ctx context.Context, orgID uuid.UUID, c Condition) (*EvaluationResult, error) {
	now := time.Now()
	filter := logs.ListFilter{
		OrgID:       orgID,
		From:        now.Add(-c.Window()),
		To:          now,
		ServiceName: c.Filters[logFilterService],
		MinSeverity: otelSeverityNumbers[strings.ToUpper(strings.TrimSpace(c.Filters[logFilterSeverity]))],
		Search:      c.Filters[logFilterSearch],
		Limit:       1,
	}
	_, total, err := e.reader.ListLogs(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("read log count: %w", err)
	}
	value := float64(total)
	return newResult(c, value, fmt.Sprintf("logs count %d %s %.2f", total, c.Operator, c.Threshold)), nil
}

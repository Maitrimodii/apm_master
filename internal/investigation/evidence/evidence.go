// Package evidence runs the telemetry queries the AI investigator asks for.
// Each query type has one Executor; add a type by adding an executor file and
// registering it in NewRegistry.
package evidence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// QueryType names an evidence query. The values are part of the contract
// with apm-ai-service (its EvidenceType enum).
type QueryType string

const (
	MetricBaseline QueryType = "metric_baseline"
	SlowTraces     QueryType = "slow_traces"
	TraceDetails   QueryType = "trace_details"
	CorrelatedLogs QueryType = "correlated_logs"
)

// ErrInvalidQuery marks a query the caller got wrong (missing or bad parameters).
var ErrInvalidQuery = errors.New("invalid evidence query")

// Query is one request for evidence, already checked against the
// investigation's tenant and time window.
type Query struct {
	TenantID   uuid.UUID
	From       time.Time
	To         time.Time
	Parameters map[string]any
}

// Result is what an executor found.
type Result struct {
	Data        any
	Warnings    []string
	ServiceName string
	Summary     string
}

// Executor runs one query type.
type Executor interface {
	Execute(ctx context.Context, q Query) (Result, error)
}

// Registry maps query types to executors.
type Registry map[QueryType]Executor

// Execute runs q with the executor for qt.
func (r Registry) Execute(ctx context.Context, qt QueryType, q Query) (Result, error) {
	exec, ok := r[qt]
	if !ok {
		return Result{}, fmt.Errorf("%w: unsupported query_type %q", ErrInvalidQuery, qt)
	}
	return exec.Execute(ctx, q)
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidQuery, fmt.Sprintf(format, args...))
}

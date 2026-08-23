package trace

import (
	"context"

	"github.com/google/uuid"
)

// TraceStoreInterface abstracts all database operations for spans and trace summaries.
type TraceStoreInterface interface {
	GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]Span, error)
	ListTraceSummaries(ctx context.Context, query ListSpansQuery) ([]TraceSummary, int, error)
	ListSpans(ctx context.Context, query ListSpansQuery) ([]Span, int, error)
}

// compile-time assertion.
var _ TraceStoreInterface = (*Store)(nil)

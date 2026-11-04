package traces

import (
	"context"

	"github.com/google/uuid"
)

// Reader abstracts all database operations for spans and trace summaries.
type Reader interface {
	GetTrace(ctx context.Context, orgID uuid.UUID, traceID string) ([]Span, error)
	ListTraceSummaries(ctx context.Context, query ListFilter) ([]TraceSummary, int, error)
	ListSpans(ctx context.Context, query ListFilter) ([]Span, int, error)
}

// compile-time assertion.
var _ Reader = (*Store)(nil)

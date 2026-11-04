package logs

import (
	"context"

	"github.com/google/uuid"
)

// Reader is the read side of the log store, used by this module's service,
// alert evaluation and investigation evidence.
type Reader interface {
	ListLogs(ctx context.Context, filter ListFilter) ([]LogRecord, int, error)
	GetTraceLogs(ctx context.Context, orgID uuid.UUID, traceID string) ([]LogRecord, error)
}

var _ Reader = (*Store)(nil)

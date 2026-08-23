package log

import "context"

// LogStoreInterface abstracts database operations for log records.
type LogStoreInterface interface {
	ListLogs(ctx context.Context, query ListLogsQuery) ([]LogRecord, int, error)
	GetTraceLogs(ctx context.Context, orgID, traceID string) ([]LogRecord, error)
}

// compile-time assertion.
var _ LogStoreInterface = (*Store)(nil)

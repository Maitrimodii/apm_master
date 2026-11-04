// Package ctxkeys holds the typed context.Context keys shared by the HTTP
// middlewares, the logger and background workers.
package ctxkeys

import "context"

type key int

const (
	requestIDKey key = iota
	correlationIDKey
	orgIDKey
	userIDKey
)

func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

func WithOrgID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, orgIDKey, id)
}

func WithUserID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

func RequestID(ctx context.Context) string     { return stringValue(ctx, requestIDKey) }
func CorrelationID(ctx context.Context) string { return stringValue(ctx, correlationIDKey) }
func OrgID(ctx context.Context) string         { return stringValue(ctx, orgIDKey) }
func UserID(ctx context.Context) string        { return stringValue(ctx, userIDKey) }

func stringValue(ctx context.Context, k key) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(k).(string)
	return v
}

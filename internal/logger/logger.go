package logger

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	OrgIDKey     ContextKey = "org_id"
	UserIDKey    ContextKey = "user_id"
)

// WithRequestID injects request_id into context using typed ContextKey.
func WithRequestID(ctx context.Context, reqID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, reqID)
}

// WithOrgID injects org_id into context using typed ContextKey.
func WithOrgID(ctx context.Context, orgID string) context.Context {
	return context.WithValue(ctx, OrgIDKey, orgID)
}

// GetRequestID retrieves request_id checking typed ContextKey first, then string fallback.
func GetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(RequestIDKey).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value("request_id").(string); ok && v != "" {
		return v
	}
	return ""
}

// GetOrgID retrieves org_id checking typed ContextKey first, then string fallback.
func GetOrgID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(OrgIDKey).(string); ok && v != "" {
		return v
	}
	if v, ok := ctx.Value("org_id").(string); ok && v != "" {
		return v
	}
	return ""
}

// levelVar allows runtime level changes (e.g. via an admin endpoint).
var levelVar = new(slog.LevelVar)

var SensitiveKeys = []string{
	"recipient",
	"email",
	"token",
	"password",
	"api_key",
	"access_token",
	"refresh_token",
	"key",
	"password_hash",
	"token_hash",
	"key_hash",
}

type Option func(*config)

type config struct {
	writer     io.Writer
	addSource  bool
	textFormat bool
	redact     []string // attr keys to redact
}

func WithWriter(w io.Writer) Option        { return func(c *config) { c.writer = w } }
func WithTextFormat() Option               { return func(c *config) { c.textFormat = true } }
func WithRedactedKeys(keys ...string) Option {
	return func(c *config) { c.redact = append(c.redact, keys...) }
}

type ContextHandler struct {
	slog.Handler
}

func (handler *ContextHandler) Handle(ctx context.Context, logRecord slog.Record) error {
	if ctx != nil {
		if reqID := GetRequestID(ctx); reqID != "" {
			logRecord.AddAttrs(slog.String("request_id", reqID))
		}
		if orgID := GetOrgID(ctx); orgID != "" {
			logRecord.AddAttrs(slog.String("org_id", orgID))
		}
	}
	return handler.Handler.Handle(ctx, logRecord)
}

// New builds the application logger used by HTTP, ingest, and startup code.
func New(serviceName, env, level string, opts ...Option) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo // fallback, don't fail startup over a bad config value
	}
	levelVar.Set(lvl)

	cfg := &config{
		writer: os.Stdout,
		redact: append([]string{}, SensitiveKeys...),
	}
	for _, opt := range opts {
		opt(cfg)
	}

	handlerOpts := &slog.HandlerOptions{
		AddSource:   lvl <= slog.LevelDebug, // only pay stack-walk cost when debugging
		Level:       levelVar,
		ReplaceAttr: redactor(cfg.redact),
	}

	var handler slog.Handler
	if cfg.textFormat {
		handler = slog.NewTextHandler(cfg.writer, handlerOpts)
	} else {
		handler = slog.NewJSONHandler(cfg.writer, handlerOpts)
	}

	return slog.New(
		&ContextHandler{Handler: handler},
	).With(
		"service", serviceName,
		"env", env,
	)
}

// SetLevel allows runtime adjustment, e.g. from an admin/debug HTTP endpoint.
func SetLevel(level string) error {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return err
	}
	levelVar.Set(lvl)
	return nil
}

func redactor(keys []string) func([]string, slog.Attr) slog.Attr {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[strings.ToLower(k)] = true
	}
	return func(groups []string, a slog.Attr) slog.Attr {
		if set[strings.ToLower(a.Key)] {
			return slog.String(a.Key, "[REDACTED]")
		}
		return a
	}
}

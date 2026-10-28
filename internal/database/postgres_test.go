package database

import (
	"context"
	"testing"
)

func TestDefaultPoolConfig(t *testing.T) {
	cfg := DefaultPoolConfig()
	if cfg.MaxConns != DefaultMaxConns {
		t.Errorf("expected MaxConns %d, got %d", DefaultMaxConns, cfg.MaxConns)
	}
	if cfg.MinConns != DefaultMinConns {
		t.Errorf("expected MinConns %d, got %d", DefaultMinConns, cfg.MinConns)
	}
	if cfg.MaxConnIdleTime != DefaultMaxConnIdleTime {
		t.Errorf("expected MaxConnIdleTime %v, got %v", DefaultMaxConnIdleTime, cfg.MaxConnIdleTime)
	}
	if cfg.MaxConnLifetime != DefaultMaxConnLifetime {
		t.Errorf("expected MaxConnLifetime %v, got %v", DefaultMaxConnLifetime, cfg.MaxConnLifetime)
	}
}

func TestConnectValidation(t *testing.T) {
	_, err := Connect(nil, "postgres://user:pass@localhost:5432/db")
	if err == nil {
		t.Fatal("expected error when context is nil")
	}

	_, err = Connect(context.Background(), "")
	if err == nil {
		t.Fatal("expected error when DSN is empty")
	}

	_, err = Connect(context.Background(), "invalid://:::bad-dsn")
	if err == nil {
		t.Fatal("expected error on invalid DSN format")
	}
}

package pg

import (
	"context"
	"testing"
)

func TestConnectValidation(t *testing.T) {
	if _, err := Connect(context.Background(), "", PoolConfig{}); err == nil {
		t.Fatal("expected error when DSN is empty")
	}
	if _, err := Connect(context.Background(), "invalid://:::bad-dsn", PoolConfig{}); err == nil {
		t.Fatal("expected error on invalid DSN format")
	}
}

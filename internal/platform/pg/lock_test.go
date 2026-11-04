package pg

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestWithAdvisoryLockReleasesOnSameSession reproduces the leak where the
// unlock ran on a different pooled connection than the lock.
func TestWithAdvisoryLockReleasesOnSameSession(t *testing.T) {
	dsn := os.Getenv("APM_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("set APM_INTEGRATION_DSN to run advisory lock tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, strings.Replace(dsn, "pgx5://", "postgres://", 1))
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)

	const key int64 = 0x7E57
	var runs atomic.Int32
	for i := 0; i < 5; i++ {
		ran, err := WithAdvisoryLock(ctx, pool, key, func(context.Context) error {
			runs.Add(1)
			return nil
		})
		if err != nil || !ran {
			t.Fatalf("run %d: WithAdvisoryLock() = (%v, %v), want (true, nil)", i, ran, err)
		}
	}
	if runs.Load() != 5 {
		t.Fatalf("fn ran %d times, want 5 — lock was not released between runs", runs.Load())
	}

	var held bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND objid = $1)", key).Scan(&held); err != nil {
		t.Fatalf("query pg_locks: %v", err)
	}
	if held {
		t.Fatal("advisory lock still held after WithAdvisoryLock returned")
	}
}

func TestWithAdvisoryLockIsExclusive(t *testing.T) {
	dsn := os.Getenv("APM_INTEGRATION_DSN")
	if dsn == "" {
		t.Skip("set APM_INTEGRATION_DSN to run advisory lock tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, strings.Replace(dsn, "pgx5://", "postgres://", 1))
	if err != nil {
		t.Fatalf("pgxpool.New() error = %v", err)
	}
	t.Cleanup(pool.Close)

	const key int64 = 0x7E58
	_, err = WithAdvisoryLock(ctx, pool, key, func(ctx context.Context) error {
		ran, err := WithAdvisoryLock(ctx, pool, key, func(context.Context) error { return nil })
		if err != nil {
			t.Fatalf("nested WithAdvisoryLock() error = %v", err)
		}
		if ran {
			t.Fatal("second holder ran while the lock was held")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WithAdvisoryLock() error = %v", err)
	}
}

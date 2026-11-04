package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Advisory lock keys. Each cluster-wide singleton task needs its own key.
const (
	LockAlertEvaluation int64 = 0xA1A2A3A4
	LockRetention       int64 = 0xB1B2B3B4
)

// WithAdvisoryLock runs fn while holding a session-level advisory lock, so
// only one instance in the cluster runs it at a time. It reports false
// without running fn when another session holds the lock.
//
// Session locks belong to a connection, so the lock, fn's lifetime and the
// unlock all use one dedicated connection taken from the pool. Releasing the
// connection also releases the lock if unlocking fails.
func WithAdvisoryLock(ctx context.Context, pool *pgxpool.Pool, key int64, fn func(ctx context.Context) error) (bool, error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return false, fmt.Errorf("acquire connection for advisory lock: %w", err)
	}
	defer conn.Release()

	var acquired bool
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&acquired); err != nil {
		return false, fmt.Errorf("try advisory lock %d: %w", key, err)
	}
	if !acquired {
		return false, nil
	}

	runErr := fn(ctx)

	// Unlock with a fresh context: ctx may already be cancelled at shutdown.
	var released bool
	if err := conn.QueryRow(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", key).Scan(&released); err != nil || !released {
		// Drop the connection so the server ends the session and frees the lock.
		_ = conn.Conn().Close(context.WithoutCancel(ctx))
		if runErr == nil {
			runErr = fmt.Errorf("release advisory lock %d: %v", key, err)
		}
	}
	return true, runErr
}

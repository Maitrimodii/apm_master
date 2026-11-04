package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PoolConfig sizes the connection pool. Defaults live in internal/config.
type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnIdleTime time.Duration
	MaxConnLifetime time.Duration
}

// Connect opens a pool for dsn and pings it.
func Connect(ctx context.Context, dsn string, cfg PoolConfig) (*pgxpool.Pool, error) {
	if dsn == "" {
		return nil, errors.New("database DSN is required")
	}
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database dsn: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

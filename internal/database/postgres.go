package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Default connection pool configurations for production workloads.
const (
	DefaultMaxConns        int32         = 50
	DefaultMinConns        int32         = 5
	DefaultMaxConnIdleTime time.Duration = 15 * time.Minute
	DefaultMaxConnLifetime time.Duration = 1 * time.Hour
)

type PoolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnIdleTime time.Duration
	MaxConnLifetime time.Duration
}

// DefaultPoolConfig returns a PoolConfig initialized with standard production defaults.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns:        DefaultMaxConns,
		MinConns:        DefaultMinConns,
		MaxConnIdleTime: DefaultMaxConnIdleTime,
		MaxConnLifetime: DefaultMaxConnLifetime,
	}
}

type Database struct {
	pool *pgxpool.Pool
}

// Connect establishes a connection to the PostgreSQL database using the provided DSN.
func Connect(ctx context.Context, dsn string, opts ...PoolConfig) (*Database, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}

	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse database dsn: %w", err)
	}

	// Sane defaults for production workloads
	defaults := DefaultPoolConfig()
	poolCfg.MaxConns = defaults.MaxConns
	poolCfg.MinConns = defaults.MinConns
	poolCfg.MaxConnIdleTime = defaults.MaxConnIdleTime
	poolCfg.MaxConnLifetime = defaults.MaxConnLifetime

	if len(opts) > 0 {
		opt := opts[0]
		if opt.MaxConns > 0 {
			poolCfg.MaxConns = opt.MaxConns
		}
		if opt.MinConns > 0 {
			poolCfg.MinConns = opt.MinConns
		}
		if opt.MaxConnIdleTime > 0 {
			poolCfg.MaxConnIdleTime = opt.MaxConnIdleTime
		}
		if opt.MaxConnLifetime > 0 {
			poolCfg.MaxConnLifetime = opt.MaxConnLifetime
		}
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	// verify connection is actually working at startup
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Database{pool: pool}, nil
}

// Close terminates the database connection pool, releasing all resources.
func (database *Database) Close() {
	database.pool.Close()
}

func (database *Database) Pool() *pgxpool.Pool {
	return database.pool
}

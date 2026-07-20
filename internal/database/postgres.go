package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Database struct {
	pool *pgxpool.Pool
}

// Connect establishes a connection to the PostgreSQL database using the provided DSN.
func Connect(ctx context.Context, dsn string) (*Database, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}

	if dsn == "" {
		return nil, fmt.Errorf("database DSN is required")
	}

	pool, err := pgxpool.New(ctx, dsn)
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

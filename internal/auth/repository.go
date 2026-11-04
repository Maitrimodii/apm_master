package auth

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	database *pgxpool.Pool
}

func NewRepository(database *pgxpool.Pool) *Repository {
	return &Repository{database: database}
}

func (r *Repository) Query() Querier {
	return r.database
}

type Querier interface {
	Query(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgx.Rows, error)

	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row

	Exec(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgconn.CommandTag, error)
}

func (r *Repository) WithTx(
	ctx context.Context,
	fn func(Querier) error,
) error {
	transaction, err := r.database.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	defer func() { _ = transaction.Rollback(ctx) }()

	if err := fn(transaction); err != nil {
		slog.WarnContext(ctx, "database transaction failed, rolling back", "error", err)
		return err
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}

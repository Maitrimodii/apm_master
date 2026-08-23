package retention

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) DeleteOldRecords(ctx context.Context, table, timeCol string, days int, batchSize int) (int64, error) {
	// Using dynamic SQL for table/column names is generally risky, but this is an internal
	// administrative task where table names are hardcoded by us, not user input.
	// To prevent locking the entire table for a long time, we delete in a loop using a CTE.
	
	query := `
		WITH deleted AS (
			DELETE FROM ` + table + `
			WHERE ctid IN (
				SELECT ctid
				FROM ` + table + `
				WHERE ` + timeCol + ` < NOW() - INTERVAL '1 day' * $1
				LIMIT $2
			)
			RETURNING 1
		)
		SELECT count(*) FROM deleted;
	`
	
	var deletedCount int64
	err := s.pool.QueryRow(ctx, query, days, batchSize).Scan(&deletedCount)
	return deletedCount, err
}

func (s *Store) AcquireLock(ctx context.Context, lockKey int64) (bool, error) {
	var acquired bool
	err := s.pool.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", lockKey).Scan(&acquired)
	return acquired, err
}

func (s *Store) ReleaseLock(ctx context.Context, lockKey int64) error {
	_, err := s.pool.Exec(ctx, "SELECT pg_advisory_unlock($1)", lockKey)
	return err
}

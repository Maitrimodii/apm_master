package retention

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// DeleteOldRecords deletes up to batchSize rows of table whose timeCol is
// older than days, and reports how many it deleted. Deleting in batches keeps
// each statement's locks short.
//
// table and timeCol come from the Policy list in this package, never from
// user input, which is why they are spliced into the SQL.
func (s *Store) DeleteOldRecords(ctx context.Context, table, timeCol string, days, batchSize int) (int64, error) {
	query := `
		WITH deleted AS (
			DELETE FROM ` + table + `
			WHERE ctid IN (
				SELECT ctid FROM ` + table + `
				WHERE ` + timeCol + ` < NOW() - INTERVAL '1 day' * $1
				LIMIT $2
			)
			RETURNING 1
		)
		SELECT count(*) FROM deleted`
	var deleted int64
	if err := s.pool.QueryRow(ctx, query, days, batchSize).Scan(&deleted); err != nil {
		return 0, fmt.Errorf("delete old %s rows: %w", table, err)
	}
	return deleted, nil
}

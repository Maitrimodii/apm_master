package database

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Migrate runs all pending migrations from the given directory.
// Each migration runs in a transaction — if it fails it rolls back cleanly.
func Migrate(dsn string, migrationsDir string) (err error) {
	// source — where migration files are
	// database — where to apply them
	mapItem, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsDir),
		dsn,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		if closeErr := closeMigrator(mapItem); closeErr != nil {
			if err != nil {
				err = errors.Join(err, closeErr)
				return
			}
			err = closeErr
		}
	}()

	if err := mapItem.Up(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

func MigrateDown(dsn string, migrationsDir string) (err error) {
	mapItem, err := migrate.New(
		fmt.Sprintf("file://%s", migrationsDir),
		dsn,
	)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		if closeErr := closeMigrator(mapItem); closeErr != nil {
			if err != nil {
				err = errors.Join(err, closeErr)
				return
			}
			err = closeErr
		}
	}()

	if err := mapItem.Down(); err != nil {
		if errors.Is(err, migrate.ErrNoChange) {
			return nil
		}
		return fmt.Errorf("failed to roll back migrations: %w", err)
	}

	return nil
}

func closeMigrator(mapItem *migrate.Migrate) error {
	sourceErr, databaseErr := mapItem.Close()
	if sourceErr == nil && databaseErr == nil {
		return nil
	}
	return fmt.Errorf("close migrator: %w", errors.Join(sourceErr, databaseErr))
}

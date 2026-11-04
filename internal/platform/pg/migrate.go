package pg

import (
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// Migrate applies all pending migrations from dir. Each migration runs in a
// transaction, so a failing one rolls back cleanly.
func Migrate(dsn, dir string) error {
	return withMigrator(dsn, dir, func(m *migrate.Migrate) error {
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("run migrations: %w", err)
		}
		return nil
	})
}

// MigrateDown rolls back every migration.
func MigrateDown(dsn, dir string) error {
	return withMigrator(dsn, dir, func(m *migrate.Migrate) error {
		if err := m.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return fmt.Errorf("roll back migrations: %w", err)
		}
		return nil
	})
}

// MigrateVersion reports the current schema version and whether the last
// migration failed half-way (dirty).
func MigrateVersion(dsn, dir string) (version uint, dirty bool, err error) {
	err = withMigrator(dsn, dir, func(m *migrate.Migrate) error {
		version, dirty, err = m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			return nil
		}
		return err
	})
	return version, dirty, err
}

// MigrateForce sets the schema version without running migrations. Use it
// only to recover from a dirty state after fixing the schema by hand.
func MigrateForce(dsn, dir string, version int) error {
	return withMigrator(dsn, dir, func(m *migrate.Migrate) error {
		if err := m.Force(version); err != nil {
			return fmt.Errorf("force migration version %d: %w", version, err)
		}
		return nil
	})
}

func withMigrator(dsn, dir string, fn func(*migrate.Migrate) error) (err error) {
	m, err := migrate.New("file://"+dir, dsn)
	if err != nil {
		return fmt.Errorf("create migrator: %w", err)
	}
	defer func() {
		srcErr, dbErr := m.Close()
		if closeErr := errors.Join(srcErr, dbErr); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migrator: %w", closeErr))
		}
	}()
	return fn(m)
}

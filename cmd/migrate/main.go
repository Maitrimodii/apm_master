// Command migrate manages the database schema. Run it as a deploy step;
// the API only auto-migrates in development (see AUTO_MIGRATE).
//
//	migrate -action up|down|reset|status
//	migrate -action force -version N     # mark version N clean after a manual fix
//	migrate -action clear-telemetry      # empty spans, logs and metric tables
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/Maitrimodii/apm_master/internal/app"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/platform/pg"
)

// telemetryTables are emptied by clear-telemetry, children first.
var telemetryTables = []string{"spans", "logs", "metric_histogram_buckets", "metrics", "metric_metadata"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	action := flag.String("action", "up", "up, down, reset, status, force or clear-telemetry")
	version := flag.Int("version", -1, "target version for -action force")
	dir := flag.String("dir", app.MigrationsDir, "directory containing the .sql migrations")
	force := flag.Bool("force", false, "allow up/down/reset while the schema is dirty")
	flag.Parse()

	if _, err := os.Stat(*dir); err != nil {
		return fmt.Errorf("migrations directory %q: %w", *dir, err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	dsn := cfg.Postgres.MigrateDSN()

	switch *action {
	case "status":
		v, dirty, err := pg.MigrateVersion(dsn, *dir)
		if err != nil {
			return err
		}
		fmt.Printf("version %d, dirty %v\n", v, dirty)
		return nil
	case "force":
		if *version < 0 {
			return errors.New("-action force needs -version N")
		}
		if err := pg.MigrateForce(dsn, *dir, *version); err != nil {
			return err
		}
		fmt.Printf("forced version %d\n", *version)
		return nil
	case "up", "down", "reset":
		if err := refuseIfDirty(dsn, *dir, *force); err != nil {
			return err
		}
		if *action != "up" {
			if err := pg.MigrateDown(dsn, *dir); err != nil {
				return err
			}
			fmt.Println("rolled back all migrations")
		}
		if *action != "down" {
			if err := pg.Migrate(dsn, *dir); err != nil {
				return err
			}
			fmt.Println("applied all migrations")
		}
		return nil
	case "clear-telemetry":
		return clearTelemetry(cfg)
	default:
		return fmt.Errorf("unknown action %q", *action)
	}
}

// refuseIfDirty stops when the last migration failed half-way. The schema
// then needs a manual fix and `-action force -version N` before continuing;
// guessing the version silently can corrupt data.
func refuseIfDirty(dsn, dir string, force bool) error {
	v, dirty, err := pg.MigrateVersion(dsn, dir)
	if err != nil {
		return err
	}
	if dirty && !force {
		return fmt.Errorf("schema version %d is dirty: fix it by hand, then run -action force -version <clean version> (or pass -force)", v)
	}
	return nil
}

func clearTelemetry(cfg *config.Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Postgres.Timeout)
	defer cancel()
	pool, err := pg.Connect(ctx, cfg.Postgres.PoolDSN(), pg.PoolConfig{MaxConns: 1})
	if err != nil {
		return err
	}
	defer pool.Close()
	for _, table := range telemetryTables {
		// Table names come from the constant list above, never from input.
		if _, err := pool.Exec(ctx, "TRUNCATE TABLE "+table+" CASCADE"); err != nil {
			return fmt.Errorf("truncate %s: %w", table, err)
		}
		fmt.Println("cleared", table)
	}
	return nil
}

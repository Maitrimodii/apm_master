package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/database"
)

func main() {
	action := flag.String("action", "up", "Migration action: up, down, reset, status, force")
	forceVersion := flag.Int("version", -1, "Target version for force action")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	dsn := cfg.GetPostgresDSN()

	// Locate migrations directory
	migrationsDir := "migrations"
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		migrationsDir = filepath.Join("..", "migrations")
		if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
			migrationsDir = "/home/maitri/project/apm_master/migrations"
		}
	}

	switch *action {
	case "status":
		v, dirty, err := database.MigrateVersion(dsn, migrationsDir)
		if err != nil {
			log.Fatalf("failed to get migration version: %v", err)
		}
		fmt.Printf("Current version: %d, Dirty: %v\n", v, dirty)

	case "force":
		if *forceVersion < 0 {
			log.Fatalf("Please provide a valid -version for force action")
		}
		if err := database.MigrateForce(dsn, migrationsDir, *forceVersion); err != nil {
			log.Fatalf("failed to force version: %v", err)
		}
		fmt.Printf("Successfully forced migration version to %d.\n", *forceVersion)

	case "down":
		// Check if dirty before rolling down
		if v, dirty, err := database.MigrateVersion(dsn, migrationsDir); err == nil && dirty {
			fmt.Printf("Migration %d is dirty, clearing dirty flag before rollback...\n", v)
			_ = database.MigrateForce(dsn, migrationsDir, int(v))
		}
		fmt.Println("==> Rolling down all migrations...")
		if err := database.MigrateDown(dsn, migrationsDir); err != nil {
			log.Fatalf("failed to roll back migrations: %v", err)
		}
		fmt.Println("==> Successfully rolled down all database migrations.")

	case "up":
		if v, dirty, err := database.MigrateVersion(dsn, migrationsDir); err == nil && dirty {
			fmt.Printf("Migration %d is dirty, forcing clean version %d before applying...\n", v, v-1)
			_ = database.MigrateForce(dsn, migrationsDir, int(v-1))
		}
		fmt.Println("==> Applying all migrations...")
		if err := database.Migrate(dsn, migrationsDir); err != nil {
			log.Fatalf("failed to apply migrations: %v", err)
		}
		fmt.Println("==> Successfully applied all database migrations.")

	case "reset":
		if v, dirty, err := database.MigrateVersion(dsn, migrationsDir); err == nil && dirty {
			fmt.Printf("Migration %d is dirty, clearing dirty flag...\n", v)
			_ = database.MigrateForce(dsn, migrationsDir, int(v))
		}
		fmt.Println("==> Step 1: Rolling down all database migrations...")
		if err := database.MigrateDown(dsn, migrationsDir); err != nil {
			log.Fatalf("failed to roll back migrations: %v", err)
		}
		fmt.Println("==> Successfully rolled down all database migrations.")

		fmt.Println("==> Step 2: Applying all database migrations from scratch...")
		if err := database.Migrate(dsn, migrationsDir); err != nil {
			log.Fatalf("failed to apply migrations: %v", err)
		}
		fmt.Println("==> Successfully recreated all database schema and tables.")

	case "clear-telemetry", "truncate":
		fmt.Println("==> Truncating telemetry tables (spans, logs, metrics)...")
		dbCtx, dbCancel := context.WithTimeout(context.Background(), cfg.PostgresTimeout)
		defer dbCancel()
		pool, err := database.Connect(dbCtx, cfg.GetPoolDSN())
		if err != nil {
			log.Fatalf("failed to connect to database: %v", err)
		}
		defer pool.Close()

		for _, table := range []string{"spans", "logs", "metric_histogram_buckets", "metrics", "metric_metadata"} {
			_, err := pool.Pool().Exec(dbCtx, fmt.Sprintf("TRUNCATE TABLE %s CASCADE;", table))
			if err != nil {
				_, _ = pool.Pool().Exec(dbCtx, fmt.Sprintf("DELETE FROM %s;", table))
			}
			fmt.Printf("    Cleared: %s\n", table)
		}
		fmt.Println("==> Telemetry tables successfully cleared!")

	default:
		log.Fatalf("Unknown action: %q. Allowed: up, down, reset, status, force, clear-telemetry", *action)
	}
}

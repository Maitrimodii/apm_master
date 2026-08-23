package server

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const healthCheckTimeout = 3 * time.Second

// ComponentStatus holds the health status of a single dependency.
type ComponentStatus struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// HealthReport is the response payload from /health.
type HealthReport struct {
	OK         bool                       `json:"ok"`
	Components map[string]ComponentStatus `json:"components"`
}

// HealthChecker checks the liveness of critical dependencies.
type HealthChecker struct {
	db  *pgxpool.Pool
	rdb *redis.Client
}

// NewHealthChecker creates a HealthChecker with Postgres and Redis clients.
func NewHealthChecker(db *pgxpool.Pool, rdb *redis.Client) *HealthChecker {
	return &HealthChecker{db: db, rdb: rdb}
}

// Check pings each dependency and returns a HealthReport.
// The report is marked OK only when all components respond successfully.
func (h *HealthChecker) Check(ctx context.Context) HealthReport {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	components := make(map[string]ComponentStatus, 2)

	// Postgres
	pgStatus := ComponentStatus{OK: true}
	if err := h.db.Ping(ctx); err != nil {
		pgStatus.OK = false
		pgStatus.Error = err.Error()
	}
	components["postgres"] = pgStatus

	// Redis
	redisStatus := ComponentStatus{OK: true}
	if err := h.rdb.Ping(ctx).Err(); err != nil {
		redisStatus.OK = false
		redisStatus.Error = err.Error()
	}
	components["redis"] = redisStatus

	allOK := pgStatus.OK && redisStatus.OK
	return HealthReport{OK: allOK, Components: components}
}

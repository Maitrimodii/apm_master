package integration_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/database"
	"github.com/Maitrimodii/apm_master/internal/ingest/writer"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const integrationDSNEnv = "APM_INTEGRATION_DSN"

func integrationDSN(testRunner *testing.T) string {
	testRunner.Helper()

	dsn := os.Getenv(integrationDSNEnv)
	if dsn == "" {
		testRunner.Skipf("set %s to run Postgres/Timescale integration tests", integrationDSNEnv)
	}
	return dsn
}

func integrationPool(testRunner *testing.T) *pgxpool.Pool {
	testRunner.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	dsn := integrationDSN(testRunner)
	if err := database.Migrate(dsn, "../../migrations"); err != nil {
		testRunner.Fatalf("db.Migrate() error = %v", err)
	}

	poolDSN := strings.Replace(dsn, "pgx5://", "postgres://", 1)
	pool, err := pgxpool.New(ctx, poolDSN)
	if err != nil {
		testRunner.Fatalf("pgxpool.New() error = %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		testRunner.Fatalf("pool.Ping() error = %v", err)
	}

	testRunner.Cleanup(pool.Close)
	return pool
}

func TestMigrationsApply(testRunner *testing.T) {
	pool := integrationPool(testRunner)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var tableCount int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
		  AND table_name IN ('spans', 'metrics', 'logs')
	`).Scan(&tableCount); err != nil {
		testRunner.Fatalf("check telemetry tables: %v", err)
	}

	if tableCount != 3 {
		testRunner.Fatalf("telemetry table count = %d, want 3", tableCount)
	}
}

func TestTelemetryRoundTrip(testRunner *testing.T) {
	pool := integrationPool(testRunner)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	orgID := uuid.New()
	traceID := "trace-" + uuid.NewString()
	spanID := "span-" + uuid.NewString()
	now := time.Now().UTC().Truncate(time.Millisecond)

	if err := writer.NewSpanWriter(pool).BulkInsert(ctx, []trace.Span{{
		SpanID:        spanID,
		TraceID:       traceID,
		OrgID:         orgID,
		ServiceName:   "checkout",
		SpanName:      "POST /checkout",
		SpanKind:      "SERVER",
		StartTime:     now,
		EndTime:       now.Add(50 * time.Millisecond),
		DurationNS:    int64(50 * time.Millisecond),
		StatusCode:    "OK",
		ResourceAttrs: map[string]any{"service.name": "checkout"},
		SpanAttrs:     map[string]any{"http.method": "POST"},
		Events:        []trace.SpanEvent{},
		Links:         []trace.SpanLink{},
	}}); err != nil {
		testRunner.Fatalf("insert span: %v", err)
	}

	gotTrace, err := trace.NewStore(pool).GetTrace(ctx, orgID, traceID)
	if err != nil {
		testRunner.Fatalf("GetTrace() error = %v", err)
	}
	if len(gotTrace) != 1 || gotTrace[0].SpanID != spanID {
		testRunner.Fatalf("GetTrace() returned %+v, want span %s", gotTrace, spanID)
	}

	logTraceID := traceID
	if err := writer.NewLogWriter(pool).BulkInsert(ctx, []log.LogRecord{{
		Time:          now,
		OrgID:         orgID,
		ServiceName:   "checkout",
		TraceID:       &logTraceID,
		SeverityNum:   17,
		SeverityText:  "ERROR",
		Body:          "payment timeout",
		ResourceAttrs: map[string]any{"service.name": "checkout"},
		LogAttrs:      map[string]any{"component": "payments"},
	}}); err != nil {
		testRunner.Fatalf("insert log: %v", err)
	}

	gotLogs, _, err := log.NewStore(pool).ListLogs(ctx, log.ListLogsQuery{
		OrgID:   orgID.String(),
		TraceID: traceID,
		Limit:   10,
	})
	if err != nil {
		testRunner.Fatalf("ListLogs() error = %v", err)
	}
	if len(gotLogs) != 1 || gotLogs[0].Body != "payment timeout" {
		testRunner.Fatalf("ListLogs() returned %+v, want payment timeout", gotLogs)
	}

	if err := writer.NewMetricWriter(pool).BulkInsert(ctx, []metric.WriteBatch{{
		Metrics: []metric.Metric{{
			Time:       now,
			OrgID:      orgID,
			MetricName: "checkout.latency",
			MetricType: metric.MetricTypeGauge,
			Value:      42,
			Labels:     map[string]any{"service.name": "checkout"},
		}},
		Buckets: []metric.HistogramBucket{{
			Time:       now,
			OrgID:      orgID,
			MetricName: "checkout.latency",
			Labels:     map[string]any{"service.name": "checkout"},
			Le:         50,
			Count:      1,
		}},
	}}); err != nil {
		testRunner.Fatalf("insert metric: %v", err)
	}

	points, _, err := metric.NewStore(pool).GetSeries(ctx, metric.SeriesQuery{
		OrgID:      orgID,
		MetricName: "checkout.latency",
		From:       now.Add(-time.Minute),
		To:         now.Add(time.Minute),
	})
	if err != nil {
		testRunner.Fatalf("GetSeries() error = %v", err)
	}
	if len(points) == 0 {
		testRunner.Fatal("expected at least one metric point")
	}
}

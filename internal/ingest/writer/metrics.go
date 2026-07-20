package writer

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type MetricWriter struct {
	pool *pgxpool.Pool
}

func NewMetricWriter(pool *pgxpool.Pool) *MetricWriter {
	return &MetricWriter{pool: pool}
}

// BulkInsert writes metrics and histogram buckets atomically.
// Both tables are written in the same transaction — a histogram metric
// with no buckets, or buckets with no parent metric row, is corrupt data.
func (writer *MetricWriter) BulkInsert(
	ctx context.Context,
	batch []metric.WriteBatch,
) error {
	if len(batch) == 0 {
		return nil
	}

	totalMetrics := 0
	totalBuckets := 0

	for _, writeBatch := range batch {
		totalMetrics += len(writeBatch.Metrics)
		totalBuckets += len(writeBatch.Buckets)
	}

	metricBatch := make([]metric.Metric, 0, totalMetrics)
	bucketBatch := make([]metric.HistogramBucket, 0, totalBuckets)

	for _, writeBatch := range batch {
		metricBatch = append(metricBatch, writeBatch.Metrics...)
		bucketBatch = append(bucketBatch, writeBatch.Buckets...)
	}

	return writer.BulkInsertWithBuckets(
		ctx,
		metricBatch,
		bucketBatch,
	)
}

// BulkInsertWithBuckets writes metrics and their histogram buckets atomically.
// Called by the pipeline when the batch contains histogram metric.
func (writer *MetricWriter) BulkInsertWithBuckets(ctx context.Context, batch []metric.Metric, buckets []metric.HistogramBucket) error {
	if len(batch) == 0 {
		return nil
	}

	transaction, err := writer.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer transaction.Rollback(ctx)

	if err := writer.insertMetrics(ctx, transaction, batch); err != nil {
		return err
	}

	if len(buckets) > 0 {
		if err := writer.insertBuckets(ctx, transaction, buckets); err != nil {
			return err
		}
	}

	if err := writer.upsertMetadata(ctx, transaction, batch, buckets); err != nil {
		return err
	}

	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit metric+bucket batch: %w", err)
	}

	return nil
}

func (writer *MetricWriter) upsertMetadata(ctx context.Context, transaction pgx.Tx, batch []metric.Metric, buckets []metric.HistogramBucket) error {
	type metaKey struct {
		orgID uuid.UUID
		name  string
	}
	type metaVal struct {
		mType string
		unit  string
	}
	uniqueMeta := make(map[metaKey]metaVal)

	for _, metricItem := range batch {
		key := metaKey{orgID: metricItem.OrgID, name: metricItem.MetricName}
		unit := ""
		if u, exists := metricItem.Labels["unit"]; exists {
			if s, exists := u.(string); exists {
				unit = s
			}
		}
		uniqueMeta[key] = metaVal{mType: string(metricItem.MetricType), unit: unit}
	}

	for _, bucketItem := range buckets {
		key := metaKey{orgID: bucketItem.OrgID, name: bucketItem.MetricName}
		unit := ""
		if u, exists := bucketItem.Labels["unit"]; exists {
			if s, exists := u.(string); exists {
				unit = s
			}
		}
		if val, exists := uniqueMeta[key]; exists {
			if val.unit == "" && unit != "" {
				val.unit = unit
				uniqueMeta[key] = val
			}
		} else {
			uniqueMeta[key] = metaVal{mType: "histogram", unit: unit}
		}
	}

	for key, val := range uniqueMeta {
		_, err := transaction.Exec(ctx, `
			INSERT INTO metric_metadata (org_id, metric_name, metric_type, unit)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (org_id, metric_name) DO UPDATE
			SET metric_type = EXCLUDED.metric_type,
			    unit = CASE WHEN EXCLUDED.unit <> '' THEN EXCLUDED.unit ELSE metric_metadata.unit END
		`, key.orgID, key.name, val.mType, val.unit)
		if err != nil {
			return fmt.Errorf("upsert metric metadata: %w", err)
		}
	}

	return nil
}

func (writer *MetricWriter) insertMetrics(ctx context.Context, transaction pgx.Tx, batch []metric.Metric) error {
	rows := make([][]any, 0, len(batch))

	for _, metricItem := range batch {
		labelsJSON, err := json.Marshal(metricItem.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for metric %s: %w", metricItem.MetricName, err)
		}

		rows = append(rows, []any{
			metricItem.Time,
			metricItem.OrgID,
			metricItem.MetricName,
			string(metricItem.MetricType),
			metricItem.Value,
			labelsJSON,
		})
	}

	cols := []string{"time", "org_id", "metric_name", "metric_type", "value", "labels"}

	_, err := transaction.CopyFrom(
		ctx,
		pgx.Identifier{"metrics"},
		cols,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("bulk insert %d metrics: %w", len(batch), err)
	}

	return nil
}

func (writer *MetricWriter) insertBuckets(ctx context.Context, transaction pgx.Tx, buckets []metric.HistogramBucket) error {
	rows := make([][]any, 0, len(buckets))

	for _, bucketItem := range buckets {
		labelsJSON, err := json.Marshal(bucketItem.Labels)
		if err != nil {
			return fmt.Errorf("marshal labels for histogram bucket %s le=%v: %w", bucketItem.MetricName, bucketItem.Le, err)
		}

		rows = append(rows, []any{
			bucketItem.Time,
			bucketItem.OrgID,
			bucketItem.MetricName,
			labelsJSON,
			bucketItem.Le,
			bucketItem.Count,
		})
	}

	cols := []string{"time", "org_id", "metric_name", "labels", "le", "count"}

	_, err := transaction.CopyFrom(
		ctx,
		pgx.Identifier{"metric_histogram_buckets"},
		cols,
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("bulk insert %d histogram buckets: %w", len(buckets), err)
	}

	return nil
}

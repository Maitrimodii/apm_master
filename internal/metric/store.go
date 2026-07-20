package metric

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultLimit = 100

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ListMetricsQuery is the filter for listing distinct metric names.
type ListMetricsQuery struct {
	OrgID       uuid.UUID // consistent with Metric.OrgID
	ServiceName string    // filters by labels->>'service.name'
	Limit       int
	Offset      int
}

// SeriesQuery is the filter for fetching a metric time series.
type SeriesQuery struct {
	OrgID      uuid.UUID // consistent with Metric.OrgID
	MetricName string
	From       time.Time
	To         time.Time
	Labels     map[string]string // exact label matchers, all must match
	Interval   string            // optional custom aggregation interval (e.g., "5m", "1h")
}

type ListMetricValuesQuery struct {
	OrgID       uuid.UUID
	MetricName  string
	ServiceName string
	Limit       int
	Offset      int
}

type AggregationQuery struct {
	OrgID      uuid.UUID
	MetricName string
	From       time.Time
	To         time.Time
	Labels     map[string]string
}

// ListMetricNames returns distinct metric names for an org.
// Used by the dashboard metric picker.
func (store *Store) ListMetricNames(ctx context.Context, query ListMetricsQuery) ([]string, int, error) {
	// Guard against a zero Limit silently returning no rows.
	if query.Limit <= 0 {
		query.Limit = defaultLimit
	}

	args := []any{query.OrgID}
	filter := ""
	nextArg := 2

	if query.ServiceName != "" {
		filter += fmt.Sprintf(` AND labels->>'service.name' = $%d`, nextArg)
		args = append(args, query.ServiceName)
		nextArg++
	}

	var total int
	err := store.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT metric_name) FROM metrics WHERE org_id = $1`+filter,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count metric names: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, query.Limit, query.Offset)

	rows, err := store.pool.Query(ctx, fmt.Sprintf(`
		SELECT DISTINCT metric_name
		FROM metrics
		WHERE org_id = $1%s
		ORDER BY metric_name ASC
		LIMIT $%d OFFSET $%d`, filter, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list metric names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, 0, fmt.Errorf("scan metric name: %w", err)
		}
		names = append(names, name)
	}

	return names, total, rows.Err()
}

// GetSeries returns aggregated time series data for a single metric.
// Automatically selects the right continuous aggregate based on time range, or uses the specified Interval.
func (store *Store) GetSeries(ctx context.Context, query SeriesQuery) ([]MetricPoint, Resolution, error) {
	var table string
	var bucket string
	var isRaw bool
	var resolution Resolution

	if query.Interval != "" {
		switch query.Interval {
		case "1m":
			table = "metrics_1m"
			bucket = "1 minute"
			resolution = Resolution1m
		case "5m":
			table = "metrics_5m"
			bucket = "5 minutes"
			resolution = Resolution5m
		case "15m":
			table = "metrics"
			bucket = "15 minutes"
			isRaw = true
			resolution = ResolutionRaw
		case "30m":
			table = "metrics"
			bucket = "30 minutes"
			isRaw = true
			resolution = ResolutionRaw
		case "1h":
			table = "metrics_1h"
			bucket = "1 hour"
			resolution = Resolution1h
		case "24h":
			table = "metrics"
			bucket = "24 hours"
			isRaw = true
			resolution = ResolutionRaw
		default:
			resolution = selectResolution(query.From, query.To)
			table = resolutionTable(resolution)
			bucket = resolutionBucket(resolution)
			isRaw = (resolution == ResolutionRaw)
		}
	} else {
		resolution = selectResolution(query.From, query.To)
		table = resolutionTable(resolution)
		bucket = resolutionBucket(resolution)
		isRaw = (resolution == ResolutionRaw)
	}

	args := []any{query.OrgID, query.From, query.To, query.MetricName}
	filter := ""
	nextArg := 5

	// Label matchers — each key/value pair appended as a JSONB containment check.
	// Using @> operator with a constructed JSONB object is index-friendly.
	for key, value := range query.Labels {
		labelJSON, _ := json.Marshal(map[string]string{key: value})
		filter += fmt.Sprintf(` AND labels @> $%d::jsonb`, nextArg)
		args = append(args, string(labelJSON))
		nextArg++
	}

	var sqlQuery string

	if isRaw {
		// Raw table: we bucket on the fly since there's no pre-aggregation.
		sqlQuery = fmt.Sprintf(`
			SELECT
				time_bucket('%s', time) AS bucket,
				AVG(value)              AS avg_value,
				MIN(value)              AS min_value,
				MAX(value)              AS max_value,
				COUNT(*)                AS sample_count
			FROM %s
			WHERE org_id = $1
			  AND time >= $2
			  AND time <= $3
			  AND metric_name = $4
			  %s
			GROUP BY bucket
			ORDER BY bucket ASC`, bucket, table, filter)
	} else {
		// Continuous aggregate views already have pre-bucketed columns.
		sqlQuery = fmt.Sprintf(`
			SELECT
				bucket,
				avg_value,
				min_value,
				max_value,
				sample_count
			FROM %s
			WHERE org_id = $1
			  AND bucket >= $2
			  AND bucket <= $3
			  AND metric_name = $4
			  %s
			ORDER BY bucket ASC`, table, filter)
	}

	rows, err := store.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, resolution, fmt.Errorf("get series from %s: %w", table, err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Bucket, &p.AvgValue, &p.MinValue, &p.MaxValue, &p.SampleCount); err != nil {
			return nil, resolution, fmt.Errorf("scan metric point: %w", err)
		}
		points = append(points, p)
	}

	return points, resolution, rows.Err()
}

// GetHistogramBuckets returns bucket distribution for a histogram metric
// at a specific timestamp. Used for latency heatmaps and percentile views.
func (store *Store) GetHistogramBuckets(ctx context.Context, orgID uuid.UUID, metricName string, from, timeout time.Time) ([]HistogramBucket, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT time, org_id, metric_name, labels, le, count
		FROM metric_histogram_buckets
		WHERE org_id = $1
		  AND metric_name = $2
		  AND time >= $3
		  AND time <= $4
		ORDER BY time ASC, le ASC
	`, orgID, metricName, from, timeout)
	if err != nil {
		return nil, fmt.Errorf("get histogram buckets: %w", err)
	}
	defer rows.Close()

	var buckets []HistogramBucket
	for rows.Next() {
		var batch HistogramBucket
		var labelsRaw []byte
		if err := rows.Scan(
			&batch.Time, &batch.OrgID, &batch.MetricName, &labelsRaw, &batch.Le, &batch.Count,
		); err != nil {
			return nil, fmt.Errorf("scan histogram bucket: %w", err)
		}
		_ = json.Unmarshal(labelsRaw, &batch.Labels)
		buckets = append(buckets, batch)
	}

	return buckets, rows.Err()
}

func (store *Store) ListMetricValues(ctx context.Context, query ListMetricValuesQuery) ([]MetricValue, int, error) {
	if query.MetricName == "" {
		return nil, 0, fmt.Errorf("metric_name is required")
	}

	args := []any{query.OrgID, query.MetricName}
	filter := ""
	nextArg := 3

	if query.ServiceName != "" {
		filter += fmt.Sprintf(` AND labels->>'service.name' = $%d`, nextArg)
		args = append(args, query.ServiceName)
		nextArg++
	}

	var total int
	err := store.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM metrics WHERE org_id = $1 AND metric_name = $2`+filter,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count metric values: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, query.Limit, query.Offset)

	rows, err := store.pool.Query(ctx, fmt.Sprintf(`
		SELECT time, metric_type, value, labels
		FROM metrics
		WHERE org_id = $1
		  AND metric_name = $2%s
		ORDER BY time DESC
		LIMIT $%d OFFSET $%d`, filter, limitIdx, offsetIdx), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list metric values: %w", err)
	}
	defer rows.Close()

	var values []MetricValue
	for rows.Next() {
		var value MetricValue
		var labelsRaw []byte
		if err := rows.Scan(&value.Time, &value.MetricType, &value.Value, &labelsRaw); err != nil {
			return nil, 0, fmt.Errorf("scan metric value: %w", err)
		}
		_ = json.Unmarshal(labelsRaw, &value.Labels)
		value.MetricName = query.MetricName
		values = append(values, value)
	}

	return values, total, rows.Err()
}

func (store *Store) GetAggregations(ctx context.Context, query AggregationQuery) (MetricAggregation, error) {
	if query.MetricName == "" {
		return MetricAggregation{}, fmt.Errorf("metric_name is required")
	}

	args := []any{query.OrgID, query.MetricName, query.From, query.To}
	filter := ""
	nextArg := 5

	for key, value := range query.Labels {
		labelJSON, _ := json.Marshal(map[string]string{key: value})
		filter += fmt.Sprintf(` AND labels @> $%d::jsonb`, nextArg)
		args = append(args, string(labelJSON))
		nextArg++
	}

	sqlQuery := fmt.Sprintf(`
		SELECT
			COUNT(*),
			COALESCE(SUM(value), 0),
			COALESCE(AVG(value), 0),
			COALESCE(MIN(value), 0),
			COALESCE(MAX(value), 0),
			COALESCE(percentile_cont(0.5) WITHIN GROUP (ORDER BY value), 0),
			COALESCE(percentile_cont(0.9) WITHIN GROUP (ORDER BY value), 0),
			COALESCE(percentile_cont(0.95) WITHIN GROUP (ORDER BY value), 0),
			COALESCE(percentile_cont(0.99) WITHIN GROUP (ORDER BY value), 0)
		FROM metrics
		WHERE org_id = $1
		  AND metric_name = $2
		  AND time >= $3
		  AND time <= $4%s`, filter)

	var count int64
	var total, avg, min, max, p50, p90, p95, p99 float64

	err := store.pool.QueryRow(ctx, sqlQuery, args...).Scan(
		&count, &total, &avg, &min, &max, &p50, &p90, &p95, &p99,
	)
	if err != nil {
		return MetricAggregation{}, fmt.Errorf("get aggregations: %w", err)
	}

	unit, _ := store.GetMetricUnit(ctx, query.OrgID, query.MetricName)
	unit = StandardizeUnit(unit)
	mType, _ := store.GetMetricType(ctx, query.OrgID, query.MetricName)

	if count == 0 {
		return MetricAggregation{
			MetricName: query.MetricName,
			Unit:       unit,
		}, nil
	}

	return MetricAggregation{
		MetricName: query.MetricName,
		Count:      count,
		Total:      roundMetricValue(total, query.MetricName, mType, unit),
		Avg:        roundMetricValue(avg, query.MetricName, mType, unit),
		Min:        roundMetricValue(min, query.MetricName, mType, unit),
		Max:        roundMetricValue(max, query.MetricName, mType, unit),
		P50:        roundMetricValue(p50, query.MetricName, mType, unit),
		P90:        roundMetricValue(p90, query.MetricName, mType, unit),
		P95:        roundMetricValue(p95, query.MetricName, mType, unit),
		P99:        roundMetricValue(p99, query.MetricName, mType, unit),
		Unit:       unit,
	}, nil
}

func (store *Store) GetMetricUnit(ctx context.Context, orgID uuid.UUID, metricName string) (string, error) {
	var unit string
	err := store.pool.QueryRow(ctx, `
		SELECT unit
		FROM metric_metadata
		WHERE org_id = $1 AND metric_name = $2
	`, orgID, metricName).Scan(&unit)
	if err != nil {
		return "", err
	}
	return unit, nil
}

func (store *Store) GetMetricType(ctx context.Context, orgID uuid.UUID, metricName string) (MetricType, error) {
	var mType string
	err := store.pool.QueryRow(ctx, `
		SELECT metric_type
		FROM metric_metadata
		WHERE org_id = $1 AND metric_name = $2
	`, orgID, metricName).Scan(&mType)
	if err != nil {
		return "", err
	}
	return MetricType(mType), nil
}

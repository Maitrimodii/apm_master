package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
func (s *Store) ListMetricNames(ctx context.Context, query ListMetricsQuery) ([]string, int, error) {
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
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(DISTINCT metric_name) FROM metrics WHERE org_id = $1`+filter,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count metric names: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, query.Limit, query.Offset)

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
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
func (s *Store) GetSeries(ctx context.Context, query SeriesQuery) ([]MetricPoint, Resolution, error) {
	resolution, src := seriesSource(query.Interval, query.From, query.To)
	table, bucket, isRaw := src.table, src.bucket, src.raw

	args := []any{query.OrgID, query.From, query.To, query.MetricName}
	filter := ""
	nextArg := 5

	// Label matchers — each key/value pair appended as a JSONB containment check.
	// Using @> operator with a constructed JSONB object is index-friendly.
	for key, value := range query.Labels {
		labelJSON, err := json.Marshal(map[string]string{key: value})
		if err != nil {
			return nil, resolution, fmt.Errorf("encode label filter: %w", err)
		}
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

	rows, err := s.pool.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, resolution, fmt.Errorf("get series from %s: %w", table, err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Bucket, &p.Avg, &p.Min, &p.Max, &p.Count); err != nil {
			return nil, resolution, fmt.Errorf("scan metric point: %w", err)
		}
		points = append(points, p)
	}

	return points, resolution, rows.Err()
}

// GetHistogramBuckets returns bucket distribution for a histogram metric
// at a specific timestamp. Used for latency heatmaps and percentile views.
func (s *Store) GetHistogramBuckets(ctx context.Context, orgID uuid.UUID, metricName string, from, to time.Time) ([]HistogramBucket, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT time, org_id, metric_name, labels, le, count
		FROM metric_histogram_buckets
		WHERE org_id = $1
		  AND metric_name = $2
		  AND time >= $3
		  AND time <= $4
		ORDER BY time ASC, le ASC
	`, orgID, metricName, from, to)
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
		if err := unmarshalLabels(labelsRaw, &batch.Labels); err != nil {
			return nil, err
		}
		buckets = append(buckets, batch)
	}

	return buckets, rows.Err()
}

func (s *Store) ListMetricValues(ctx context.Context, query ListMetricValuesQuery) ([]MetricValue, int, error) {
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
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM metrics WHERE org_id = $1 AND metric_name = $2`+filter,
		args...,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count metric values: %w", err)
	}

	limitIdx := nextArg
	offsetIdx := nextArg + 1
	args = append(args, query.Limit, query.Offset)

	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
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
		if err := unmarshalLabels(labelsRaw, &value.Labels); err != nil {
			return nil, 0, err
		}
		value.MetricName = query.MetricName
		values = append(values, value)
	}

	return values, total, rows.Err()
}

func (s *Store) GetAggregations(ctx context.Context, query AggregationQuery) (MetricAggregation, error) {
	if query.MetricName == "" {
		return MetricAggregation{}, fmt.Errorf("metric_name is required")
	}

	args := []any{query.OrgID, query.MetricName, query.From, query.To}
	filter := ""
	nextArg := 5

	for key, value := range query.Labels {
		labelJSON, err := json.Marshal(map[string]string{key: value})
		if err != nil {
			return MetricAggregation{}, fmt.Errorf("encode label filter: %w", err)
		}
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

	err := s.pool.QueryRow(ctx, sqlQuery, args...).Scan(
		&count, &total, &avg, &min, &max, &p50, &p90, &p95, &p99,
	)
	if err != nil {
		return MetricAggregation{}, fmt.Errorf("get aggregations: %w", err)
	}

	return MetricAggregation{
		MetricName: query.MetricName,
		Count:      count,
		Total:      total,
		Avg:        avg,
		Min:        min,
		Max:        max,
		P50:        p50,
		P90:        p90,
		P95:        p95,
		P99:        p99,
	}, nil
}

// GetMetadata returns the unit and type a metric was reported with. A metric
// with no metadata row yields zero values and no error.
func (s *Store) GetMetadata(ctx context.Context, orgID uuid.UUID, metricName string) (Metadata, error) {
	var md Metadata
	err := s.pool.QueryRow(ctx, `
		SELECT unit, metric_type
		FROM metric_metadata
		WHERE org_id = $1 AND metric_name = $2
	`, orgID, metricName).Scan(&md.Unit, &md.Type)
	if errors.Is(err, pgx.ErrNoRows) {
		return Metadata{}, nil
	}
	if err != nil {
		return Metadata{}, fmt.Errorf("get metric metadata: %w", err)
	}
	md.Unit = StandardizeUnit(md.Unit)
	return md, nil
}

func unmarshalLabels(raw []byte, dst *map[string]any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("decode labels: %w", err)
	}
	return nil
}

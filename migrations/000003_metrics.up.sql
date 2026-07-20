-- migrations/000003_metrics.up.sql

CREATE TABLE metrics (
    time         TIMESTAMPTZ NOT NULL,
    org_id       UUID        NOT NULL,

    -- identity
    metric_name  TEXT        NOT NULL,
    metric_type  TEXT        NOT NULL,       -- gauge, counter, histogram, summary

    -- the value (histogram stores sum; use metric_histogram for buckets)
    value        DOUBLE PRECISION NOT NULL,

    -- high-cardinality labels stored as JSONB
    -- e.g. {"host":"web-1","env":"prod","region":"us-east-1"}
    labels       JSONB       NOT NULL DEFAULT '{}'
);

SELECT create_hypertable('metrics', 'time',
    chunk_time_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

CREATE INDEX ON metrics (org_id, metric_name, time DESC);
CREATE INDEX ON metrics USING GIN (labels);

ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'org_id, metric_name',
    timescaledb.compress_orderby   = 'time DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');
SELECT add_retention_policy('metrics', INTERVAL '30 days');

-- histogram buckets stored separately (avoids wide pivots)
CREATE TABLE metric_histogram_buckets (
    time         TIMESTAMPTZ NOT NULL,
    org_id       UUID        NOT NULL,
    metric_name  TEXT        NOT NULL,
    labels       JSONB       NOT NULL DEFAULT '{}',
    le           DOUBLE PRECISION NOT NULL,   -- upper bound of bucket
    count        BIGINT      NOT NULL
);

SELECT create_hypertable('metric_histogram_buckets', 'time',
    chunk_time_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

CREATE INDEX ON metric_histogram_buckets (org_id, metric_name, time DESC);

ALTER TABLE metric_histogram_buckets SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'org_id, metric_name',
    timescaledb.compress_orderby   = 'time DESC'
);
SELECT add_compression_policy('metric_histogram_buckets', INTERVAL '1 hour');
SELECT add_retention_policy('metric_histogram_buckets', INTERVAL '30 days');

-- ── Continuous aggregates ──────────────────────────────────────────────────────
-- 1-minute rollup
CREATE MATERIALIZED VIEW metrics_1m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 minute', time) AS bucket,
    org_id,
    metric_name,
    labels,
    AVG(value)   AS avg_value,
    MIN(value)   AS min_value,
    MAX(value)   AS max_value,
    COUNT(*)     AS sample_count
FROM metrics
GROUP BY bucket, org_id, metric_name, labels
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics_1m',
    start_offset => INTERVAL '5 minutes',
    end_offset   => INTERVAL '1 minute',
    schedule_interval => INTERVAL '1 minute'
);
SELECT add_retention_policy('metrics_1m', INTERVAL '7 days');

-- 5-minute rollup (rolls up from raw, not from 1m — avoids double-counting)
CREATE MATERIALIZED VIEW metrics_5m
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('5 minutes', time) AS bucket,
    org_id,
    metric_name,
    labels,
    AVG(value)   AS avg_value,
    MIN(value)   AS min_value,
    MAX(value)   AS max_value,
    COUNT(*)     AS sample_count
FROM metrics
GROUP BY bucket, org_id, metric_name, labels
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics_5m',
    start_offset => INTERVAL '30 minutes',
    end_offset   => INTERVAL '5 minutes',
    schedule_interval => INTERVAL '5 minutes'
);
SELECT add_retention_policy('metrics_5m', INTERVAL '30 days');

-- 1-hour rollup
CREATE MATERIALIZED VIEW metrics_1h
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', time) AS bucket,
    org_id,
    metric_name,
    labels,
    AVG(value)   AS avg_value,
    MIN(value)   AS min_value,
    MAX(value)   AS max_value,
    COUNT(*)     AS sample_count
FROM metrics
GROUP BY bucket, org_id, metric_name, labels
WITH NO DATA;

SELECT add_continuous_aggregate_policy('metrics_1h',
    start_offset => INTERVAL '3 hours',
    end_offset   => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour'
);
SELECT add_retention_policy('metrics_1h', INTERVAL '1 year');
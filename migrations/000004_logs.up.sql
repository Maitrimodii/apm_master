CREATE TABLE logs (
    time         TIMESTAMPTZ NOT NULL,
    org_id       UUID        NOT NULL,

    -- source
    service_name TEXT        NOT NULL,
    trace_id     TEXT,                        -- optional correlation to a span
    span_id      TEXT,

    -- severity follows OTel log severity numbers
    severity_num  SMALLINT   NOT NULL DEFAULT 9,   -- 9 = INFO
    severity_text TEXT       NOT NULL DEFAULT 'INFO',

    -- body
    body         TEXT        NOT NULL,

    -- attributes
    resource_attrs JSONB     NOT NULL DEFAULT '{}',
    log_attrs      JSONB     NOT NULL DEFAULT '{}',

    -- full-text search
    body_tsv     TSVECTOR    GENERATED ALWAYS AS (to_tsvector('english', body)) STORED
);

SELECT create_hypertable('logs', 'time',
    chunk_time_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

CREATE INDEX ON logs (org_id, service_name, time DESC);
CREATE INDEX ON logs (org_id, severity_num, time DESC);
CREATE INDEX ON logs (org_id, trace_id)  WHERE trace_id IS NOT NULL;

-- full-text search index
CREATE INDEX ON logs USING GIN (body_tsv);

-- attribute filtering
CREATE INDEX ON logs USING GIN (log_attrs);

ALTER TABLE logs SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'org_id, service_name',
    timescaledb.compress_orderby   = 'time DESC'
);
SELECT add_compression_policy('logs', INTERVAL '1 hour');
SELECT add_retention_policy('logs', INTERVAL '14 days');
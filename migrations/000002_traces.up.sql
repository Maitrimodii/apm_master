CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE spans (
    span_id     TEXT        NOT NULL,
    trace_id    TEXT        NOT NULL,
    parent_id   TEXT,

    org_id      UUID        NOT NULL,

    service_name TEXT       NOT NULL,
    span_name    TEXT       NOT NULL,
    span_kind    TEXT       NOT NULL,

    start_time  TIMESTAMPTZ NOT NULL,
    end_time    TIMESTAMPTZ NOT NULL,
    duration_ns BIGINT      NOT NULL,  

    status_code TEXT        NOT NULL DEFAULT 'UNSET',
    status_msg  TEXT,

    resource_attrs JSONB    NOT NULL DEFAULT '{}',
    span_attrs     JSONB    NOT NULL DEFAULT '{}',
    events         JSONB    NOT NULL DEFAULT '[]',
    links          JSONB    NOT NULL DEFAULT '[]',

    PRIMARY KEY (span_id, start_time)
);

SELECT create_hypertable('spans', 'start_time',
    chunk_time_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

CREATE INDEX ON spans (org_id, trace_id);
CREATE INDEX ON spans (org_id, service_name, start_time DESC);
CREATE INDEX ON spans (org_id, status_code, start_time DESC) WHERE status_code = 'ERROR';
CREATE INDEX ON spans USING GIN (span_attrs);
CREATE INDEX ON spans USING GIN (resource_attrs);

ALTER TABLE spans SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'org_id, service_name',
    timescaledb.compress_orderby   = 'start_time DESC'
);

SELECT add_compression_policy('spans', INTERVAL '1 hour');
SELECT add_retention_policy('spans', INTERVAL '7 days');

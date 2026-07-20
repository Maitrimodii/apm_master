-- migrations/000007_metrics_metadata.up.sql

CREATE TABLE metric_metadata (
    org_id       UUID        NOT NULL,
    metric_name  TEXT        NOT NULL,
    metric_type  TEXT        NOT NULL,
    unit         TEXT        NOT NULL,
    PRIMARY KEY (org_id, metric_name)
);

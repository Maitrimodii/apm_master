CREATE TABLE investigations (
    id                      UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id                  UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    incident_id             UUID NOT NULL REFERENCES alert_instances(id) ON DELETE CASCADE,
    requested_by            UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status                  TEXT NOT NULL DEFAULT 'queued',
    time_range_start        TIMESTAMPTZ NOT NULL,
    time_range_end          TIMESTAMPTZ NOT NULL,
    focus                   TEXT[] NOT NULL DEFAULT '{}',
    correlation_id          UUID NOT NULL,
    idempotency_key         TEXT,
    parent_investigation_id UUID REFERENCES investigations(id) ON DELETE SET NULL,
    cancel_requested_at     TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at              TIMESTAMPTZ,
    completed_at            TIMESTAMPTZ,
    failure_code            TEXT,
    failure_message         TEXT,
    CONSTRAINT investigations_idempotency UNIQUE (org_id, idempotency_key)
);

CREATE INDEX idx_investigations_org_incident ON investigations(org_id, incident_id);
CREATE INDEX idx_investigations_status       ON investigations(status) WHERE status NOT IN ('completed','failed','cancelled','timed_out','inconclusive');
CREATE INDEX idx_investigations_org_created  ON investigations(org_id, created_at DESC);

CREATE TABLE investigation_outbox (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id UUID NOT NULL REFERENCES investigations(id) ON DELETE CASCADE,
    correlation_id   UUID NOT NULL,
    payload          JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at     TIMESTAMPTZ
);

CREATE INDEX idx_investigation_outbox_unpublished ON investigation_outbox(created_at) WHERE published_at IS NULL;

CREATE TABLE investigation_evidence (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id      UUID NOT NULL REFERENCES investigations(id) ON DELETE CASCADE,
    evidence_type         TEXT NOT NULL,
    source_service        TEXT NOT NULL DEFAULT '',
    query_parameters      JSONB NOT NULL DEFAULT '{}',
    summary               TEXT NOT NULL DEFAULT '',
    result                JSONB NOT NULL DEFAULT '{}',
    provenance            JSONB NOT NULL DEFAULT '{}',
    data_quality_warnings TEXT[] NOT NULL DEFAULT '{}',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_investigation_evidence_inv ON investigation_evidence(investigation_id);

CREATE TABLE investigation_reports (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id  UUID NOT NULL REFERENCES investigations(id) ON DELETE CASCADE UNIQUE,
    schema_version    TEXT NOT NULL DEFAULT '1.0',
    report_json       JSONB NOT NULL,
    validation_status TEXT NOT NULL DEFAULT 'valid',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE investigation_events (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id UUID NOT NULL REFERENCES investigations(id) ON DELETE CASCADE,
    sequence_number  INT NOT NULL,
    event_type       TEXT NOT NULL,
    summary          TEXT NOT NULL,
    payload          JSONB NOT NULL DEFAULT '{}',
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_investigation_events_inv_seq ON investigation_events(investigation_id, sequence_number ASC);

-- Seed investigation permissions and grant to admin role
INSERT INTO permissions (name, action, description)
VALUES 
    ('investigations', 'read', 'View incident investigations, reports, and evidence'),
    ('investigations', 'write', 'Create, retry, and cancel incident investigations')
ON CONFLICT (name, action) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
CROSS JOIN permissions p
WHERE r.name = 'admin' AND p.name = 'investigations'
ON CONFLICT DO NOTHING;

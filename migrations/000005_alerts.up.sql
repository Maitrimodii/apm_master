CREATE TABLE alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    created_by UUID NOT NULL
        REFERENCES users(id),

    name TEXT NOT NULL,
    description TEXT,
    severity TEXT NOT NULL,
    condition TEXT NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    notification_roles UUID[] NOT NULL DEFAULT '{}',

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE(org_id, name)
);

CREATE TABLE alert_instances (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    state TEXT NOT NULL DEFAULT 'FIRING',
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    acknowledged_at TIMESTAMPTZ,
    last_evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    value DOUBLE PRECISION NOT NULL DEFAULT 0,
    threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    notification_sent BOOLEAN NOT NULL DEFAULT FALSE,
    service_name TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE alert_evaluation_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    alert_id UUID NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    evaluator TEXT NOT NULL,
    state TEXT NOT NULL,
    value DOUBLE PRECISION NOT NULL DEFAULT 0,
    threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    service_name TEXT
);

CREATE INDEX idx_alerts_org_id ON alerts(org_id);

CREATE INDEX idx_alert_instances_alert_id ON alert_instances(alert_id);
CREATE INDEX idx_alert_instances_state ON alert_instances(state);
CREATE INDEX idx_alert_instances_triggered_at ON alert_instances(triggered_at DESC);
CREATE INDEX idx_alert_instances_resolved_at ON alert_instances(resolved_at DESC);
CREATE INDEX idx_alert_instances_service_name ON alert_instances(service_name);
CREATE INDEX idx_alert_instances_org_alert ON alert_instances(alert_id, state);

CREATE INDEX idx_alert_evaluation_history_alert_id ON alert_evaluation_history(alert_id);
CREATE INDEX idx_alert_evaluation_history_evaluated_at ON alert_evaluation_history(evaluated_at DESC);
CREATE INDEX idx_alert_evaluation_history_org_id ON alert_evaluation_history(org_id);
CREATE INDEX idx_alert_evaluation_history_service_name ON alert_evaluation_history(service_name);

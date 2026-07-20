CREATE TYPE notification_channel AS ENUM (
    'email',
    'in_app'
);

CREATE TYPE notification_status AS ENUM (
    'pending',
    'processing',
    'sent',
    'failed'
);

CREATE TABLE notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    org_id UUID NOT NULL
        REFERENCES orgs(id) ON DELETE CASCADE,

    user_id UUID
        REFERENCES users(id) ON DELETE SET NULL,

    event_type TEXT NOT NULL,
    channel notification_channel NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    status notification_status NOT NULL DEFAULT 'pending',
    error TEXT,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ
);

CREATE TABLE notification_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    alert_id UUID REFERENCES alerts(id) ON DELETE CASCADE,
    instance_id UUID REFERENCES alert_instances(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL,
    channel TEXT NOT NULL,
    recipient TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE notification_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    event_type TEXT NOT NULL,
    provider TEXT NOT NULL,
    recipient TEXT NOT NULL,
    subject TEXT NOT NULL,
    body TEXT NOT NULL,
    reference_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    
    state TEXT NOT NULL DEFAULT 'pending', -- pending, processing, sent, failed, dead_letter
    priority INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    run_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT,
    
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_notification_events_org_id ON notification_events(org_id);
CREATE INDEX idx_notification_events_user_id ON notification_events(user_id);
CREATE INDEX idx_notification_events_status ON notification_events(status);
CREATE INDEX idx_notification_events_pending ON notification_events(channel, status, next_attempt_at);

CREATE INDEX idx_notification_deliveries_alert_id ON notification_deliveries(alert_id);
CREATE INDEX idx_notification_deliveries_status ON notification_deliveries(status);
CREATE INDEX idx_notification_deliveries_next_attempt_at ON notification_deliveries(next_attempt_at);
CREATE INDEX idx_notification_deliveries_org_id ON notification_deliveries(org_id);

CREATE INDEX idx_notification_jobs_poll ON notification_jobs(state, run_at, priority DESC);
CREATE INDEX idx_notification_jobs_reference ON notification_jobs(reference_id, recipient, event_type);

ALTER TABLE alert_instances ADD COLUMN pending_fired_at TIMESTAMPTZ;
UPDATE alert_instances SET pending_fired_at = triggered_at WHERE state = 'FIRING';

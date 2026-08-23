SELECT add_retention_policy('spans', INTERVAL '30 days');
SELECT add_retention_policy('metrics', INTERVAL '30 days');
SELECT add_retention_policy('logs', INTERVAL '30 days');

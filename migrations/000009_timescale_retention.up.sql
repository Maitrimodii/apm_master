SELECT remove_retention_policy('spans', if_exists => true);
SELECT add_retention_policy('spans', INTERVAL '30 days');

SELECT remove_retention_policy('metrics', if_exists => true);
SELECT add_retention_policy('metrics', INTERVAL '30 days');

SELECT remove_retention_policy('logs', if_exists => true);
SELECT add_retention_policy('logs', INTERVAL '30 days');

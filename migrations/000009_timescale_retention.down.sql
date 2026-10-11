SELECT remove_retention_policy('spans', if_exists => true);
SELECT remove_retention_policy('metrics', if_exists => true);
SELECT remove_retention_policy('logs', if_exists => true);

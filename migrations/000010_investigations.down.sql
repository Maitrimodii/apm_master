DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE name = 'investigations'
);

DELETE FROM permissions WHERE name = 'investigations';

DROP TABLE IF EXISTS investigation_events;
DROP TABLE IF EXISTS investigation_reports;
DROP TABLE IF EXISTS investigation_evidence;
DROP TABLE IF EXISTS investigation_outbox;
DROP TABLE IF EXISTS investigations;

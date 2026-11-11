DELETE FROM role_permissions
WHERE permission_id IN (SELECT id FROM permissions WHERE name = 'invites' AND action = 'delete');

DELETE FROM permissions WHERE name = 'invites' AND action = 'delete';

ALTER TABLE invites DROP COLUMN IF EXISTS revoked_at;

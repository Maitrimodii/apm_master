-- Permission resource names become plural to match the API routes (internal/authz).
UPDATE permissions SET name = 'invites'     WHERE name = 'invite';
UPDATE permissions SET name = 'permissions' WHERE name = 'permission';
UPDATE permissions SET name = 'roles'       WHERE name = 'role';
UPDATE permissions SET name = 'api_keys'    WHERE name = 'api_key';

-- Invites can be cancelled. A cancelled invite keeps its row (for the
-- invite list and audit) but its link no longer works.
ALTER TABLE invites ADD COLUMN revoked_at TIMESTAMPTZ;

-- Cancelling invites is its own permission. New organizations get it from the
-- catalogue when their admin role is seeded; existing admin roles get it here.
INSERT INTO permissions (name, action, description)
VALUES ('invites', 'delete', 'Cancel pending invites')
ON CONFLICT (name, action) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM roles r
JOIN permissions p ON p.name = 'invites' AND p.action = 'delete'
WHERE r.name = 'admin' AND r.is_system
ON CONFLICT DO NOTHING;

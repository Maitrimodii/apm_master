-- Revoked duplicates, closed expired invites and lowercased emails are not
-- restored.
DROP INDEX IF EXISTS invites_one_open_per_email;
ALTER TABLE invites DROP COLUMN IF EXISTS last_sent_at;

DROP INDEX IF EXISTS idx_refresh_tokens_family_active;

ALTER TABLE refresh_tokens
    DROP COLUMN IF EXISTS session_expires_at,
    DROP COLUMN IF EXISTS family_id;

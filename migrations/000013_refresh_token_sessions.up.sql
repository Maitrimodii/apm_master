-- Group refresh tokens into sessions ("families") so a session can be
-- revoked as a whole when a rotated token is replayed, and give every session
-- an absolute end that rotation cannot extend.
--
-- family_id has a default and session_expires_at is nullable so rows inserted
-- by an older binary during a rolling deploy still succeed. The application
-- treats a NULL session_expires_at as "ends at this token's expires_at".
ALTER TABLE refresh_tokens
    ADD COLUMN family_id UUID NOT NULL DEFAULT gen_random_uuid(),
    ADD COLUMN session_expires_at TIMESTAMPTZ;

-- Existing tokens become one-token sessions that end when the token does, so
-- no pre-existing session can be extended past its current expiry.
UPDATE refresh_tokens
SET family_id = id,
    session_expires_at = expires_at;

CREATE INDEX idx_refresh_tokens_family_active
ON refresh_tokens(family_id)
WHERE revoked_at IS NULL;

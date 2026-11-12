-- Emails carrying one-time links (invites, password resets) are stored only
-- encrypted. The application seals their body and metadata with AES-256-GCM
-- into payload_ciphertext; the plaintext columns hold placeholders.
--
-- IRREVERSIBLE: historical plaintext bodies and links of these emails are
-- redacted below and cannot be recovered by the down migration.

ALTER TABLE notification_jobs
    ADD COLUMN payload_ciphertext BYTEA,
    ADD COLUMN payload_key_id TEXT;

-- Redact historical copies of links.
UPDATE notification_jobs
SET body = '[encrypted]', metadata = metadata - 'invite_link' - 'reset_link'
WHERE event_type IN ('invite_created', 'password_reset_requested');

-- Queued legacy jobs can no longer be sent: there is no plaintext fallback.
-- Their users request a new link.
UPDATE notification_jobs
SET state = 'dead_letter',
    last_error = 'plaintext payload removed by migration 000015; request a new link',
    updated_at = now()
WHERE event_type IN ('invite_created', 'password_reset_requested')
  AND state IN ('pending', 'processing');

UPDATE notification_jobs
SET metadata = metadata - 'invite_link' - 'reset_link'
WHERE metadata ?| array['invite_link', 'reset_link'];

UPDATE notification_events
SET body = '[redacted]'
WHERE event_type IN ('invite_created', 'password_reset_requested');

UPDATE notification_events
SET metadata = metadata - 'invite_link' - 'reset_link'
WHERE metadata ?| array['invite_link', 'reset_link'];

-- Enforce it in the database too, so no code path (or outdated binary) can
-- store a link in plaintext.
ALTER TABLE notification_jobs
    ADD CONSTRAINT notification_jobs_no_plaintext_links CHECK (
        NOT (metadata ?| array['invite_link', 'reset_link'])
        AND (event_type NOT IN ('invite_created', 'password_reset_requested') OR body = '[encrypted]')
    ),
    ADD CONSTRAINT notification_jobs_sensitive_encrypted CHECK (
        event_type NOT IN ('invite_created', 'password_reset_requested')
        OR state NOT IN ('pending', 'processing')
        OR (payload_ciphertext IS NOT NULL AND payload_key_id IS NOT NULL)
    );

ALTER TABLE notification_events
    ADD CONSTRAINT notification_events_no_plaintext_links CHECK (
        NOT (metadata ?| array['invite_link', 'reset_link'])
        AND (event_type NOT IN ('invite_created', 'password_reset_requested') OR body = '[redacted]')
    );

-- Finds dead-lettered payloads whose recovery window has passed.
CREATE INDEX idx_notification_jobs_dead_letter_payload
ON notification_jobs (updated_at)
WHERE state = 'dead_letter' AND payload_ciphertext IS NOT NULL;

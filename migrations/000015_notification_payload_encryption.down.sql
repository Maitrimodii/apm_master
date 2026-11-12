-- Removes encryption support. Redacted historical bodies and links are not
-- restored, and remaining ciphertext is dropped with its column.
DROP INDEX IF EXISTS idx_notification_jobs_dead_letter_payload;

ALTER TABLE notification_events DROP CONSTRAINT IF EXISTS notification_events_no_plaintext_links;

ALTER TABLE notification_jobs
    DROP CONSTRAINT IF EXISTS notification_jobs_sensitive_encrypted,
    DROP CONSTRAINT IF EXISTS notification_jobs_no_plaintext_links,
    DROP COLUMN IF EXISTS payload_key_id,
    DROP COLUMN IF EXISTS payload_ciphertext;

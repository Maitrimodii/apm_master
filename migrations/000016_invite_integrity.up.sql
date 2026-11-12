-- At most one open invite per organization and email (compared trimmed and
-- lowercased), enforced by the database so concurrent requests cannot both
-- insert one, plus the timestamp the resend cooldown is measured from.
--
-- Data changes (not undone by the down migration):
--   * open invites that have already expired are closed (revoked_at = now());
--     closed after expiry, they still display as expired;
--   * of several open, unexpired invites for one org/email, the newest is kept
--     and the older ones are revoked (their links stop working);
--   * stored invite emails are lowercased and trimmed.

ALTER TABLE invites ADD COLUMN last_sent_at TIMESTAMPTZ NOT NULL DEFAULT now();
UPDATE invites SET last_sent_at = created_at;

UPDATE invites
SET revoked_at = now()
WHERE accepted_at IS NULL AND revoked_at IS NULL AND expires_at <= now();

WITH ranked AS (
    SELECT id,
           row_number() OVER (
               PARTITION BY org_id, lower(btrim(email))
               ORDER BY created_at DESC, id DESC
           ) AS position
    FROM invites
    WHERE accepted_at IS NULL AND revoked_at IS NULL
)
UPDATE invites
SET revoked_at = now()
FROM ranked
WHERE invites.id = ranked.id AND ranked.position > 1;

UPDATE invites SET email = lower(btrim(email)) WHERE email <> lower(btrim(email));

CREATE UNIQUE INDEX invites_one_open_per_email
ON invites (org_id, lower(email))
WHERE accepted_at IS NULL AND revoked_at IS NULL;

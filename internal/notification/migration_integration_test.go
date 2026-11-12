package notification

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Migration 000015 against legacy plaintext rows, inside a transaction that
// is rolled back: links are redacted everywhere, queued legacy sensitive jobs
// are dead-lettered (there is no plaintext fallback), unrelated jobs are left
// alone, and the CHECK constraints are in place.
func TestMigration000015RedactsLegacyPlaintext(t *testing.T) {
	pool := testPool(t)
	orgID, userID := seedOrg(t, pool)
	ctx := context.Background()
	read := func(name string) string {
		sql, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		return string(sql)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, read("000015_notification_payload_encryption.down.sql")); err != nil {
		t.Fatalf("down: %v", err)
	}

	link := "https://ui/reset-password?token=" + secretToken
	meta := `{"first_name":"Ada","reset_link":"` + link + `"}`
	for _, row := range []struct{ eventType, state string }{
		{"password_reset_requested", "pending"},
		{"password_reset_requested", "sent"},
		{"invite_created", "dead_letter"},
	} {
		if _, err := tx.Exec(ctx, `INSERT INTO notification_jobs (org_id, user_id, event_type, provider, recipient, subject, body, metadata, state)
			VALUES ($1, $2, $3, 'email', 'ada@example.com', 's', $4, $5, $6)`,
			orgID, userID, row.eventType, "Link: "+link, meta, row.state); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notification_jobs (org_id, event_type, provider, recipient, subject, body, state)
		VALUES ($1, 'alert_firing', 'email', 'oncall@example.com', 'Alert', 'CPU high', 'pending')`, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO notification_events (org_id, event_type, channel, recipient, subject, body, metadata, status)
		VALUES ($1, 'password_reset_requested', 'email', 'ada@example.com', 's', $2, $3, 'sent')`, orgID, "Link: "+link, meta); err != nil {
		t.Fatal(err)
	}

	if _, err := tx.Exec(ctx, read("000015_notification_payload_encryption.up.sql")); err != nil {
		t.Fatalf("up: %v", err)
	}

	var dump string
	if err := tx.QueryRow(ctx, `SELECT string_agg(r::text, E'\n') FROM (
		SELECT to_jsonb(j) AS r FROM notification_jobs j UNION ALL SELECT to_jsonb(e) FROM notification_events e) rows`).Scan(&dump); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dump, secretToken) || strings.Contains(dump, "reset_link") {
		t.Fatalf("legacy plaintext survived the migration:\n%s", dump)
	}
	var queuedSensitive, deadLettered int
	var alertState, alertBody string
	if err := tx.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE event_type = 'password_reset_requested' AND state IN ('pending', 'processing')),
		count(*) FILTER (WHERE event_type <> 'alert_firing' AND state = 'dead_letter')
		FROM notification_jobs`).Scan(&queuedSensitive, &deadLettered); err != nil {
		t.Fatal(err)
	}
	if queuedSensitive != 0 || deadLettered != 2 {
		t.Errorf("queued sensitive = %d, dead-lettered = %d; want 0 and 2", queuedSensitive, deadLettered)
	}
	if err := tx.QueryRow(ctx, `SELECT state, body FROM notification_jobs WHERE event_type = 'alert_firing'`).Scan(&alertState, &alertBody); err != nil ||
		alertState != "pending" || alertBody != "CPU high" {
		t.Errorf("unrelated job changed: %s %q %v", alertState, alertBody, err)
	}
	var constraints int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname IN
		('notification_jobs_no_plaintext_links', 'notification_jobs_sensitive_encrypted', 'notification_events_no_plaintext_links')`).Scan(&constraints); err != nil || constraints != 3 {
		t.Errorf("%d of 3 constraints present: %v", constraints, err)
	}
}

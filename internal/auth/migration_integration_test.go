package auth

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func readMigration(t *testing.T, name string) string {
	t.Helper()
	sql, err := os.ReadFile("../../migrations/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(sql)
}

// Migration 000016 against seeded duplicates, inside a transaction that is
// rolled back: the newest valid invite per org/email stays open, older open
// duplicates are revoked, expired ones are closed but still read as expired,
// and stored emails are normalised.
func TestMigration000016ResolvesDuplicateInvites(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "mig16")
	other := f.createAdmin(t, "mig16-other")
	ctx := context.Background()

	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, readMigration(t, "000016_invite_integrity.down.sql")); err != nil {
		t.Fatalf("down: %v", err)
	}
	insert := func(email string, age, expiresIn time.Duration, accepted bool, org uuid.UUID, inviter uuid.UUID) uuid.UUID {
		t.Helper()
		id := uuid.New()
		var acceptedAt *time.Time
		if accepted {
			now := time.Now()
			acceptedAt = &now
		}
		if _, err := tx.Exec(ctx, `INSERT INTO invites (id, org_id, invited_by, first_name, last_name, email, token_hash, expires_at, accepted_at, created_at)
			VALUES ($1, $2, $3, 'P', 'Q', $4, $5, now() + $6::interval, $7, now() - $8::interval)`,
			id, org, inviter, email, uuid.NewString(), expiresIn.String(), acceptedAt, age.String()); err != nil {
			t.Fatal(err)
		}
		return id
	}
	older := insert("Pat@Example.com", 3*time.Hour, 24*time.Hour, false, admin.orgID, admin.userID)
	newest := insert(" pat@example.com", time.Hour, 24*time.Hour, false, admin.orgID, admin.userID)
	expired := insert("PAT@example.com", 48*time.Hour, -time.Hour, false, admin.orgID, admin.userID)
	accepted := insert("pat@example.com", 72*time.Hour, 24*time.Hour, true, admin.orgID, admin.userID)
	otherOrg := insert("Pat@Example.com", 2*time.Hour, 24*time.Hour, false, other.orgID, other.userID)

	if _, err := tx.Exec(ctx, readMigration(t, "000016_invite_integrity.up.sql")); err != nil {
		t.Fatalf("up: %v", err)
	}

	status := func(id uuid.UUID) (InviteStatus, string) {
		t.Helper()
		inv, err := scanInvite(tx.QueryRow(ctx, `SELECT `+inviteColumns+` FROM invites WHERE id = $1`, id))
		if err != nil || inv == nil {
			t.Fatalf("invite %s: %v", id, err)
		}
		return inv.Status(time.Now()), inv.Email
	}
	for name, c := range map[string]struct {
		id   uuid.UUID
		want InviteStatus
	}{
		"newest stays open":             {newest, InviteStatusPending},
		"older duplicate revoked":       {older, InviteStatusRevoked},
		"expired closed, shown expired": {expired, InviteStatusExpired},
		"accepted untouched":            {accepted, InviteStatusAccepted},
		"other org untouched":           {otherOrg, InviteStatusPending},
	} {
		got, email := status(c.id)
		if got != c.want {
			t.Errorf("%s: status %q, want %q", name, got, c.want)
		}
		if email != "pat@example.com" {
			t.Errorf("%s: email %q not normalised", name, email)
		}
	}

	var sentMatchesCreated bool
	if err := tx.QueryRow(ctx, `SELECT bool_and(last_sent_at = created_at) FROM invites`).Scan(&sentMatchesCreated); err != nil || !sentMatchesCreated {
		t.Errorf("last_sent_at not backfilled from created_at: %v", err)
	}
	// The index now rejects a second open invite for the same org and email.
	_, err = tx.Exec(ctx, `SAVEPOINT dup`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO invites (org_id, invited_by, first_name, last_name, email, token_hash, expires_at)
		VALUES ($1, $2, 'D', 'U', 'PAT@EXAMPLE.COM', 'h-dup', now() + interval '1 day')`, admin.orgID, admin.userID)
	if err == nil {
		t.Error("index allowed a second open invite for the same email")
	}
	_, _ = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT dup`)
}

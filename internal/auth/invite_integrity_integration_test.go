package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/secretbox"
)

// withNotifications wires a real notification queue into the invite service.
// With keys nil the queue refuses sensitive emails, which simulates a failure
// to queue the invite email.
func (f *authFixture) withNotifications(t *testing.T, keys *secretbox.Keyring) {
	t.Helper()
	f.invites.notification = notification.NewService(notification.NewStore(f.pool, keys), nil, nil)
}

func newKeyring(t *testing.T) *secretbox.Keyring {
	t.Helper()
	key := make([]byte, secretbox.KeySize)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	ring, err := secretbox.ParseKeyring("t:"+base64.StdEncoding.EncodeToString(key), "t")
	if err != nil {
		t.Fatal(err)
	}
	return ring
}

func (f *authFixture) countOpenInvites(t *testing.T, orgID uuid.UUID, email string) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM invites
		WHERE org_id = $1 AND lower(email) = lower($2) AND accepted_at IS NULL AND revoked_at IS NULL`, orgID, email).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *authFixture) inviteStatus(t *testing.T, orgID, inviteID uuid.UUID) InviteStatus {
	t.Helper()
	list, _, err := f.invites.ListInvites(context.Background(), orgID, "", 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, inv := range list {
		if inv.ID == inviteID {
			return inv.Status
		}
	}
	t.Fatalf("invite %s not listed", inviteID)
	return ""
}

// --- duplicate prevention and email normalisation ---------------------------

func TestConcurrentInvitesForOneEmailCreateExactlyOne(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "dup-race")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")
	variants := []string{"Dup@Example.com", " dup@example.com", "DUP@EXAMPLE.COM ", "dup@Example.Com"}

	const n = 8
	errs := runConcurrently(n, func(i int) error {
		_, err := f.invite(t, admin, variants[i%len(variants)], role)
		return err
	})
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		requireAppErr(t, err, apperr.CodeInviteAlreadyExists)
	}
	if successes != 1 {
		t.Fatalf("%d invites created for one email, want 1 (errors: %v)", successes, errs)
	}
	var stored string
	if err := f.pool.QueryRow(context.Background(), `SELECT email FROM invites WHERE org_id = $1`, admin.orgID).Scan(&stored); err != nil || stored != "dup@example.com" {
		t.Fatalf("stored email = %q, %v; want normalised dup@example.com", stored, err)
	}
}

func TestInviteRejectsCaseVariantsOfExistingMembers(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "member-case")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")
	member := f.createMember(t, admin, "Mixed.Case")
	f.exec(t, `UPDATE users SET email = 'Mixed.Case@Example.com' WHERE id = $1`, member.userID)

	_, err := f.invite(t, admin, "mixed.case@example.com", role)
	requireAppErr(t, err, apperr.CodeEmailExists)

	// A member who registered in a different case after the invite was sent
	// is caught at acceptance too.
	token := f.useInviteToken("late-member")
	if _, err := f.invite(t, admin, "late@example.com", role); err != nil {
		t.Fatal(err)
	}
	other := f.createMember(t, admin, "late-x")
	f.exec(t, `UPDATE users SET email = 'LATE@Example.com' WHERE id = $1`, other.userID)
	_, err = f.accept(token, "late-password")
	requireAppErr(t, err, apperr.CodeEmailExists)
}

func TestReinviteAfterExpiryAndRevocation(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "reinvite")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")
	ctx := context.Background()

	expired, err := f.invite(t, admin, "again@example.com", role)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE invites SET expires_at = now() - interval '1 hour' WHERE id = $1`, expired.InviteID)
	second, err := f.invite(t, admin, "Again@Example.com", role)
	if err != nil {
		t.Fatalf("re-invite after expiry: %v", err)
	}
	if got := f.inviteStatus(t, admin.orgID, expired.InviteID); got != InviteStatusExpired {
		t.Errorf("superseded expired invite shows %q, want expired", got)
	}
	_, err = f.invite(t, admin, "again@example.com", role)
	requireAppErr(t, err, apperr.CodeInviteAlreadyExists)

	if err := f.invites.CancelInvite(ctx, admin.orgID, admin.userID, second.InviteID); err != nil {
		t.Fatal(err)
	}
	third, err := f.invite(t, admin, "again@example.com", role)
	if err != nil {
		t.Fatalf("re-invite after revocation: %v", err)
	}
	if got := f.inviteStatus(t, admin.orgID, second.InviteID); got != InviteStatusRevoked {
		t.Errorf("cancelled invite shows %q, want revoked", got)
	}
	if got := f.inviteStatus(t, admin.orgID, third.InviteID); got != InviteStatusPending {
		t.Errorf("new invite shows %q, want pending", got)
	}
	if got := f.countOpenInvites(t, admin.orgID, "again@example.com"); got != 1 {
		t.Errorf("%d open invites, want 1", got)
	}
}

// --- resend cooldown -------------------------------------------------------

func (f *authFixture) inviteTokenHash(t *testing.T, inviteID uuid.UUID) (hash string, lastSent string) {
	t.Helper()
	if err := f.pool.QueryRow(context.Background(),
		`SELECT token_hash, last_sent_at::text FROM invites WHERE id = $1`, inviteID).Scan(&hash, &lastSent); err != nil {
		t.Fatal(err)
	}
	return hash, lastSent
}

func TestResendCooldownRejectsImmediateRepeatAndKeepsLink(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.InviteResendCooldownSecs = 60
	admin := f.createAdmin(t, "cooldown")
	token := f.useInviteToken("first-link")
	created, err := f.invite(t, admin, "cool@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}
	hashBefore, sentBefore := f.inviteTokenHash(t, created.InviteID)

	f.useInviteToken("second-link")
	_, err = f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID)
	requireAppErr(t, err, apperr.CodeRateLimit)
	var appErr *apperr.Error
	errors.As(err, &appErr)
	if wait, _ := appErr.Data["retry_after_seconds"].(int); wait < 1 || wait > 60 {
		t.Fatalf("retry_after_seconds = %v, want 1..60", appErr.Data["retry_after_seconds"])
	}

	if hash, sent := f.inviteTokenHash(t, created.InviteID); hash != hashBefore || sent != sentBefore {
		t.Fatal("a throttled resend modified the invite")
	}
	if _, err := f.invites.GetInviteByToken(context.Background(), GetInviteRequest{Token: token}); err != nil {
		t.Fatalf("current link stopped working after a throttled resend: %v", err)
	}
}

// The cooldown is measured on the database clock from last_sent_at.
func TestResendCooldownUsesDatabaseClock(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.InviteResendCooldownSecs = 60
	admin := f.createAdmin(t, "db-clock")
	created, err := f.invite(t, admin, "clock@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE invites SET last_sent_at = now() - interval '61 seconds' WHERE id = $1`, created.InviteID)

	if _, err := f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID); err != nil {
		t.Fatalf("resend after the cooldown: %v", err)
	}
	var sinceSent float64
	if err := f.pool.QueryRow(context.Background(),
		`SELECT extract(epoch FROM now() - last_sent_at) FROM invites WHERE id = $1`, created.InviteID).Scan(&sinceSent); err != nil || sinceSent > 5 {
		t.Fatalf("last_sent_at not refreshed: %v seconds ago, %v", sinceSent, err)
	}
}

func TestConcurrentResendsSendOnce(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.InviteResendCooldownSecs = 60
	f.withNotifications(t, newKeyring(t))
	admin := f.createAdmin(t, "resend-race")
	created, err := f.invite(t, admin, "race@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE invites SET last_sent_at = now() - interval '2 minutes' WHERE id = $1`, created.InviteID)

	errs := runConcurrently(6, func(i int) error {
		_, err := f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID)
		return err
	})
	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		requireAppErr(t, err, apperr.CodeRateLimit)
	}
	if successes != 1 {
		t.Fatalf("%d resends succeeded, want exactly 1 (errors: %v)", successes, errs)
	}
	var jobs int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notification_jobs WHERE org_id = $1`, admin.orgID).Scan(&jobs); err != nil || jobs != 2 {
		t.Fatalf("%d invite emails queued, want 2 (create + one resend)", jobs)
	}
}

// If the email cannot be queued, the resend changes nothing: same link, same
// last_sent_at, so the cooldown is not consumed.
func TestFailedEnqueueDoesNotConsumeCooldown(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.InviteResendCooldownSecs = 60
	admin := f.createAdmin(t, "enqueue-fail")
	token := f.useInviteToken("kept-link")
	created, err := f.invite(t, admin, "kept@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, `UPDATE invites SET last_sent_at = now() - interval '2 minutes' WHERE id = $1`, created.InviteID)
	hashBefore, sentBefore := f.inviteTokenHash(t, created.InviteID)

	f.withNotifications(t, nil) // the queue refuses sensitive emails
	f.useInviteToken("never-sent")
	if _, err := f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID); err == nil {
		t.Fatal("resend succeeded although its email could not be queued")
	}
	if hash, sent := f.inviteTokenHash(t, created.InviteID); hash != hashBefore || sent != sentBefore {
		t.Fatal("a failed resend changed the token or consumed the cooldown")
	}
	if _, err := f.invites.GetInviteByToken(context.Background(), GetInviteRequest{Token: token}); err != nil {
		t.Fatalf("current link broken by a failed resend: %v", err)
	}

	// With a working queue the same resend now goes through.
	f.withNotifications(t, newKeyring(t))
	newToken := f.useInviteToken("fresh-link")
	if _, err := f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID); err != nil {
		t.Fatalf("resend with a working queue: %v", err)
	}
	if _, err := f.invites.GetInviteByToken(context.Background(), GetInviteRequest{Token: newToken}); err != nil {
		t.Fatalf("new link: %v", err)
	}
	_, err = f.invites.GetInviteByToken(context.Background(), GetInviteRequest{Token: token})
	requireAppErr(t, err, apperr.CodeNotFound)
}

// Creating an invite queues its email in the same transaction, encrypted; if
// queuing fails no invite is created.
func TestCreateInviteQueuesEncryptedEmailAtomically(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "create-atomic")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")

	f.withNotifications(t, nil)
	if _, err := f.invite(t, admin, "nobody@example.com", role); err == nil {
		t.Fatal("invite created although its email could not be queued")
	}
	if got := f.countOpenInvites(t, admin.orgID, "nobody@example.com"); got != 0 {
		t.Fatalf("%d invites left behind by a failed create", got)
	}

	f.withNotifications(t, newKeyring(t))
	token := f.useInviteToken("queued-link")
	if _, err := f.invite(t, admin, "queued@example.com", role); err != nil {
		t.Fatal(err)
	}
	var body, metadata string
	var ciphertext []byte
	if err := f.pool.QueryRow(context.Background(),
		`SELECT body, metadata::text, payload_ciphertext FROM notification_jobs WHERE recipient = 'queued@example.com'`).
		Scan(&body, &metadata, &ciphertext); err != nil {
		t.Fatal(err)
	}
	if len(ciphertext) == 0 || strings.Contains(body+metadata, token) || strings.Contains(metadata, "invite_link") {
		t.Fatalf("queued job is not encrypted: body=%q metadata=%s", body, metadata)
	}
}

func TestResendThrottleOverHTTP(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.InviteResendCooldownSecs = 60
	api := f.apiServer(t)
	admin := f.createAdmin(t, "http-throttle")
	created, err := f.invite(t, admin, "throttled@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}

	rec := api.do(t, http.MethodPost, fmt.Sprintf("/api/v1/auth/invites/%s/resend", created.InviteID), f.accessToken(t, admin), nil)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("resend inside cooldown = %d %s, want 429", rec.Code, rec.Body)
	}
	if rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), `"retry_after_seconds"`) {
		t.Fatalf("429 lacks retry information: headers=%v body=%s", rec.Header(), rec.Body)
	}
}

// Invites from callers who share no role (neither their own nor the ones they
// grant) are not serialised by the grant-policy locks, so only the
// one-open-invite-per-email index stops the second insert. The loser must get
// the same 409 as the ordinary duplicate check, never a 500.
func TestInvitesFromUnrelatedInvitersAreStoppedByTheIndex(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "index-race")
	inviterA := f.createMember(t, admin, "inviter-a", f.createRole(t, admin.orgID, "inviter-a", "invites:write", "logs:read"))
	inviterB := f.createMember(t, admin, "inviter-b", f.createRole(t, admin.orgID, "inviter-b", "invites:write", "traces:read"))
	logsRole := f.createRole(t, admin.orgID, "logs-only", "logs:read")
	tracesRole := f.createRole(t, admin.orgID, "traces-only", "traces:read")

	for round := range 15 {
		email := fmt.Sprintf("race-%d@example.com", round)
		errs := runConcurrently(2, func(i int) error {
			if i == 0 {
				_, err := f.invite(t, inviterA, email, logsRole)
				return err
			}
			_, err := f.invite(t, inviterB, strings.ToUpper(email), tracesRole)
			return err
		})
		successes := 0
		for _, err := range errs {
			if err == nil {
				successes++
				continue
			}
			requireAppErr(t, err, apperr.CodeInviteAlreadyExists)
		}
		if successes != 1 {
			t.Fatalf("round %d: %d invites created, want 1 (errors: %v)", round, successes, errs)
		}
	}
}

package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

const newPassword = "a-brand-new-password"

// issueResetToken stores a reset token for u and returns its raw value. The
// forgot-password endpoint only emails the raw token, so tests insert their
// own when they need to redeem one.
func (f *authFixture) issueResetToken(t *testing.T, u testUser, ttl time.Duration) string {
	t.Helper()
	raw, err := GenerateToken()
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.repo.CreatePasswordResetToken(context.Background(), f.repo.Query(), PasswordResetToken{
		UserID: u.userID, TokenHash: HashToken(raw), ExpiresAt: time.Now().Add(ttl),
	})
	if err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}
	return raw
}

func (f *authFixture) resetPassword(rawToken, password string) error {
	_, err := f.accounts.ResetPassword(context.Background(), ResetPasswordRequest{Token: rawToken, Password: password})
	return err
}

func (f *authFixture) resetTokenUsed(t *testing.T, rawToken string) bool {
	t.Helper()
	row, err := f.repo.GetPasswordResetTokenByHash(context.Background(), f.repo.Query(), HashToken(rawToken))
	if err != nil || row == nil {
		t.Fatalf("GetPasswordResetTokenByHash = %v, %v", row, err)
	}
	return row.UsedAt != nil
}

func (f *authFixture) unusedResetTokens(t *testing.T, userID uuid.UUID) int {
	t.Helper()
	var n int
	err := f.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL", userID).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestResetPasswordChangesPasswordAndEndsSessions(t *testing.T) {
	f := newAuthFixture(t)
	victim := f.createUser(t, "victim")
	bystander := f.createUser(t, "bystander")

	laptop := f.mustLogin(t, victim)
	stolen := f.mustLogin(t, victim) // a session an attacker holds
	otherUser := f.mustLogin(t, bystander)
	raw := f.issueResetToken(t, victim, 30*time.Minute)

	if err := f.resetPassword(raw, newPassword); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// The password changed.
	if _, err := f.login(t, victim, newPassword); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	_, err := f.login(t, victim, testPassword)
	requireAppErr(t, err, apperr.CodeInvalidCredentials)

	// Every session the user had before the reset is over…
	for name, session := range map[string]*LoginResponse{"laptop": laptop, "stolen": stolen} {
		_, err := f.refresh(session.RefreshToken)
		requireAppErr(t, err, apperr.CodeTokenExpired)
		if f.tokenRow(t, session.RefreshToken).RevokedAt == nil {
			t.Errorf("%s session still active after password reset", name)
		}
	}
	// …and only that user's: another user's session still refreshes.
	if _, err := f.refresh(otherUser.RefreshToken); err != nil {
		t.Errorf("another user's session was revoked: %v", err)
	}

	if !f.resetTokenUsed(t, raw) {
		t.Error("reset token not marked used")
	}
}

func TestResetPasswordRejectsUnusableTokens(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "reject-tokens")

	t.Run("unknown", func(t *testing.T) {
		requireAppErr(t, f.resetPassword("no-such-token", newPassword), apperr.CodeInvalidToken)
	})
	t.Run("expired", func(t *testing.T) {
		raw := f.issueResetToken(t, u, -time.Minute)
		requireAppErr(t, f.resetPassword(raw, newPassword), apperr.CodeTokenExpired)
	})
	t.Run("already used", func(t *testing.T) {
		raw := f.issueResetToken(t, u, 30*time.Minute)
		if err := f.resetPassword(raw, newPassword); err != nil {
			t.Fatal(err)
		}
		requireAppErr(t, f.resetPassword(raw, "yet-another-password"), apperr.CodeTokenExpired)
		if _, err := f.login(t, u, newPassword); err != nil {
			t.Errorf("second reset with a used token changed the password: %v", err)
		}
	})
}

// Of several concurrent resets with one token exactly one succeeds, and its
// password is the one that sticks.
func TestConcurrentResetConsumesTokenOnce(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "concurrent-reset")
	raw := f.issueResetToken(t, u, 30*time.Minute)

	const n = 6
	passwords := make([]string, n)
	for i := range passwords {
		passwords[i] = fmt.Sprintf("concurrent-password-%d", i)
	}
	errs := runConcurrently(n, func(i int) error { return f.resetPassword(raw, passwords[i]) })

	winner := -1
	for i, err := range errs {
		if err == nil {
			if winner != -1 {
				t.Fatalf("requests %d and %d both reset the password", winner, i)
			}
			winner = i
			continue
		}
		requireAppErr(t, err, apperr.CodeTokenExpired)
	}
	if winner == -1 {
		t.Fatalf("no reset succeeded: %v", errs)
	}
	for i, password := range passwords {
		_, err := f.login(t, u, password)
		if (err == nil) != (i == winner) {
			t.Errorf("login with password %d: err = %v, winner = %d", i, err, winner)
		}
	}
}

// When any step fails, the reset rolls back completely: the password, the
// sessions and the token are all as they were.
func TestResetPasswordRollsBackOnFailure(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "rollback")
	session := f.mustLogin(t, u)
	raw := f.issueResetToken(t, u, 30*time.Minute)

	// Make the password update fail for this user only.
	f.exec(t, `CREATE OR REPLACE FUNCTION test_fail_password_update() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected failure'; END $$`)
	f.exec(t, fmt.Sprintf(`CREATE TRIGGER test_fail_password_update
		BEFORE UPDATE OF password_hash ON users
		FOR EACH ROW WHEN (NEW.id = '%s') EXECUTE FUNCTION test_fail_password_update()`, u.userID))
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = f.pool.Exec(ctx, "DROP TRIGGER IF EXISTS test_fail_password_update ON users")
		_, _ = f.pool.Exec(ctx, "DROP FUNCTION IF EXISTS test_fail_password_update()")
	})

	requireAppErr(t, f.resetPassword(raw, newPassword), apperr.CodeDatabase)

	if f.resetTokenUsed(t, raw) {
		t.Error("reset token consumed by a failed reset")
	}
	if _, err := f.login(t, u, testPassword); err != nil {
		t.Errorf("old password stopped working after a failed reset: %v", err)
	}
	if f.tokenRow(t, session.RefreshToken).RevokedAt != nil {
		t.Error("session revoked by a failed reset")
	}

	// With the fault removed the same token still works.
	f.exec(t, "DROP TRIGGER test_fail_password_update ON users")
	if err := f.resetPassword(raw, newPassword); err != nil {
		t.Fatalf("reset after removing the fault: %v", err)
	}
}

// A successful reset also retires the user's other outstanding reset links.
func TestResetPasswordRetiresOtherResetTokens(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "other-links")
	first := f.issueResetToken(t, u, 30*time.Minute)
	second := f.issueResetToken(t, u, 30*time.Minute)

	if err := f.resetPassword(second, newPassword); err != nil {
		t.Fatal(err)
	}
	requireAppErr(t, f.resetPassword(first, "attacker-password"), apperr.CodeTokenExpired)
}

func TestForgotPasswordKeepsOnlyNewestToken(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "forgot")
	ctx := context.Background()
	older := f.issueResetToken(t, u, 30*time.Minute)

	resp, err := f.accounts.ForgotPassword(ctx, ForgotPasswordRequest{OrgSlug: u.orgSlug, Email: u.email})
	if err != nil {
		t.Fatal(err)
	}
	if !f.resetTokenUsed(t, older) {
		t.Error("older reset token still usable after a new request")
	}
	if got := f.unusedResetTokens(t, u.userID); got != 1 {
		t.Errorf("%d usable reset tokens, want 1", got)
	}
	requireAppErr(t, f.resetPassword(older, newPassword), apperr.CodeTokenExpired)

	// Unknown accounts get the same response and create nothing.
	unknown, err := f.accounts.ForgotPassword(ctx, ForgotPasswordRequest{OrgSlug: u.orgSlug, Email: "nobody@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Message != resp.Message {
		t.Errorf("unknown-account message %q differs from %q", unknown.Message, resp.Message)
	}
}

// Concurrent forgot-password requests for one user still leave exactly one
// usable link.
func TestConcurrentForgotPasswordLeavesOneToken(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "forgot-concurrent")

	errs := runConcurrently(6, func(int) error {
		_, err := f.accounts.ForgotPassword(context.Background(), ForgotPasswordRequest{OrgSlug: u.orgSlug, Email: u.email})
		return err
	})
	for _, err := range errs {
		if err != nil {
			t.Fatalf("ForgotPassword: %v", err)
		}
	}
	if got := f.unusedResetTokens(t, u.userID); got != 1 {
		t.Errorf("%d usable reset tokens after concurrent requests, want 1", got)
	}
}

// Every endpoint that sets a password rejects ones bcrypt cannot hash in full
// with a validation error instead of a 500.
func TestOverlongPasswordsAreValidationErrors(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	tooLong := strings.Repeat("€", 25) // 25 characters, 75 bytes

	_, err := f.accounts.RegisterOrgOwner(ctx, RegisterOrgOwnerRequest{
		OrgName: "long", OrgSlug: "long-pass", FirstName: "L", LastName: "P", Email: "l@example.com", Password: tooLong,
	})
	requireAppErr(t, err, apperr.CodeValidation)

	admin := f.createAdmin(t, "long-pass-org")
	raw := f.issueResetToken(t, admin, 30*time.Minute)
	requireAppErr(t, f.resetPassword(raw, tooLong), apperr.CodeValidation)
	if f.resetTokenUsed(t, raw) {
		t.Error("rejected password consumed the reset token")
	}

	token := f.useInviteToken("long-pass-invite")
	if _, err := f.invite(t, admin, "long@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read")); err != nil {
		t.Fatal(err)
	}
	_, err = f.accept(token, tooLong)
	requireAppErr(t, err, apperr.CodeValidation)
	if f.userByEmail(t, admin.orgID, "long@example.com") != nil {
		t.Error("rejected password created a user")
	}
}

// Within the cooldown a new request changes nothing: the link already sent
// keeps working and no new one is issued.
func TestForgotPasswordCooldownKeepsExistingLink(t *testing.T) {
	f := newAuthFixture(t)
	f.cfg.Auth.PasswordResetCooldownSecs = 60
	u := f.createUser(t, "cooldown")
	ctx := context.Background()
	request := func() {
		t.Helper()
		resp, err := f.accounts.ForgotPassword(ctx, ForgotPasswordRequest{OrgSlug: u.orgSlug, Email: u.email})
		if err != nil || !strings.HasPrefix(resp.Message, "If the account exists") {
			t.Fatalf("ForgotPassword = %+v, %v; want the generic response", resp, err)
		}
	}

	sent := f.issueResetToken(t, u, 30*time.Minute) // the link the user just received
	for range 3 {
		request()
	}
	if f.resetTokenUsed(t, sent) {
		t.Fatal("a throttled request invalidated the link already sent")
	}
	if got := f.unusedResetTokens(t, u.userID); got != 1 {
		t.Fatalf("%d usable reset tokens, want just the one already sent", got)
	}
	if err := f.resetPassword(sent, newPassword); err != nil {
		t.Fatalf("the link sent before the throttled requests: %v", err)
	}

	// Once the cooldown has passed, a request issues a new link again.
	f.exec(t, "UPDATE password_reset_tokens SET created_at = now() - interval '2 minutes' WHERE user_id = $1", u.userID)
	request()
	if got := f.unusedResetTokens(t, u.userID); got != 1 {
		t.Fatalf("%d usable reset tokens after the cooldown, want 1 new one", got)
	}
}

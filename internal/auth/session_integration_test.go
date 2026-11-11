package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

func TestLoginStartsSession(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "login-session")

	resp := f.mustLogin(t, u)
	row := f.tokenRow(t, resp.RefreshToken)

	if row.SessionExpiresAt == nil {
		t.Fatal("session_expires_at is NULL on a new session")
	}
	wantEnd := time.Now().Add(f.cfg.Auth.RefreshSessionMaxTTL())
	if d := row.SessionExpiresAt.Sub(wantEnd); d > time.Minute || d < -time.Minute {
		t.Errorf("session end = %v, want about %v", row.SessionExpiresAt, wantEnd)
	}
	wantExpiry := time.Now().Add(f.cfg.Auth.RefreshTokenTTL())
	if d := row.ExpiresAt.Sub(wantExpiry); d > time.Minute || d < -time.Minute {
		t.Errorf("token expiry = %v, want about %v", row.ExpiresAt, wantExpiry)
	}
}

func TestRefreshRotatesWithinSession(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "rotate")
	login := f.mustLogin(t, u)
	parent := f.tokenRow(t, login.RefreshToken)

	rotated, err := f.refresh(login.RefreshToken)
	if err != nil {
		t.Fatalf("RefreshToken: %v", err)
	}
	if rotated.RefreshToken == login.RefreshToken || rotated.AccessToken == "" {
		t.Fatal("rotation did not issue new tokens")
	}
	if rotated.User.ID != u.userID {
		t.Errorf("user = %s, want %s", rotated.User.ID, u.userID)
	}

	if f.tokenRow(t, login.RefreshToken).RevokedAt == nil {
		t.Error("parent token was not revoked by rotation")
	}
	child := f.tokenRow(t, rotated.RefreshToken)
	if child.RevokedAt != nil {
		t.Error("child token is revoked")
	}
	if child.FamilyID != parent.FamilyID {
		t.Errorf("child family = %s, want parent's %s", child.FamilyID, parent.FamilyID)
	}
	if !child.SessionEnd().Equal(parent.SessionEnd()) {
		t.Errorf("child session end = %v, want parent's %v", child.SessionEnd(), parent.SessionEnd())
	}

	// The new token keeps working.
	if _, err := f.refresh(rotated.RefreshToken); err != nil {
		t.Fatalf("refresh with rotated token: %v", err)
	}
}

func TestRefreshUnknownToken(t *testing.T) {
	f := newAuthFixture(t)
	_, err := f.refresh("not-a-real-token")
	requireAppErr(t, err, apperr.CodeInvalidToken)
}

// Reusing a rotated (revoked) token is rejected and ends the whole session,
// including the token that replaced it.
func TestRefreshReuseRevokesFamily(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "reuse")
	login := f.mustLogin(t, u)
	other := f.mustLogin(t, u) // a second, unrelated session of the same user

	rotated, err := f.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}

	_, err = f.refresh(login.RefreshToken)
	requireAppErr(t, err, apperr.CodeTokenExpired)

	if f.tokenRow(t, rotated.RefreshToken).RevokedAt == nil {
		t.Error("token issued by the rotation is still active after reuse of its parent")
	}
	_, err = f.refresh(rotated.RefreshToken)
	requireAppErr(t, err, apperr.CodeTokenExpired)

	// Other sessions are untouched.
	if _, err := f.refresh(other.RefreshToken); err != nil {
		t.Errorf("unrelated session was revoked: %v", err)
	}
}

// An expired token is rejected but is not treated as reuse: the rest of its
// session is left alone.
func TestRefreshExpiredTokenDoesNotRevokeFamily(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "expired")

	t.Run("expired active token", func(t *testing.T) {
		login := f.mustLogin(t, u)
		f.exec(t, "UPDATE refresh_tokens SET expires_at = now() - interval '1 minute' WHERE token_hash = $1", HashToken(login.RefreshToken))

		_, err := f.refresh(login.RefreshToken)
		requireAppErr(t, err, apperr.CodeTokenExpired)
		if f.tokenRow(t, login.RefreshToken).RevokedAt != nil {
			t.Error("expired token was revoked; expiry must not count as reuse")
		}
	})

	t.Run("expired rotated token", func(t *testing.T) {
		login := f.mustLogin(t, u)
		rotated, err := f.refresh(login.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		// The rotated parent is both revoked and expired.
		f.exec(t, "UPDATE refresh_tokens SET expires_at = now() - interval '1 minute' WHERE token_hash = $1", HashToken(login.RefreshToken))

		_, err = f.refresh(login.RefreshToken)
		requireAppErr(t, err, apperr.CodeTokenExpired)
		if _, err := f.refresh(rotated.RefreshToken); err != nil {
			t.Errorf("session was revoked after presenting an expired token: %v", err)
		}
	})
}

// Of several concurrent refreshes with one token exactly one rotates it. The
// others find it revoked, which the replay policy treats as reuse, so the
// session (including the winner's new token) ends. The frontend never sends
// one token twice concurrently (single-flight plus a cross-tab lock).
func TestConcurrentRefreshRotatesOnce(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "concurrent-refresh")
	login := f.mustLogin(t, u)
	family := f.tokenRow(t, login.RefreshToken).FamilyID

	const n = 8
	results := make([]*LoginResponse, n)
	errs := runConcurrently(n, func(i int) error {
		resp, err := f.refresh(login.RefreshToken)
		results[i] = resp
		return err
	})

	successes := 0
	for i, err := range errs {
		if err == nil {
			successes++
			continue
		}
		requireAppErr(t, err, apperr.CodeTokenExpired)
		if results[i] != nil {
			t.Errorf("request %d returned tokens with an error", i)
		}
	}
	if successes != 1 {
		t.Fatalf("%d refreshes succeeded, want exactly 1 (errors: %v)", successes, errs)
	}
	if got := f.countTokens(t, "family_id = $1", family); got != 2 {
		t.Errorf("session has %d tokens, want 2 (the parent and one child)", got)
	}
	if got := f.countTokens(t, "family_id = $1 AND revoked_at IS NULL", family); got != 0 {
		t.Errorf("%d tokens still active, want 0 after concurrent reuse", got)
	}
}

// Rotation never extends a session past its absolute end.
func TestRefreshCannotExceedSessionEnd(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "session-cap")
	login := f.mustLogin(t, u)
	family := f.tokenRow(t, login.RefreshToken).FamilyID

	// The session ends in an hour, sooner than a token's normal 7 days.
	f.exec(t, "UPDATE refresh_tokens SET session_expires_at = now() + interval '1 hour' WHERE family_id = $1", family)
	sessionEnd := *f.tokenRow(t, login.RefreshToken).SessionExpiresAt

	rotated, err := f.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	child := f.tokenRow(t, rotated.RefreshToken)
	if !child.SessionEnd().Equal(sessionEnd) {
		t.Errorf("child session end = %v, want %v", child.SessionEnd(), sessionEnd)
	}
	if child.ExpiresAt.After(sessionEnd) {
		t.Errorf("child expires %v, after the session end %v", child.ExpiresAt, sessionEnd)
	}

	// Once the session end passes, the last token is expired and refresh fails.
	f.exec(t, "UPDATE refresh_tokens SET session_expires_at = now() - interval '1 second', expires_at = now() - interval '1 second' WHERE family_id = $1", family)
	_, err = f.refresh(rotated.RefreshToken)
	requireAppErr(t, err, apperr.CodeTokenExpired)
}

// Sessions created before migration 000013 have no session_expires_at; they
// end with their current token and rotation cannot extend them.
func TestRefreshLegacyTokenIsCapped(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "legacy")
	login := f.mustLogin(t, u)
	f.exec(t, "UPDATE refresh_tokens SET session_expires_at = NULL, expires_at = now() + interval '2 hours' WHERE token_hash = $1", HashToken(login.RefreshToken))
	legacyEnd := f.tokenRow(t, login.RefreshToken).ExpiresAt

	rotated, err := f.refresh(login.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	child := f.tokenRow(t, rotated.RefreshToken)
	if child.ExpiresAt.After(legacyEnd) || !child.SessionEnd().Equal(legacyEnd) {
		t.Errorf("legacy session extended: child expires %v, session end %v, want <= %v", child.ExpiresAt, child.SessionEnd(), legacyEnd)
	}
}

// A rotation that fails before commit leaves the presented token usable.
func TestFailedRotationKeepsPreviousToken(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "failed-rotation")
	login := f.mustLogin(t, u)
	family := f.tokenRow(t, login.RefreshToken).FamilyID

	sign := f.sessions.issuer.signAccessToken
	f.sessions.issuer.signAccessToken = func(User) (string, error) { return "", errors.New("signing unavailable") }

	_, err := f.refresh(login.RefreshToken)
	requireAppErr(t, err, apperr.CodeInternal)

	if f.tokenRow(t, login.RefreshToken).RevokedAt != nil {
		t.Fatal("presented token was revoked by a failed rotation")
	}
	if got := f.countTokens(t, "family_id = $1", family); got != 1 {
		t.Fatalf("session has %d tokens after a failed rotation, want 1", got)
	}

	f.sessions.issuer.signAccessToken = sign
	if _, err := f.refresh(login.RefreshToken); err != nil {
		t.Fatalf("refresh after the failure: %v", err)
	}
}

func TestLogoutRevokesToken(t *testing.T) {
	f := newAuthFixture(t)
	u := f.createUser(t, "logout")
	login := f.mustLogin(t, u)

	if err := f.sessions.Logout(context.Background(), LogoutRequest{RefreshToken: login.RefreshToken}); err != nil {
		t.Fatal(err)
	}
	_, err := f.refresh(login.RefreshToken)
	requireAppErr(t, err, apperr.CodeTokenExpired)
}

// Invite acceptance issues its session through the shared issuer: the result
// must be a normal, refreshable session.
func TestAcceptInviteIssuesSession(t *testing.T) {
	f := newAuthFixture(t)
	owner := f.createUser(t, "invite-org")
	ctx := context.Background()

	role, err := f.repo.CreateRole(ctx, f.repo.Query(), Role{OrgID: owner.orgID, Name: "viewer"})
	if err != nil {
		t.Fatal(err)
	}
	inviteToken := f.useInviteToken("invite-session-token")
	if _, err := f.invites.CreateInvite(ctx, InviteUserRequest{
		FirstName: "New", LastName: "Member", Email: "new.member@example.com",
		RoleIDs: []uuid.UUID{role.ID}, UserID: owner.userID, OrgID: owner.orgID,
	}); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	details, err := f.invites.GetInviteByToken(ctx, GetInviteRequest{Token: inviteToken})
	if err != nil {
		t.Fatalf("GetInviteByToken: %v", err)
	}
	if details.Email != "new.member@example.com" || details.OrgName != "invite-org" || details.OrgSlug != "invite-org" {
		t.Fatalf("invite details = %+v, want the invitee's email and the org's name and slug", details)
	}

	resp, err := f.invites.AcceptInvite(ctx, AcceptInviteRequest{Token: inviteToken, Password: "a-new-password"})
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if resp.AccessToken == "" || resp.User.Email != "new.member@example.com" || resp.User.OrgID != owner.orgID {
		t.Fatalf("unexpected AcceptInvite response: %+v", resp.User)
	}
	userID, orgID, _, err := f.sessions.ParseToken(resp.AccessToken)
	if err != nil || userID != resp.User.ID.String() || orgID != owner.orgID.String() {
		t.Fatalf("access token claims = %s/%s, %v", userID, orgID, err)
	}

	row := f.tokenRow(t, resp.RefreshToken)
	if row.SessionExpiresAt == nil || row.UserID != resp.User.ID {
		t.Fatalf("invite session row = %+v", row)
	}
	if _, err := f.refresh(resp.RefreshToken); err != nil {
		t.Fatalf("refresh after accepting invite: %v", err)
	}
}

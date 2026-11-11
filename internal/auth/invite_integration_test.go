package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

func (f *authFixture) accept(token, password string) (*LoginResponse, error) {
	return f.invites.AcceptInvite(context.Background(), AcceptInviteRequest{Token: token, Password: password})
}

func (f *authFixture) userByEmail(t *testing.T, orgID uuid.UUID, email string) *User {
	t.Helper()
	user, err := f.repo.GetUserByEmail(context.Background(), f.repo.Query(), orgID, email)
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func (f *authFixture) userRoleIDs(t *testing.T, userID uuid.UUID) []uuid.UUID {
	t.Helper()
	ids, err := collectIDs(context.Background(), f.pool, "user roles", "SELECT role_id FROM user_roles WHERE user_id = $1 ORDER BY role_id", userID)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func TestConcurrentInviteAcceptanceSucceedsOnce(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "accept-race")
	token := f.useInviteToken("race-token")
	if _, err := f.invite(t, admin, "racer@example.com", f.createRole(t, admin.orgID, "viewer", "traces:read")); err != nil {
		t.Fatal(err)
	}

	const n = 6
	errs := runConcurrently(n, func(i int) error {
		_, err := f.accept(token, fmt.Sprintf("password-number-%d", i))
		return err
	})

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		requireAppErr(t, err, apperr.CodeInviteAlreadyAccepted)
	}
	if successes != 1 {
		t.Fatalf("%d acceptances succeeded, want exactly 1 (errors: %v)", successes, errs)
	}
	var users int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE email = 'racer@example.com'").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("%d users created, want 1", users)
	}
}

// The org, email and roles of the new user come only from the invite row,
// and the email is verified only by a successful acceptance.
func TestAcceptInviteUsesInviteRecord(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "accept-record")
	other := f.createAdmin(t, "accept-record-other")
	viewer := f.createRole(t, admin.orgID, "viewer", "traces:read")
	token := f.useInviteToken("record-token")
	created, err := f.invite(t, admin, "joiner@example.com", viewer)
	if err != nil {
		t.Fatal(err)
	}
	// Plant a foreign role on the invite, as stale or tampered data would.
	foreign := f.createRole(t, other.orgID, "foreign-admin", "roles:update")
	f.exec(t, "INSERT INTO invite_roles (invite_id, role_id) VALUES ($1, $2)", created.InviteID, foreign)

	resp, err := f.accept(token, "joiner-password")
	if err != nil {
		t.Fatalf("AcceptInvite: %v", err)
	}
	if resp.User.OrgID != admin.orgID || resp.User.Email != "joiner@example.com" {
		t.Fatalf("user org/email = %s/%s, want the invite's", resp.User.OrgID, resp.User.Email)
	}
	if !resp.User.IsEmailVerified {
		t.Error("accepted user's email is not verified")
	}
	if got := f.userRoleIDs(t, resp.User.ID); !slices.Equal(got, []uuid.UUID{viewer}) {
		t.Errorf("user roles = %v, want only the invite's own-org role %s", got, viewer)
	}

	// Registration does not verify email; only invite acceptance does.
	if f.userByEmail(t, admin.orgID, admin.email).IsEmailVerified {
		t.Error("registered admin is marked verified")
	}
}

func TestAcceptInviteRejectsUnusableInvites(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "accept-reject")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")
	ctx := context.Background()

	t.Run("expired", func(t *testing.T) {
		token := f.useInviteToken("expired-token")
		if _, err := f.invite(t, admin, "late@example.com", role); err != nil {
			t.Fatal(err)
		}
		f.exec(t, "UPDATE invites SET expires_at = now() - interval '1 minute' WHERE token_hash = $1", HashToken(token))
		_, err := f.accept(token, "late-password")
		requireAppErr(t, err, apperr.CodeInviteExpired)
		if f.userByEmail(t, admin.orgID, "late@example.com") != nil {
			t.Error("expired invite created a user")
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		token := f.useInviteToken("cancelled-token")
		created, err := f.invite(t, admin, "cancelled@example.com", role)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.invites.CancelInvite(ctx, admin.orgID, admin.userID, created.InviteID); err != nil {
			t.Fatal(err)
		}
		_, err = f.accept(token, "cancelled-password")
		requireAppErr(t, err, apperr.CodeInviteRevoked)
		_, err = f.invites.GetInviteByToken(ctx, GetInviteRequest{Token: token})
		requireAppErr(t, err, apperr.CodeInviteRevoked)
	})

	t.Run("already accepted", func(t *testing.T) {
		token := f.useInviteToken("twice-token")
		if _, err := f.invite(t, admin, "twice@example.com", role); err != nil {
			t.Fatal(err)
		}
		if _, err := f.accept(token, "twice-password"); err != nil {
			t.Fatal(err)
		}
		_, err := f.accept(token, "twice-password")
		requireAppErr(t, err, apperr.CodeInviteAlreadyAccepted)
	})

	t.Run("email registered meanwhile rolls back", func(t *testing.T) {
		token := f.useInviteToken("taken-token")
		if _, err := f.invite(t, admin, "taken@example.com", role); err != nil {
			t.Fatal(err)
		}
		f.createMember(t, admin, "taken") // creates taken@accept-reject.example.com
		f.exec(t, "UPDATE users SET email = 'taken@example.com' WHERE email = 'taken@accept-reject.example.com'")

		_, err := f.accept(token, "taken-password")
		requireAppErr(t, err, apperr.CodeEmailExists)
		invite, err := f.repo.GetInviteByTokenHash(ctx, f.repo.Query(), HashToken(token))
		if err != nil || invite == nil || invite.AcceptedAt != nil {
			t.Fatalf("invite after a rolled-back acceptance = %+v, %v; want still pending", invite, err)
		}
		if f.userByEmail(t, admin.orgID, "taken@example.com").IsEmailVerified {
			t.Error("failed acceptance verified the existing user's email")
		}
	})
}

// Invite tokens are delivered by email only: no API response carries one.
func TestInviteResponsesNeverContainToken(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "no-token")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")

	token := f.useInviteToken("secret-invite-token-1")
	created, err := f.invite(t, admin, "quiet@example.com", role)
	if err != nil {
		t.Fatal(err)
	}
	requireNoToken(t, created, token)

	resentToken := f.useInviteToken("secret-invite-token-2")
	resent, err := f.invites.ResendInvite(context.Background(), admin.orgID, admin.userID, created.InviteID)
	if err != nil {
		t.Fatal(err)
	}
	requireNoToken(t, resent, resentToken)

	listed, _, err := f.invites.ListInvites(context.Background(), admin.orgID, "", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	requireNoToken(t, listed, resentToken)
	requireNoToken(t, listed, HashToken(resentToken))
}

func requireNoToken(t *testing.T, value any, token string) {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), token) || strings.Contains(string(body), "invite_link") {
		t.Fatalf("response exposes the invite token: %s", body)
	}
}

func TestResendAndCancelInvite(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "lifecycle")
	role := f.createRole(t, admin.orgID, "viewer", "traces:read")
	ctx := context.Background()

	oldToken := f.useInviteToken("first-link")
	created, err := f.invite(t, admin, "lifecycle@example.com", role)
	if err != nil {
		t.Fatal(err)
	}
	f.exec(t, "UPDATE invites SET expires_at = now() - interval '1 day' WHERE id = $1", created.InviteID)

	// An expired invite can be resent; the new link works and the old one does not.
	newToken := f.useInviteToken("second-link")
	resent, err := f.invites.ResendInvite(ctx, admin.orgID, admin.userID, created.InviteID)
	if err != nil {
		t.Fatalf("ResendInvite: %v", err)
	}
	if !resent.ExpiresAt.After(created.ExpiresAt) {
		t.Errorf("resend did not extend the expiry: %v -> %v", created.ExpiresAt, resent.ExpiresAt)
	}
	_, err = f.invites.GetInviteByToken(ctx, GetInviteRequest{Token: oldToken})
	requireAppErr(t, err, apperr.CodeNotFound)
	if _, err := f.invites.GetInviteByToken(ctx, GetInviteRequest{Token: newToken}); err != nil {
		t.Fatalf("new link: %v", err)
	}

	// Cancel is idempotent and leaves the row listed as revoked.
	for range 2 {
		if err := f.invites.CancelInvite(ctx, admin.orgID, admin.userID, created.InviteID); err != nil {
			t.Fatalf("CancelInvite: %v", err)
		}
	}
	revoked, total, err := f.invites.ListInvites(ctx, admin.orgID, InviteStatusRevoked, 50, 0)
	if err != nil || total != 1 || revoked[0].ID != created.InviteID {
		t.Fatalf("revoked list = %+v (total %d), %v", revoked, total, err)
	}
	_, err = f.invites.ResendInvite(ctx, admin.orgID, admin.userID, created.InviteID)
	requireAppErr(t, err, apperr.CodeInviteRevoked)

	// An accepted invite can be neither resent nor cancelled.
	acceptedToken := f.useInviteToken("accepted-link")
	accepted, err := f.invite(t, admin, "joined@example.com", role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.accept(acceptedToken, "joined-password"); err != nil {
		t.Fatal(err)
	}
	requireAppErr(t, f.invites.CancelInvite(ctx, admin.orgID, admin.userID, accepted.InviteID), apperr.CodeInviteAlreadyAccepted)
	_, err = f.invites.ResendInvite(ctx, admin.orgID, admin.userID, accepted.InviteID)
	requireAppErr(t, err, apperr.CodeInviteAlreadyAccepted)
}

// Resending hands the invite's roles out again, so the grant policy applies.
func TestResendRequiresGrantAuthority(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "resend-grant")
	created, err := f.invite(t, admin, "future.admin@example.com", f.adminRoleID(t, admin.orgID))
	if err != nil {
		t.Fatal(err)
	}
	inviter := f.createMember(t, admin, "inviter", f.createRole(t, admin.orgID, "inviter", "invites:write"))

	_, err = f.invites.ResendInvite(context.Background(), admin.orgID, inviter.userID, created.InviteID)
	requireAppErr(t, err, apperr.CodePermissionDenied)
}

func TestInviteLifecycleIsTenantIsolated(t *testing.T) {
	f := newAuthFixture(t)
	orgA := f.createAdmin(t, "tenant-a")
	orgB := f.createAdmin(t, "tenant-b")
	ctx := context.Background()
	theirs, err := f.invite(t, orgB, "b.person@example.com", f.createRole(t, orgB.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}

	listed, total, err := f.invites.ListInvites(ctx, orgA.orgID, "", 50, 0)
	if err != nil || total != 0 || len(listed) != 0 {
		t.Fatalf("org A sees %d invites (total %d), %v; want none of org B's", len(listed), total, err)
	}
	_, err = f.invites.ResendInvite(ctx, orgA.orgID, orgA.userID, theirs.InviteID)
	requireAppErr(t, err, apperr.CodeNotFound)
	requireAppErr(t, f.invites.CancelInvite(ctx, orgA.orgID, orgA.userID, theirs.InviteID), apperr.CodeNotFound)

	pending, total, err := f.invites.ListInvites(ctx, orgB.orgID, InviteStatusPending, 50, 0)
	if err != nil || total != 1 || pending[0].ID != theirs.InviteID {
		t.Fatalf("org B's invite was changed by org A: %+v (total %d), %v", pending, total, err)
	}
}

func TestMeReturnsEffectivePermissions(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "me-perms")
	member := f.createMember(t, admin, "member", f.createRole(t, admin.orgID, "viewer", "traces:read", "logs:read"))

	me, err := f.accounts.Me(context.Background(), MeRequest{UserID: member.userID, OrgID: member.orgID})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range me.Permissions {
		got = append(got, p.Name+":"+string(p.Action))
	}
	if !slices.Equal(got, []string{"logs:read", "traces:read"}) {
		t.Fatalf("member permissions = %v", got)
	}

	adminMe, err := f.accounts.Me(context.Background(), MeRequest{UserID: admin.userID, OrgID: admin.orgID})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(adminMe.Permissions, func(p PermissionGrant) bool { return p.Name == "invites" && p.Action == "delete" }) {
		t.Fatal("admin lacks invites:delete")
	}
}

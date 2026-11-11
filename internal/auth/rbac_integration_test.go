package auth

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

func (f *authFixture) invite(t *testing.T, caller testUser, email string, roleIDs ...uuid.UUID) (*InviteUserResponse, error) {
	t.Helper()
	return f.invites.CreateInvite(context.Background(), InviteUserRequest{
		FirstName: "Invited", LastName: "Person", Email: email,
		RoleIDs: roleIDs, UserID: caller.userID, OrgID: caller.orgID,
	})
}

func (f *authFixture) countInvites(t *testing.T, orgID uuid.UUID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM invites WHERE org_id = $1", orgID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// V1: invites:write alone must not let someone invite an admin.
func TestInviteCannotGrantMoreThanCallerHolds(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-invite")
	inviterRole := f.createRole(t, admin.orgID, "inviter", "invites:write", "traces:read")
	inviter := f.createMember(t, admin, "inviter", inviterRole)

	_, err := f.invite(t, inviter, "attacker@example.com", f.adminRoleID(t, admin.orgID))
	requireAppErr(t, err, apperr.CodePermissionDenied)

	logsRole := f.createRole(t, admin.orgID, "log-reader", "logs:read")
	_, err = f.invite(t, inviter, "logs@example.com", logsRole)
	requireAppErr(t, err, apperr.CodePermissionDenied)

	if got := f.countInvites(t, admin.orgID); got != 0 {
		t.Fatalf("%d invites stored after refused requests, want 0", got)
	}

	// A role carrying only what the inviter holds is fine.
	tracesRole := f.createRole(t, admin.orgID, "trace-reader", "traces:read")
	if _, err := f.invite(t, inviter, "viewer@example.com", tracesRole); err != nil {
		t.Fatalf("invite with a grantable role: %v", err)
	}
	// And the org admin may invite another admin.
	if _, err := f.invite(t, admin, "second.admin@example.com", f.adminRoleID(t, admin.orgID)); err != nil {
		t.Fatalf("admin inviting an admin: %v", err)
	}
}

func TestInviteRejectsUnknownAndForeignRoles(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-home")
	other := f.createAdmin(t, "rbac-other")
	foreignRole := f.createRole(t, other.orgID, "foreign", "traces:read")

	for name, roles := range map[string][]uuid.UUID{
		"another org's role": {foreignRole},
		"unknown role":       {uuid.New()},
		"no roles":           {},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := f.invite(t, admin, "x@example.com", roles...)
			requireAppErr(t, err, apperr.CodeValidation)
		})
	}
	if got := f.countInvites(t, admin.orgID); got != 0 {
		t.Fatalf("%d invites stored, want 0", got)
	}
}

// V2a: roles:write must not create a role carrying permissions the caller lacks.
func TestCreateRoleCannotExceedCallerPermissions(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-create")
	creator := f.createMember(t, admin, "creator", f.createRole(t, admin.orgID, "role-maker", "roles:write", "traces:read"))
	ctx := context.Background()
	create := func(name string, permissions ...uuid.UUID) error {
		_, err := f.accessService().CreateRole(ctx, CreateRoleRequest{
			Name: name, PermissionIDs: permissions, OrgID: admin.orgID, UserID: creator.userID,
		})
		return err
	}

	err := create("everything", f.permissionIDs(t, "traces:read", "api_keys:write")...)
	requireAppErr(t, err, apperr.CodePermissionDenied)

	requireAppErr(t, create("bogus", uuid.New()), apperr.CodeValidation)

	if err := create("traces-only", f.permissionID(t, "traces:read")); err != nil {
		t.Fatalf("creating a role within the caller's permissions: %v", err)
	}
	requireAppErr(t, create("traces-only"), apperr.CodeRoleAlreadyExists)
}

// V2b: roles:update must not let a user add permissions to their own role.
func TestRoleEditCannotEscalateOwnPermissions(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-escalate")
	ownRole := f.createRole(t, admin.orgID, "editor", "roles:update", "traces:read")
	editor := f.createMember(t, admin, "editor", ownRole)

	_, err := f.updateRole(t, editor, ownRole, "editor", "roles:update", "traces:read", "invites:write")
	requireAppErr(t, err, apperr.CodePermissionDenied)

	if got := f.rolePermissionNames(t, ownRole); !slices.Equal(got, []string{"roles:update", "traces:read"}) {
		t.Fatalf("role permissions after refused edit = %v", got)
	}
}

// Removing permissions is always safe, even ones the caller does not hold.
func TestRoleEditAllowsReductions(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-reduce")
	editor := f.createMember(t, admin, "editor", f.createRole(t, admin.orgID, "editor", "roles:update", "traces:read"))
	broad := f.createRole(t, admin.orgID, "broad", "traces:read", "logs:read", "metrics:read")

	// Drop metrics:read, keep logs:read (which the editor lacks), rename.
	if _, err := f.updateRole(t, editor, broad, "narrower", "traces:read", "logs:read"); err != nil {
		t.Fatalf("reducing a role: %v", err)
	}
	if got := f.rolePermissionNames(t, broad); !slices.Equal(got, []string{"logs:read", "traces:read"}) {
		t.Fatalf("permissions after reduction = %v", got)
	}

	// Re-adding metrics:read is an addition the editor cannot grant.
	_, err := f.updateRole(t, editor, broad, "narrower", "traces:read", "logs:read", "metrics:read")
	requireAppErr(t, err, apperr.CodePermissionDenied)
}

func TestRoleEditProtectsSystemAndForeignRoles(t *testing.T) {
	f := newAuthFixture(t)
	admin := f.createAdmin(t, "rbac-system")
	other := f.createAdmin(t, "rbac-system-other")

	_, err := f.updateRole(t, admin, f.adminRoleID(t, admin.orgID), "admin", "traces:read")
	requireAppErr(t, err, apperr.CodeForbidden)

	foreign := f.createRole(t, other.orgID, "theirs", "traces:read")
	_, err = f.updateRole(t, admin, foreign, "mine-now", "traces:read")
	requireAppErr(t, err, apperr.CodeRoleNotFound)
	if got := f.rolePermissionNames(t, foreign); !slices.Equal(got, []string{"traces:read"}) {
		t.Fatalf("foreign role changed: %v", got)
	}
}

func (f *authFixture) accessService() *AccessService {
	return NewAccessService(f.repo, nil)
}

func (f *authFixture) updateRole(t *testing.T, caller testUser, roleID uuid.UUID, name string, permissions ...string) (*RoleResponse, error) {
	t.Helper()
	return f.accessService().UpdateRole(context.Background(), UpdateRoleRequest{
		RoleID: roleID, OrgID: caller.orgID, UserID: caller.userID,
		Name: name, PermissionIDs: f.permissionIDs(t, permissions...),
	})
}

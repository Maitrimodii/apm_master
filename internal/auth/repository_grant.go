package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// LockOrgRoles locks the org's roles with the given IDs and returns the ones
// that exist in the org. Roles of other orgs are never returned, so a caller
// can tell unknown or foreign IDs apart by comparing lengths.
//
// The rows are locked in id order in this one statement. Every transaction
// that checks or changes role permissions locks its roles this way, so they
// cannot deadlock each other. FOR NO KEY UPDATE serializes those
// transactions but does not block foreign-key inserts into user_roles and
// invite_roles (which take FOR KEY SHARE on the role).
func (r *Repository) LockOrgRoles(ctx context.Context, querier Querier, orgID uuid.UUID, roleIDs []uuid.UUID) ([]Role, error) {
	rows, err := querier.Query(ctx, `
		SELECT id, org_id, name, description, is_system, created_at, updated_at
		FROM roles
		WHERE org_id = $1 AND id = ANY($2)
		ORDER BY id
		FOR NO KEY UPDATE`, orgID, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("lock roles: %w", err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var role Role
		if err := rows.Scan(&role.ID, &role.OrgID, &role.Name, &role.Description, &role.IsSystem, &role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}
	return roles, nil
}

// GetUserRoleIDs returns the IDs of the roles a user holds in an org.
func (r *Repository) GetUserRoleIDs(ctx context.Context, querier Querier, userID, orgID uuid.UUID) ([]uuid.UUID, error) {
	return collectIDs(ctx, querier, "get user roles", `
		SELECT ur.role_id
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		WHERE ur.user_id = $1 AND r.org_id = $2`, userID, orgID)
}

// GetPermissionIDsOfRoles returns the distinct permission IDs granted to any
// of the roles.
func (r *Repository) GetPermissionIDsOfRoles(ctx context.Context, querier Querier, roleIDs []uuid.UUID) ([]uuid.UUID, error) {
	return collectIDs(ctx, querier, "get role permissions", `
		SELECT DISTINCT permission_id FROM role_permissions WHERE role_id = ANY($1)`, roleIDs)
}

// GetExistingPermissionIDs returns which of the IDs are real permissions.
func (r *Repository) GetExistingPermissionIDs(ctx context.Context, querier Querier, permissionIDs []uuid.UUID) ([]uuid.UUID, error) {
	return collectIDs(ctx, querier, "get permissions", `SELECT id FROM permissions WHERE id = ANY($1)`, permissionIDs)
}

// GetUserPermissions returns the permissions a user effectively holds in an
// org: the union over all of their roles there.
func (r *Repository) GetUserPermissions(ctx context.Context, querier Querier, userID, orgID uuid.UUID) ([]Permission, error) {
	rows, err := querier.Query(ctx, `
		SELECT DISTINCT p.id, p.name, p.action, p.description
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id
		JOIN role_permissions rp ON rp.role_id = r.id
		JOIN permissions p ON p.id = rp.permission_id
		WHERE ur.user_id = $1 AND r.org_id = $2
		ORDER BY p.name, p.action`, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("get user permissions: %w", err)
	}
	defer rows.Close()

	permissions := make([]Permission, 0)
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	return permissions, rows.Err()
}

// RecentPasswordResetExists reports whether a reset token was issued to the
// user within the cooldown, measured on the database clock.
func (r *Repository) RecentPasswordResetExists(ctx context.Context, querier Querier, userID uuid.UUID, cooldown time.Duration) (bool, error) {
	var exists bool
	err := querier.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM password_reset_tokens
			WHERE user_id = $1 AND created_at > now() - make_interval(secs => $2)
		)`, userID, cooldown.Seconds()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent password reset: %w", err)
	}
	return exists, nil
}

func collectIDs(ctx context.Context, querier Querier, what, query string, args ...any) ([]uuid.UUID, error) {
	rows, err := querier.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer rows.Close()

	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", what, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return ids, nil
}

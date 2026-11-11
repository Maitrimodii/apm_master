package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// Grant policy
//
// Having permission to manage invites or roles is not enough to hand out any
// permission: a caller may only grant permissions they effectively hold
// (the union over their roles in the org). Otherwise anyone with invites:write
// could invite an address they control as admin, and anyone with roles:update
// could add every permission to a role they hold.
//
// The policy applies to inviting with roles, creating a role and adding
// permissions to a role. Removing permissions is always allowed, even from a
// role that holds permissions the caller lacks: a reduction cannot escalate.

var (
	errUnknownRoles       = apperr.ValidationError("role_ids", "one or more roles do not exist in this organization")
	errUnknownPermissions = apperr.ValidationError("permission_ids", "one or more permissions do not exist")
)

func errGrantDenied() *apperr.Error {
	return apperr.Forbidden(apperr.CodePermissionDenied, "you cannot grant permissions you do not have")
}

// grantScope is a caller's grant authority and the target roles, read under
// the row locks described in Repository.LockOrgRoles.
type grantScope struct {
	held    map[uuid.UUID]bool
	targets map[uuid.UUID]Role
}

// lockGrantScope locks the caller's roles and targetRoleIDs (in one ordered
// statement) and returns what the caller may grant plus the target roles.
// Target IDs that are not roles of the org are an errUnknownRoles: roles of
// another org are indistinguishable from roles that do not exist.
func lockGrantScope(ctx context.Context, repo *Repository, querier Querier, callerID, orgID uuid.UUID, targetRoleIDs []uuid.UUID) (*grantScope, error) {
	callerRoleIDs, err := repo.GetUserRoleIDs(ctx, querier, callerID, orgID)
	if err != nil {
		return nil, err
	}
	locked, err := repo.LockOrgRoles(ctx, querier, orgID, uniqueIDs(append(slices.Clone(callerRoleIDs), targetRoleIDs...)))
	if err != nil {
		return nil, err
	}

	scope := &grantScope{held: map[uuid.UUID]bool{}, targets: map[uuid.UUID]Role{}}
	byID := make(map[uuid.UUID]Role, len(locked))
	for _, role := range locked {
		byID[role.ID] = role
	}
	for _, id := range targetRoleIDs {
		role, ok := byID[id]
		if !ok {
			return nil, errUnknownRoles
		}
		scope.targets[id] = role
	}

	// Read after locking, so a concurrent edit of the caller's roles is either
	// fully visible or waits for this transaction.
	held, err := repo.GetPermissionIDsOfRoles(ctx, querier, callerRoleIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range held {
		scope.held[id] = true
	}
	return scope, nil
}

// requireGrantable fails unless the caller holds every permission in ids.
func (g *grantScope) requireGrantable(ids []uuid.UUID) error {
	for _, id := range ids {
		if !g.held[id] {
			return errGrantDenied()
		}
	}
	return nil
}

// requireKnownPermissions fails with errUnknownPermissions unless every ID is
// a real permission. ids must already be deduplicated.
func requireKnownPermissions(ctx context.Context, repo *Repository, querier Querier, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	existing, err := repo.GetExistingPermissionIDs(ctx, querier, ids)
	if err != nil {
		return err
	}
	if len(existing) != len(ids) {
		return errUnknownPermissions
	}
	return nil
}

// uniqueIDs drops duplicates and uuid.Nil, keeping first occurrences.
func uniqueIDs(ids []uuid.UUID) []uuid.UUID {
	seen := make(map[uuid.UUID]bool, len(ids))
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

// difference returns the IDs in a that are not in b.
func difference(a, b []uuid.UUID) []uuid.UUID {
	inB := make(map[uuid.UUID]bool, len(b))
	for _, id := range b {
		inB[id] = true
	}
	var out []uuid.UUID
	for _, id := range a {
		if !inB[id] {
			out = append(out, id)
		}
	}
	return out
}

// appError passes application errors through and wraps anything else as an
// internal error. Transactions return the service's own *apperr.Error values
// (a refusal decided under lock) alongside database failures.
func appError(err error) error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return appErr
	}
	return apperr.Internal(err)
}

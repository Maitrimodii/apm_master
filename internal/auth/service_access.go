package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// sqlStateUniqueViolation is Postgres's unique_violation error code.
const sqlStateUniqueViolation = "23505"

type AccessService struct {
	repo *Repository
	log  *slog.Logger
}

func NewAccessService(repo *Repository, log *slog.Logger) *AccessService {
	if log == nil {
		log = slog.Default()
	}

	return &AccessService{
		repo: repo,
		log:  log,
	}
}

func (s *AccessService) GetPermissions(
	ctx context.Context,
) ([]PermissionResponse, error) {

	permissions, err := s.repo.GetPermissions(
		ctx,
		s.repo.Query(),
	)
	if err != nil {
		return nil,
			apperr.Internal(err)
	}

	resp := make(
		[]PermissionResponse,
		0,
		len(permissions),
	)

	for _, permission := range permissions {

		resp = append(
			resp,
			PermissionResponse{
				ID:          permission.ID,
				Name:        permission.Name,
				Action:      permission.Action,
				Description: permission.Description,
			},
		)
	}

	return resp, nil
}

func (s *AccessService) CreateRole(
	ctx context.Context,
	input CreateRoleRequest,
) (*RoleResponse, error) {

	permissionIDs := uniqueIDs(input.PermissionIDs)
	var CreatedRole *Role

	err := s.repo.WithTx(ctx, func(querier Querier) error {
		if err := requireKnownPermissions(ctx, s.repo, querier, permissionIDs); err != nil {
			return err
		}
		scope, err := lockGrantScope(ctx, s.repo, querier, input.UserID, input.OrgID, nil)
		if err != nil {
			return err
		}
		if err := scope.requireGrantable(permissionIDs); err != nil {
			return err
		}
		if err := s.requireNameFree(ctx, querier, input.OrgID, input.Name); err != nil {
			return err
		}

		role, err := s.repo.CreateRole(ctx, querier, Role{
			OrgID:       input.OrgID,
			Name:        input.Name,
			Description: input.Description,
			IsSystem:    false,
		})
		if err != nil {
			return err
		}
		for _, permissionID := range permissionIDs {
			if err := s.repo.AssignPermissionToRole(ctx, querier, role.ID, permissionID); err != nil {
				return err
			}
		}
		CreatedRole = role
		return nil
	})
	if err != nil {
		return nil, s.roleWriteError(err)
	}

	resp := NewRoleResponse(
		*CreatedRole,
	)

	s.log.Info("role created", "org_id", input.OrgID, "role_id", CreatedRole.ID)

	return &resp, nil
}

func (s *AccessService) GetRoles(
	ctx context.Context,
	input GetRolesRequest,
) ([]RoleResponse, error) {

	roles, err := s.repo.GetRoles(
		ctx,
		s.repo.Query(),
		input.OrgID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	resp := make(
		[]RoleResponse,
		0,
		len(roles),
	)

	for _, role := range roles {

		resp = append(
			resp,
			NewRoleResponse(role),
		)
	}

	return resp, nil
}

func (s *AccessService) GetRole(
	ctx context.Context,
	input GetRoleRequest,
) (*GetDetailedRoleResponse, error) {
	role, err := s.repo.GetRoleByID(
		ctx,
		s.repo.Query(),
		input.RoleID,
		input.OrgID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if role == nil {
		return nil, apperr.NotFound(
			apperr.CodeRoleNotFound,
			"role not found",
		)
	}

	permissions, err := s.repo.GetRolePermissions(
		ctx,
		s.repo.Query(),
		role.ID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	permissionsResp := make(
		[]PermissionResponse,
		0,
		len(permissions),
	)

	for _, permission := range permissions {
		permissionsResp = append(
			permissionsResp,
			PermissionResponse{
				ID:          permission.ID,
				Name:        permission.Name,
				Action:      permission.Action,
				Description: permission.Description,
			},
		)
	}

	resp := &GetDetailedRoleResponse{
		ID:          role.ID,
		Name:        role.Name,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Permissions: permissionsResp,
		CreatedAt:   role.CreatedAt,
		UpdatedAt:   role.UpdatedAt,
	}

	return resp, nil
}

func (s *AccessService) UpdateRole(
	ctx context.Context,
	input UpdateRoleRequest,
) (*RoleResponse, error) {

	permissionIDs := uniqueIDs(input.PermissionIDs)
	var role Role

	err := s.repo.WithTx(ctx, func(querier Querier) error {
		if err := requireKnownPermissions(ctx, s.repo, querier, permissionIDs); err != nil {
			return err
		}
		// Locking the role as a grant target also serializes this update with
		// any other change to the role or to the caller's own roles.
		scope, err := lockGrantScope(ctx, s.repo, querier, input.UserID, input.OrgID, []uuid.UUID{input.RoleID})
		if errors.Is(err, errUnknownRoles) {
			return apperr.NotFound(apperr.CodeRoleNotFound, "role not found")
		}
		if err != nil {
			return err
		}
		role = scope.targets[input.RoleID]
		if role.IsSystem {
			return apperr.Forbidden(apperr.CodeForbidden, "cannot modify system role")
		}

		// Only additions are checked: keeping or removing a permission the
		// caller lacks cannot raise anyone's access.
		current, err := s.repo.GetPermissionIDsOfRoles(ctx, querier, []uuid.UUID{role.ID})
		if err != nil {
			return err
		}
		if err := scope.requireGrantable(difference(permissionIDs, current)); err != nil {
			return err
		}
		if input.Name != role.Name {
			if err := s.requireNameFree(ctx, querier, input.OrgID, input.Name); err != nil {
				return err
			}
		}

		if err := s.repo.UpdateRole(ctx, querier, Role{
			ID:          role.ID,
			OrgID:       role.OrgID,
			Name:        input.Name,
			Description: input.Description,
			IsSystem:    role.IsSystem,
		}); err != nil {
			return err
		}
		if err := s.repo.DeleteRolePermissions(ctx, querier, role.ID); err != nil {
			return err
		}
		for _, permissionID := range permissionIDs {
			if err := s.repo.AssignPermissionToRole(ctx, querier, role.ID, permissionID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, s.roleWriteError(err)
	}

	s.log.Info("role updated", "org_id", input.OrgID, "role_id", role.ID)

	return &RoleResponse{
		ID:          role.ID,
		Name:        input.Name,
		Description: input.Description,
		IsSystem:    role.IsSystem,
	}, nil
}

// requireNameFree fails if the org already has a role with this name.
func (s *AccessService) requireNameFree(ctx context.Context, querier Querier, orgID uuid.UUID, name string) error {
	existing, err := s.repo.GetRoleByName(ctx, querier, orgID, name)
	if err != nil {
		return err
	}
	if existing != nil {
		return errRoleNameTaken()
	}
	return nil
}

func errRoleNameTaken() *apperr.Error {
	return apperr.AlreadyExists(apperr.CodeRoleAlreadyExists, "role with the same name already exists")
}

// roleWriteError maps a failed role transaction to an API error. Two
// concurrent requests can both pass requireNameFree; the loser then hits the
// UNIQUE(org_id, name) constraint, which is the same conflict.
func (s *AccessService) roleWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == sqlStateUniqueViolation && pgErr.ConstraintName == "roles_org_id_name_key" {
		return errRoleNameTaken()
	}
	return appError(err)
}

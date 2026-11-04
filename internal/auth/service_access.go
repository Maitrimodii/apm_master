package auth

import (
	"context"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

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

	existingRole, err := s.repo.GetRoleByName(
		ctx,
		s.repo.Query(),
		input.OrgID,
		input.Name,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	if existingRole != nil {
		return nil, apperr.AlreadyExists(
			apperr.CodeRoleAlreadyExists,
			"role with the same name already exists",
		)
	}

	var CreatedRole *Role

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			role, err := s.repo.CreateRole(
				ctx,
				querier,
				Role{
					OrgID:       input.OrgID,
					Name:        input.Name,
					Description: input.Description,
					IsSystem:    false,
				},
			)
			if err != nil {
				return err
			}

			for _, permissionID := range input.PermissionIDs {

				err := s.repo.AssignPermissionToRole(
					ctx,
					querier,
					role.ID,
					permissionID,
				)
				if err != nil {
					return err
				}
			}

			CreatedRole = role

			return nil
		},
	)

	if err != nil {
		return nil, apperr.Internal(err)
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

	if role.IsSystem {
		return nil, apperr.Forbidden(
			apperr.CodeForbidden,
			"cannot modify system role",
		)
	}

	if input.Name != role.Name {
		existingRole, err := s.repo.GetRoleByName(
			ctx,
			s.repo.Query(),
			input.OrgID,
			input.Name,
		)
		if err != nil {
			return nil, apperr.Internal(err)
		}

		if existingRole != nil {
			return nil, apperr.AlreadyExists(
				apperr.CodeRoleAlreadyExists,
				"role with the same name already exists",
			)
		}
	}

	err = s.repo.WithTx(
		ctx,
		func(querier Querier) error {

			if err := s.repo.UpdateRole(
				ctx,
				querier,
				Role{
					ID:          role.ID,
					OrgID:       role.OrgID,
					Name:        input.Name,
					Description: input.Description,
					IsSystem:    role.IsSystem,
				},
			); err != nil {
				return err
			}

			if input.PermissionIDs != nil {

				if err := s.repo.DeleteRolePermissions(
					ctx,
					querier,
					role.ID,
				); err != nil {
					return err
				}
			}

			for _, permissionID := range input.PermissionIDs {

				err := s.repo.AssignPermissionToRole(
					ctx,
					querier,
					role.ID,
					permissionID,
				)
				if err != nil {
					return err
				}
			}
			return nil
		},
	)

	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("role updated", "org_id", input.OrgID, "role_id", role.ID)

	return &RoleResponse{
		ID:          role.ID,
		Name:        input.Name,
		Description: input.Description,
		IsSystem:    role.IsSystem,
	}, nil
}

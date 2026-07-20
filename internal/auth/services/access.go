package services

import (
	"context"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/auth/dto"
	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/auth/repositories"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/redis/go-redis/v9"
)

type AccessService struct {
	repo *repositories.Repository
	rdb  *redis.Client
	log  *slog.Logger
}

func NewAccessService(repo *repositories.Repository, rdb *redis.Client, log *slog.Logger) *AccessService {
	if log == nil {
		log = slog.Default()
	}

	return &AccessService{
		repo: repo,
		rdb:  rdb,
		log:  log,
	}
}
func (service *AccessService) GetPermissions(
	ctx context.Context,
) ([]dto.PermissionResponse, error) {

	permissions, err := service.repo.GetPermissions(
		ctx,
		service.repo.Query(),
	)
	if err != nil {
		return nil,
			server.Internal(err)
	}

	resp := make(
		[]dto.PermissionResponse,
		0,
		len(permissions),
	)

	for _, permission := range permissions {

		resp = append(
			resp,
			dto.PermissionResponse{
				ID:          permission.ID,
				Name:        permission.Name,
				Action:      permission.Action,
				Description: permission.Description,
			},
		)
	}

	return resp, nil
}

func (service *AccessService) CreateRole(
	ctx context.Context,
	input dto.CreateRoleRequest,
) (*dto.RoleResponse, error) {

	existingRole, err := service.repo.GetRoleByName(
		ctx,
		service.repo.Query(),
		input.OrgID,
		input.Name,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if existingRole != nil {
		return nil, server.AlreadyExists(
			server.CodeRoleAlreadyExists,
			"role with the same name already exists",
		)
	}

	var CreatedRole *models.Role

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			role, err := service.repo.CreateRole(
				ctx,
				querier,
				models.Role{
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

				err := service.repo.AssignPermissionToRole(
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
		return nil, server.Internal(err)
	}

	resp := dto.NewRoleResponse(
		*CreatedRole,
	)

	service.log.Info("role created", "org_id", input.OrgID, "role_id", CreatedRole.ID)

	return &resp, nil
}

func (service *AccessService) GetRoles(
	ctx context.Context,
	input dto.GetRolesRequest,
) ([]dto.RoleResponse, error) {

	roles, err := service.repo.GetRoles(
		ctx,
		service.repo.Query(),
		input.OrgID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	resp := make(
		[]dto.RoleResponse,
		0,
		len(roles),
	)

	for _, role := range roles {

		resp = append(
			resp,
			dto.NewRoleResponse(role),
		)
	}

	return resp, nil
}

func (service *AccessService) GetRole(
	ctx context.Context,
	input dto.GetRoleRequest,
) (*dto.GetDetailedRoleResponse, error) {
	role, err := service.repo.GetRoleByID(
		ctx,
		service.repo.Query(),
		input.RoleID,
		input.OrgID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if role == nil {
		return nil, server.NotFound(
			server.CodeRoleNotFound,
			"role not found",
		)
	}

	permissions, err := service.repo.GetRolePermissions(
		ctx,
		service.repo.Query(),
		role.ID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	permissionsResp := make(
		[]dto.PermissionResponse,
		0,
		len(permissions),
	)

	for _, permission := range permissions {
		permissionsResp = append(
			permissionsResp,
			dto.PermissionResponse{
				ID:          permission.ID,
				Name:        permission.Name,
				Action:      permission.Action,
				Description: permission.Description,
			},
		)
	}

	resp := &dto.GetDetailedRoleResponse{
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

func (service *AccessService) UpdateRole(
	ctx context.Context,
	input dto.UpdateRoleRequest,
) (*dto.RoleResponse, error) {

	role, err := service.repo.GetRoleByID(
		ctx,
		service.repo.Query(),
		input.RoleID,
		input.OrgID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	if role == nil {
		return nil, server.NotFound(
			server.CodeRoleNotFound,
			"role not found",
		)
	}

	if role.IsSystem {
		return nil, server.Forbidden(
			server.CodeForbidden,
			"cannot modify system role",
		)
	}

	if input.Name != role.Name {
		existingRole, err := service.repo.GetRoleByName(
			ctx,
			service.repo.Query(),
			input.OrgID,
			input.Name,
		)
		if err != nil {
			return nil, server.Internal(err)
		}

		if existingRole != nil {
			return nil, server.AlreadyExists(
				server.CodeRoleAlreadyExists,
				"role with the same name already exists",
			)
		}
	}

	err = service.repo.WithTx(
		ctx,
		func(querier repositories.Querier) error {

			if err := service.repo.UpdateRole(
				ctx,
				querier,
				models.Role{
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

				if err := service.repo.DeleteRolePermissions(
					ctx,
					querier,
					role.ID,
				); err != nil {
					return err
				}
			}

			for _, permissionID := range input.PermissionIDs {

				err := service.repo.AssignPermissionToRole(
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
		return nil, server.Internal(err)
	}

	service.log.Info("role updated", "org_id", input.OrgID, "role_id", role.ID)

	return &dto.RoleResponse{
		ID:          role.ID,
		Name:        input.Name,
		Description: input.Description,
		IsSystem:    role.IsSystem,
	}, nil
}

func (service *AccessService) CreateAPIKey(
	ctx context.Context,
	input dto.CreateAPIKeyRequest,
) (*dto.CreateAPIKeyResponse, error) {
	rawKey, prefix, err := GenerateAPIKey()
	if err != nil {
		return nil, server.Internal(err)
	}

	hashedKey := HashToken(rawKey)

	apiKey, err := service.repo.CreateAPIKey(
		ctx,
		service.repo.Query(),
		models.APIKey{
			OrgID:     input.OrgID,
			Name:      input.Name,
			KeyHash:   hashedKey,
			KeyPrefix: prefix,
		},
	)

	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("api key created", "org_id", input.OrgID, "api_key_id", apiKey.ID)

	return &dto.CreateAPIKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       rawKey,
		Prefix:    apiKey.KeyPrefix,
		CreatedAt: apiKey.CreatedAt,
	}, nil
}

func (service *AccessService) GetAPIKeys(
	ctx context.Context,
	input dto.GetAPIKeysRequest,
) ([]dto.APIKeyResponse, error) {
	apiKeys, err := service.repo.GetAPIKeys(
		ctx,
		service.repo.Query(),
		input.OrgID,
	)
	if err != nil {
		return nil, server.Internal(err)
	}

	resp := make(
		[]dto.APIKeyResponse,
		0,
		len(apiKeys),
	)

	for _, apiKey := range apiKeys {
		resp = append(
			resp,
			dto.APIKeyResponse{
				ID:         apiKey.ID,
				Name:       apiKey.Name,
				KeyPrefix:  apiKey.KeyPrefix,
				LastUsedAt: apiKey.LastUsedAt,
				RevokedAt:  apiKey.RevokedAt,
				CreatedAt:  apiKey.CreatedAt,
			},
		)
	}

	return resp, nil
}

func (service *AccessService) RevokeAPIKey(
	ctx context.Context,
	input dto.RevokeAPIKeyRequest,
) error {

	apiKey, err := service.repo.GetAPIKeyByID(
		ctx,
		service.repo.Query(),
		input.APIKeyID,
		input.OrgID,
	)
	if err != nil {
		return server.Internal(err)
	}

	if apiKey == nil {
		return server.NotFound(
			server.CodeNotFound,
			"api key not found",
		)
	}

	if apiKey.RevokedAt != nil {
		return server.BadRequest(
			server.CodeValidation,
			"api key already revoked",
		)
	}

	err = service.repo.RevokeAPIKey(
		ctx,
		service.repo.Query(),
		apiKey.ID,
	)
	if err != nil {
		return server.Internal(err)
	}

	// Proactively invalidate cached API key
	cacheKey := "apikey:org:" + apiKey.KeyHash
	if err := service.rdb.Del(ctx, cacheKey).Err(); err != nil {
		service.log.Warn("failed to invalidate api key cache on revocation", "api_key_id", apiKey.ID, "error", err)
	}

	service.log.Info("api key revoked", "org_id", input.OrgID, "api_key_id", apiKey.ID)

	return nil
}

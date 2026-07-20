package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *Repository) CreatePermission(ctx context.Context, querier Querier, permission models.Permission) (*models.Permission, error) {
	query := `INSERT INTO permissions (name, action, description) VALUES ($1, $2, $3) RETURNING id`
	err := querier.QueryRow(ctx, query, permission.Name, permission.Action, permission.Description).Scan(&permission.ID)
	if err != nil {
		return nil, fmt.Errorf(
			"create permission: %w",
			err,
		)
	}
	return &permission, nil
}

func (repo *Repository) GetPermission(ctx context.Context, querier Querier, name string, action models.PermissionAction) (*models.Permission, error) {
	query := `SELECT id, name, action, description FROM permissions WHERE name = $1 and action = $2`
	var permission models.Permission
	err := querier.QueryRow(ctx, query, name, action).Scan(
		&permission.ID,
		&permission.Name,
		&permission.Action,
		&permission.Description,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"get permission by name: %w",
			err,
		)
	}
	return &permission, nil
}

func (repo *Repository) GetPermissions(
	ctx context.Context,
	querier Querier,
) ([]models.Permission, error) {

	query := `
		SELECT
			id,
			name,
			action,
			description
		FROM permissions
		ORDER BY name, action
	`

	rows, err := querier.Query(
		ctx,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get permissions: %w",
			err,
		)
	}
	defer rows.Close()

	permissions := make(
		[]models.Permission,
		0,
	)

	for rows.Next() {
		var permission models.Permission

		err := rows.Scan(
			&permission.ID,
			&permission.Name,
			&permission.Action,
			&permission.Description,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"scan permission: %w",
				err,
			)
		}

		permissions = append(
			permissions,
			permission,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate permissions: %w",
			err,
		)
	}

	return permissions, nil
}

func (repo *Repository) GetPermissionByID(ctx context.Context, querier Querier, identifier uuid.UUID) (*models.Permission, error) {
	query := `SELECT id, name, action, description FROM permissions WHERE id = $1`
	var permission models.Permission
	err := querier.QueryRow(ctx, query, identifier).Scan(
		&permission.ID,
		&permission.Name,
		&permission.Action,
		&permission.Description,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"get permission by id: %w",
			err,
		)
	}
	return &permission, nil
}

func (repo *Repository) AssignPermissionToRole(ctx context.Context, querier Querier, roleID, permissionID uuid.UUID) error {
	query := `INSERT INTO role_permissions (role_id, permission_id) VALUES ($1, $2) on conflict do nothing`
	_, err := querier.Exec(ctx, query, roleID, permissionID)
	if err != nil {
		return fmt.Errorf(
			"assign permission to role: %w",
			err,
		)
	}
	return nil
}

func (repo *Repository) GetRolePermissions(ctx context.Context, querier Querier, roleID uuid.UUID) ([]models.Permission, error) {
	query := `
		SELECT p.id, p.name, p.action, p.description
		FROM permissions p
		JOIN role_permissions rp ON p.id = rp.permission_id
		WHERE rp.role_id = $1
	`

	rows, err := querier.Query(ctx, query, roleID)
	if err != nil {
		return nil, fmt.Errorf(
			"get role permissions: %w",
			err,
		)
	}
	defer rows.Close()

	permissions := make([]models.Permission, 0)

	for rows.Next() {
		var permission models.Permission
		if err := rows.Scan(&permission.ID, &permission.Name, &permission.Action, &permission.Description); err != nil {
			return nil, fmt.Errorf(
				"scan permission: %w",
				err,
			)
		}
		permissions = append(permissions, permission)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate permissions: %w",
			err,
		)
	}

	return permissions, nil
}

func (repo *Repository) GetRoles(ctx context.Context, querier Querier, orgID uuid.UUID) ([]models.Role, error) {
	query := `
		SELECT id, org_id, name, description, is_system, created_at, updated_at
		FROM roles
		WHERE org_id = $1
		ORDER BY name
	`
	rows, err := querier.Query(ctx, query, orgID)
	if err != nil {
		return nil, fmt.Errorf("get roles: %w", err)
	}
	defer rows.Close()

	roles := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
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

func (repo *Repository) UpdateRole(ctx context.Context, querier Querier, role models.Role) error {
	query := `
		UPDATE roles
		SET name = $1, description = $2, updated_at = NOW()
		WHERE id = $3 AND org_id = $4
	`
	_, err := querier.Exec(ctx, query, role.Name, role.Description, role.ID, role.OrgID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

func (repo *Repository) DeleteRolePermissions(ctx context.Context, querier Querier, roleID uuid.UUID) error {
	query := `DELETE FROM role_permissions WHERE role_id = $1`
	_, err := querier.Exec(ctx, query, roleID)
	if err != nil {
		return fmt.Errorf("delete role permissions: %w", err)
	}
	return nil
}

func (repo *Repository) CreateRefreshToken(ctx context.Context, querier Querier, token models.RefreshToken) (*models.RefreshToken, error) {
	query := `
	INSERT INTO refresh_tokens (
		user_id,
		token_hash,
		user_agent,
		ip_address,
		device_name,
		expires_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
	RETURNING
		id,
		created_at,
		updated_at
	`
	err := querier.QueryRow(ctx, query, token.UserID, token.TokenHash, token.UserAgent, token.IPAddress, token.DeviceName, token.ExpiresAt).Scan(&token.ID, &token.CreatedAt, &token.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf(
			"create refresh token: %w",
			err,
		)
	}
	return &token, nil
}

func (repo *Repository) GetRefreshTokenByHash(ctx context.Context, querier Querier, tokenHash string) (*models.RefreshToken, error) {
	query := `SELECT id, user_id, token_hash, user_agent, ip_address, device_name, expires_at, last_used_at, created_at, revoked_at, updated_at FROM refresh_tokens WHERE token_hash = $1`
	var token models.RefreshToken
	err := querier.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.UserAgent,
		&token.IPAddress,
		&token.DeviceName,
		&token.ExpiresAt,
		&token.LastUsedAt,
		&token.CreatedAt,
		&token.RevokedAt,
		&token.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"get refresh token by hash: %w",
			err,
		)
	}
	return &token, nil
}

func (repo *Repository) RevokeRefreshToken(ctx context.Context, querier Querier, tokenID uuid.UUID) error {
	query := `UPDATE refresh_tokens SET revoked_at = Now(), updated_at = NOW() WHERE id = $1`
	_, err := querier.Exec(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf(
			"revoke refresh token: %w",
			err,
		)
	}
	return nil
}

func (repo *Repository) DeleteExpiredRefreshTokens(ctx context.Context, querier Querier, before time.Time) error {
	query := `DELETE FROM refresh_tokens WHERE expires_at < $1 OR (revoked_at IS NOT NULL AND revoked_at < $1)`
	_, err := querier.Exec(ctx, query, before)
	if err != nil {
		return fmt.Errorf(
			"delete expired refresh tokens: %w",
			err,
		)
	}
	return nil
}

func (repo *Repository) CreateAPIKey(ctx context.Context, querier Querier, apiKey models.APIKey) (*models.APIKey, error) {
	query := `INSERT INTO api_keys (org_id, name, key_prefix, key_hash) VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at`
	err := querier.QueryRow(ctx, query, apiKey.OrgID, apiKey.Name, apiKey.KeyPrefix, apiKey.KeyHash).Scan(&apiKey.ID, &apiKey.CreatedAt, &apiKey.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf(
			"create API key: %w",
			err,
		)
	}
	return &apiKey, nil
}

func (repo *Repository) GetAPIKeys(
	ctx context.Context,
	querier Querier,
	orgID uuid.UUID,
) ([]models.APIKey, error) {

	query := `
		SELECT
			id,
			name,
			key_prefix,
			last_used_at,
			revoked_at,
			created_at
		FROM api_keys
		WHERE org_id = $1
		ORDER BY created_at DESC
	`

	rows, err := querier.Query(
		ctx,
		query,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get API keys: %w",
			err,
		)
	}
	defer rows.Close()

	apiKeys := make([]models.APIKey, 0)

	for rows.Next() {
		var apiKey models.APIKey

		if err := rows.Scan(
			&apiKey.ID,
			&apiKey.Name,
			&apiKey.KeyPrefix,
			&apiKey.LastUsedAt,
			&apiKey.RevokedAt,
			&apiKey.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf(
				"scan API key: %w",
				err,
			)
		}

		apiKeys = append(
			apiKeys,
			apiKey,
		)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate API keys: %w",
			err,
		)
	}

	return apiKeys, nil
}

func (repo *Repository) GetAPIKeyByHash(ctx context.Context, querier Querier, keyHash string) (*models.APIKey, error) {
	query := `SELECT id, org_id, name, key_prefix, key_hash, created_at, last_used_at, revoked_at, updated_at FROM api_keys WHERE key_hash = $1`
	var apiKey models.APIKey
	err := querier.QueryRow(ctx, query, keyHash).Scan(
		&apiKey.ID,
		&apiKey.OrgID,
		&apiKey.Name,
		&apiKey.KeyPrefix,
		&apiKey.KeyHash,
		&apiKey.CreatedAt,
		&apiKey.LastUsedAt,
		&apiKey.RevokedAt,
		&apiKey.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"get API key by hash: %w",
			err,
		)
	}
	return &apiKey, nil
}

func (repo *Repository) UpdateAPIKeyLastUsed(ctx context.Context, querier Querier, apiKeyID uuid.UUID) error {
	query := `UPDATE api_keys SET last_used_at = Now(), updated_at = Now() WHERE id = $1`
	_, err := querier.Exec(ctx, query, apiKeyID)
	if err != nil {
		return fmt.Errorf(
			"update API key last used: %w",
			err,
		)
	}
	return nil
}

func (repo *Repository) RevokeAPIKey(ctx context.Context, querier Querier, apiKeyID uuid.UUID) error {
	query := `UPDATE api_keys SET revoked_at = Now(), updated_at = Now() WHERE id = $1`
	_, err := querier.Exec(ctx, query, apiKeyID)
	if err != nil {
		return fmt.Errorf(
			"revoke API key: %w",
			err,
		)
	}
	return nil
}

func (repo *Repository) GetAPIKeyByID(ctx context.Context, querier Querier, apiKeyID uuid.UUID, orgID uuid.UUID) (*models.APIKey, error) {
	query := `SELECT id, org_id, name, key_prefix, key_hash, created_at, last_used_at, revoked_at, updated_at FROM api_keys WHERE id = $1 AND org_id = $2 LIMIT 1`
	var apiKey models.APIKey
	err := querier.QueryRow(ctx, query, apiKeyID, orgID).Scan(
		&apiKey.ID,
		&apiKey.OrgID,
		&apiKey.Name,
		&apiKey.KeyPrefix,
		&apiKey.KeyHash,
		&apiKey.CreatedAt,
		&apiKey.LastUsedAt,
		&apiKey.RevokedAt,
		&apiKey.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf(
			"get API key by ID: %w",
			err,
		)
	}
	return &apiKey, nil
}

func (repo *Repository) HasPermission(
	ctx context.Context,
	querier Querier,
	userID, orgID uuid.UUID,
	name string,
	action models.PermissionAction,
) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM user_roles ur
			JOIN roles r ON ur.role_id = r.id
			JOIN role_permissions rp ON r.id = rp.role_id
			JOIN permissions p ON rp.permission_id = p.id
			WHERE ur.user_id = $1
			  AND r.org_id = $2
			  AND p.name = $3
			  AND p.action = $4
		)
	`
	var has bool
	err := querier.QueryRow(ctx, query, userID, orgID, name, action).Scan(&has)
	if err != nil {
		return false, fmt.Errorf("check permission mapping: %w", err)
	}
	return has, nil
}

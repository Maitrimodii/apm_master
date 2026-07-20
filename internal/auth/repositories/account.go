package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *Repository) CreateOrg(ctx context.Context, querier Querier, org models.Org) (*models.Org, error) {
	query := `INSERT INTO orgs (name, slug) VALUES ($1, $2) RETURNING id, created_at, updated_at`
	err := querier.QueryRow(ctx, query, org.Name, org.Slug).Scan(&org.ID, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf(
			"create org: %w",
			err,
		)
	}
	return &org, nil
}

func (repo *Repository) CreateUser(ctx context.Context, querier Querier, user models.User) (*models.User, error) {
	query := `INSERT INTO users 
	(org_id, 
	is_email_verified, 
	first_name, 
	last_name, 
	email, 
	password_hash) 
	VALUES ($1, $2, $3, $4, $5, $6) 
	RETURNING id, created_at, updated_at`

	err := querier.QueryRow(
		ctx,
		query,
		user.OrgID,
		user.IsEmailVerified,
		user.FirstName,
		user.LastName,
		user.Email,
		user.PasswordHash,
	).Scan(
		&user.ID,
		&user.CreatedAt,
		&user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf(
			"Create user %w",
			err,
		)
	}
	return &user, nil
}

func (repo *Repository) GetUserByEmail(ctx context.Context, querier Querier, orgID uuid.UUID, email string) (*models.User, error) {
	query := `SELECT id, org_id, is_email_verified, first_name, last_name, email, password_hash, created_at, updated_at FROM users WHERE org_id = $1 AND email = $2`
	var user models.User
	err := querier.QueryRow(ctx, query, orgID, email).Scan(
		&user.ID,
		&user.OrgID,
		&user.IsEmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get user by email: %w",
			err,
		)
	}
	return &user, nil
}

func (repo *Repository) GetUserByID(ctx context.Context, querier Querier, userID uuid.UUID) (*models.User, error) {
	query := `SELECT id, org_id, is_email_verified, first_name, last_name, email, password_hash, created_at, updated_at FROM users WHERE id = $1`
	var user models.User
	err := querier.QueryRow(ctx, query, userID).Scan(
		&user.ID,
		&user.OrgID,
		&user.IsEmailVerified,
		&user.FirstName,
		&user.LastName,
		&user.Email,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get user by ID: %w",
			err,
		)
	}
	return &user, nil
}

func (repo *Repository) UpdateUserPassword(ctx context.Context, querier Querier, userID uuid.UUID, passwordHash string) error {
	query := `UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`
	_, err := querier.Exec(ctx, query, passwordHash, userID)
	if err != nil {
		return fmt.Errorf("update user password: %w", err)
	}
	return nil
}

func (repo *Repository) CreateRole(
	ctx context.Context,
	querier Querier,
	role models.Role,
) (*models.Role, error) {
	query := `INSERT INTO roles (org_id, name, description, is_system) VALUES ($1, $2, $3, $4) RETURNING id, created_at, updated_at`
	err := querier.QueryRow(ctx, query, role.OrgID, role.Name, role.Description, role.IsSystem).Scan(&role.ID, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf(
			"create role: %w",
			err,
		)
	}
	return &role, nil
}

func (repo *Repository) GetRoleByName(ctx context.Context, querier Querier, orgID uuid.UUID, name string) (*models.Role, error) {
	query := `SELECT id, org_id, name, description, is_system, created_at, updated_at FROM roles WHERE org_id = $1 AND name = $2`
	var role models.Role
	err := querier.QueryRow(ctx, query, orgID, name).Scan(
		&role.ID,
		&role.OrgID,
		&role.Name,
		&role.Description,
		&role.IsSystem,
		&role.CreatedAt,
		&role.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get role by name: %w",
			err,
		)
	}
	return &role, nil
}

func (repo *Repository) GetRoleByID(ctx context.Context, querier Querier, roleID uuid.UUID, orgID uuid.UUID) (*models.Role, error) {
	query := `SELECT id, org_id, name, description, is_system, created_at, updated_at FROM roles WHERE id = $1 AND org_id = $2`
	var role models.Role
	err := querier.QueryRow(ctx, query, roleID, orgID).Scan(
		&role.ID,
		&role.OrgID,
		&role.Name,
		&role.Description,
		&role.IsSystem,
		&role.CreatedAt,
		&role.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get role by ID: %w",
			err,
		)
	}
	return &role, nil
}

func (repo *Repository) AssignRoleToUser(ctx context.Context, querier Querier, userID, roleID uuid.UUID) error {
	query := `INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := querier.Exec(ctx, query, userID, roleID)
	if err != nil {
		return fmt.Errorf("assign role to user: %w", err)
	}
	return nil
}

func (repo *Repository) GetOrgBySlug(
	ctx context.Context,
	querier Querier,
	slug string,
) (*models.Org, error) {

	query := `
		SELECT
			id,
			name,
			slug,
			created_at,
			updated_at
		FROM orgs
		WHERE slug = $1
	`

	var org models.Org

	err := querier.QueryRow(
		ctx,
		query,
		slug,
	).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.CreatedAt,
		&org.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get org by slug: %w",
			err,
		)
	}

	return &org, nil
}

func (repo *Repository) GetOrgByID(ctx context.Context, querier Querier, orgID uuid.UUID) (*models.Org, error) {
	query := `
		SELECT id, name, slug, created_at, updated_at
		FROM orgs
		WHERE id = $1
	`

	var org models.Org
	err := querier.QueryRow(ctx, query, orgID).Scan(
		&org.ID,
		&org.Name,
		&org.Slug,
		&org.CreatedAt,
		&org.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get org by ID: %w", err)
	}

	return &org, nil
}

func (repo *Repository) GetUserRoles(
	ctx context.Context,
	querier Querier,
	userID uuid.UUID,
) ([]models.Role, error) {
	query := `SELECT r.id, r.org_id, r.name, r.description, r.is_system, r.created_at, r.updated_at
	FROM roles r
	JOIN user_roles ur ON ur.role_id = r.id
	WHERE ur.user_id = $1`
	var roles []models.Role
	rows, err := querier.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("get user roles: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role models.Role
		err := rows.Scan(
			&role.ID,
			&role.OrgID,
			&role.Name,
			&role.Description,
			&role.IsSystem,
			&role.CreatedAt,
			&role.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("get user roles: %w", err)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

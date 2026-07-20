package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *Repository) CreateInvite(
	ctx context.Context,
	querier Querier,
	invite models.Invite,
) (*models.Invite, error) {
	query := `
	INSERT INTO invites (
		org_id,
		invited_by,
		first_name,
		last_name,
		token_hash,
		email,
		expires_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	RETURNING id, created_at
	`

	err := querier.QueryRow(
		ctx,
		query,
		invite.OrgID,
		invite.InvitedBy,
		invite.FirstName,
		invite.LastName,
		invite.TokenHash,
		invite.Email,
		invite.ExpiresAt,
	).Scan(
		&invite.ID,
		&invite.CreatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create invite: %w",
			err,
		)
	}

	return &invite, nil
}

func (repo *Repository) GetPendingInviteByEmail(
	ctx context.Context,
	querier Querier,
	email string,
	orgID uuid.UUID,
) (*models.Invite, error) {
	query := `
		SELECT
			id,
			org_id,
			invited_by,
			first_name,
			last_name,
			token_hash,
			email,
			expires_at,
			accepted_at,
			created_at
		FROM invites
		WHERE org_id = $1 AND email = $2 AND accepted_at IS NULL AND expires_at > NOW() LIMIT 1
	`

	var invite models.Invite

	err := querier.QueryRow(
		ctx,
		query,
		orgID,
		email,
	).Scan(
		&invite.ID,
		&invite.OrgID,
		&invite.InvitedBy,
		&invite.FirstName,
		&invite.LastName,
		&invite.TokenHash,
		&invite.Email,
		&invite.ExpiresAt,
		&invite.AcceptedAt,
		&invite.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get pending invite by email: %w",
			err,
		)
	}

	return &invite, nil
}

func (repo *Repository) GetInviteByTokenHash(
	ctx context.Context,
	querier Querier,
	tokenHash string,
) (*models.Invite, error) {
	query := `
		SELECT
			id,
			org_id,
			invited_by,
			first_name,
			last_name,
			token_hash,
			email,
			expires_at,
			accepted_at,
			created_at
		FROM invites
		WHERE token_hash = $1 LIMIT 1
	`

	var invite models.Invite

	err := querier.QueryRow(
		ctx,
		query,
		tokenHash,
	).Scan(
		&invite.ID,
		&invite.OrgID,
		&invite.InvitedBy,
		&invite.FirstName,
		&invite.LastName,
		&invite.TokenHash,
		&invite.Email,
		&invite.ExpiresAt,
		&invite.AcceptedAt,
		&invite.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf(
			"get invite by token hash: %w",
			err,
		)
	}

	return &invite, nil
}

func (repo *Repository) AssignInviteRole(
	ctx context.Context,
	querier Querier,
	inviteRole models.InviteRole,
) error {
	query := `
		INSERT INTO invite_roles (
			invite_id,
			role_id
		)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`

	_, err := querier.Exec(
		ctx,
		query,
		inviteRole.InviteID,
		inviteRole.RoleID,
	)

	if err != nil {
		return fmt.Errorf(
			"assign invite role: %w",
			err,
		)
	}

	return nil
}

func (repo *Repository) GetInviteRoles(
	ctx context.Context,
	querier Querier,
	inviteID uuid.UUID,
) ([]uuid.UUID, error) {
	query := `
		SELECT role_id
		FROM invite_roles
		WHERE invite_id = $1
	`

	rows, err := querier.Query(
		ctx,
		query,
		inviteID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get invite roles: %w",
			err,
		)
	}
	defer rows.Close()

	roleIDs := make([]uuid.UUID, 0)

	for rows.Next() {
		var roleID uuid.UUID

		if err := rows.Scan(&roleID); err != nil {
			return nil, fmt.Errorf(
				"scan invite role: %w",
				err,
			)
		}

		roleIDs = append(roleIDs, roleID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate invite roles: %w",
			err,
		)
	}

	return roleIDs, nil
}

func (repo *Repository) MarkInviteAccepted(
	ctx context.Context,
	querier Querier,
	inviteID uuid.UUID,
) error {
	query := `
		UPDATE invites
		SET accepted_at = NOW()
		WHERE id = $1
	`

	_, err := querier.Exec(
		ctx,
		query,
		inviteID,
	)
	if err != nil {
		return fmt.Errorf(
			"mark invite accepted: %w",
			err,
		)
	}

	return nil
}

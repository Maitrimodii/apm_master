package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreateInvite(
	ctx context.Context,
	querier Querier,
	invite Invite,
) (*Invite, error) {
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

// inviteColumns is the column list scanInvite reads, in order.
const inviteColumns = `id, org_id, invited_by, first_name, last_name, token_hash, email,
	expires_at, accepted_at, revoked_at, created_at`

// scanInvite scans one inviteColumns row; no row is (nil, nil).
func scanInvite(row pgx.Row) (*Invite, error) {
	var invite Invite
	err := row.Scan(
		&invite.ID,
		&invite.OrgID,
		&invite.InvitedBy,
		&invite.FirstName,
		&invite.LastName,
		&invite.TokenHash,
		&invite.Email,
		&invite.ExpiresAt,
		&invite.AcceptedAt,
		&invite.RevokedAt,
		&invite.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

// GetPendingInviteByEmail returns a usable (not accepted, revoked or expired)
// invite for email in the org.
func (r *Repository) GetPendingInviteByEmail(
	ctx context.Context,
	querier Querier,
	email string,
	orgID uuid.UUID,
) (*Invite, error) {
	query := `SELECT ` + inviteColumns + ` FROM invites
		WHERE org_id = $1 AND email = $2
		  AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
		LIMIT 1`
	invite, err := scanInvite(querier.QueryRow(ctx, query, orgID, email))
	if err != nil {
		return nil, fmt.Errorf("get pending invite by email: %w", err)
	}
	return invite, nil
}

func (r *Repository) GetInviteByTokenHash(
	ctx context.Context,
	querier Querier,
	tokenHash string,
) (*Invite, error) {
	query := `SELECT ` + inviteColumns + ` FROM invites WHERE token_hash = $1`
	invite, err := scanInvite(querier.QueryRow(ctx, query, tokenHash))
	if err != nil {
		return nil, fmt.Errorf("get invite by token hash: %w", err)
	}
	return invite, nil
}

// ConsumeInvite marks the invite with tokenHash accepted if it is still
// usable and returns it; otherwise it returns nil. As a single conditional
// UPDATE, only one of several concurrent acceptances gets the row: the others
// wait on its row lock and then match nothing.
func (r *Repository) ConsumeInvite(ctx context.Context, querier Querier, tokenHash string) (*Invite, error) {
	query := `
		UPDATE invites
		SET accepted_at = now()
		WHERE token_hash = $1
		  AND accepted_at IS NULL
		  AND revoked_at IS NULL
		  AND expires_at > now()
		RETURNING ` + inviteColumns
	invite, err := scanInvite(querier.QueryRow(ctx, query, tokenHash))
	if err != nil {
		return nil, fmt.Errorf("consume invite: %w", err)
	}
	return invite, nil
}

// LockInvite loads an invite of the org and locks its row until the
// transaction ends; an invite of another org is (nil, nil).
func (r *Repository) LockInvite(ctx context.Context, querier Querier, orgID, inviteID uuid.UUID) (*Invite, error) {
	query := `SELECT ` + inviteColumns + ` FROM invites WHERE id = $1 AND org_id = $2 FOR UPDATE`
	invite, err := scanInvite(querier.QueryRow(ctx, query, inviteID, orgID))
	if err != nil {
		return nil, fmt.Errorf("lock invite: %w", err)
	}
	return invite, nil
}

// ReissueInvite gives an invite a new token and expiry, so the previous link
// stops working.
func (r *Repository) ReissueInvite(ctx context.Context, querier Querier, inviteID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := querier.Exec(ctx,
		`UPDATE invites SET token_hash = $2, expires_at = $3 WHERE id = $1`,
		inviteID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("reissue invite: %w", err)
	}
	return nil
}

// RevokeInvite cancels an invite so its link stops working.
func (r *Repository) RevokeInvite(ctx context.Context, querier Querier, inviteID uuid.UUID) error {
	_, err := querier.Exec(ctx, `UPDATE invites SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, inviteID)
	if err != nil {
		return fmt.Errorf("revoke invite: %w", err)
	}
	return nil
}

// InviteListItem is an invite as listed for an org's administrators. It
// deliberately has no token.
type InviteListItem struct {
	Invite
	Status        InviteStatus
	InvitedByName string
	Roles         []RoleSummary
}

// ListInvites returns an org's invites, newest first, optionally only those
// in one status, with the total matching count.
func (r *Repository) ListInvites(ctx context.Context, querier Querier, orgID uuid.UUID, status InviteStatus, limit, offset int) ([]InviteListItem, int, error) {
	// status_of mirrors InviteStatus; the order of the checks matters (an
	// accepted invite is accepted even after its expiry).
	const base = `
		WITH listed AS (
			SELECT i.*, u.first_name || ' ' || u.last_name AS invited_by_name,
			       CASE
			           WHEN i.accepted_at IS NOT NULL THEN 'accepted'
			           WHEN i.revoked_at IS NOT NULL THEN 'revoked'
			           WHEN i.expires_at <= now() THEN 'expired'
			           ELSE 'pending'
			       END AS status
			FROM invites i
			JOIN users u ON u.id = i.invited_by
			WHERE i.org_id = $1
		)`
	filter := ` WHERE ($2 = '' OR status = $2)`

	var total int
	if err := querier.QueryRow(ctx, base+` SELECT count(*) FROM listed`+filter, orgID, string(status)).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count invites: %w", err)
	}

	rows, err := querier.Query(ctx, base+`
		SELECT `+inviteColumns+`, status, invited_by_name
		FROM listed`+filter+`
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4`, orgID, string(status), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	var items []InviteListItem
	for rows.Next() {
		var item InviteListItem
		inv := &item.Invite
		if err := rows.Scan(
			&inv.ID, &inv.OrgID, &inv.InvitedBy, &inv.FirstName, &inv.LastName, &inv.TokenHash, &inv.Email,
			&inv.ExpiresAt, &inv.AcceptedAt, &inv.RevokedAt, &inv.CreatedAt,
			&item.Status, &item.InvitedByName,
		); err != nil {
			return nil, 0, fmt.Errorf("scan invite: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate invites: %w", err)
	}

	for i := range items {
		roles, err := r.GetInviteRoleSummaries(ctx, querier, items[i].ID)
		if err != nil {
			return nil, 0, err
		}
		items[i].Roles = roles
	}
	return items, total, nil
}

func (r *Repository) AssignInviteRole(
	ctx context.Context,
	querier Querier,
	inviteRole InviteRole,
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

// GetInviteRoleSummaries returns the roles stored on an invite.
func (r *Repository) GetInviteRoleSummaries(ctx context.Context, querier Querier, inviteID uuid.UUID) ([]RoleSummary, error) {
	rows, err := querier.Query(ctx, `
		SELECT r.id, r.name
		FROM invite_roles ir
		JOIN roles r ON r.id = ir.role_id
		WHERE ir.invite_id = $1
		ORDER BY r.name`, inviteID)
	if err != nil {
		return nil, fmt.Errorf("get invite roles: %w", err)
	}
	defer rows.Close()

	roles := make([]RoleSummary, 0)
	for rows.Next() {
		var role RoleSummary
		if err := rows.Scan(&role.ID, &role.Name); err != nil {
			return nil, fmt.Errorf("scan invite role: %w", err)
		}
		roles = append(roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invite roles: %w", err)
	}
	return roles, nil
}

// GetInviteRoleIDsInOrg returns the role IDs stored on an invite that belong
// to orgID. Accepting an invite assigns exactly these: stored at invite time,
// never taken from the accepting request, and filtered by org so a role of
// another org can never be attached.
func (r *Repository) GetInviteRoleIDsInOrg(ctx context.Context, querier Querier, inviteID, orgID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := querier.Query(ctx, `
		SELECT ir.role_id
		FROM invite_roles ir
		JOIN roles r ON r.id = ir.role_id
		WHERE ir.invite_id = $1 AND r.org_id = $2`, inviteID, orgID)
	if err != nil {
		return nil, fmt.Errorf("get invite roles: %w", err)
	}
	defer rows.Close()

	roleIDs := make([]uuid.UUID, 0)
	for rows.Next() {
		var roleID uuid.UUID
		if err := rows.Scan(&roleID); err != nil {
			return nil, fmt.Errorf("scan invite role: %w", err)
		}
		roleIDs = append(roleIDs, roleID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invite roles: %w", err)
	}
	return roleIDs, nil
}

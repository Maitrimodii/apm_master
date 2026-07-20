package repositories

import (
	"context"
	"errors"
	"fmt"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repo *Repository) CreatePasswordResetToken(ctx context.Context, querier Querier, token models.PasswordResetToken) (*models.PasswordResetToken, error) {
	query := `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, created_at
	`

	err := querier.QueryRow(ctx, query, token.UserID, token.TokenHash, token.ExpiresAt).Scan(&token.ID, &token.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create password reset token: %w", err)
	}

	return &token, nil
}

func (repo *Repository) GetPasswordResetTokenByHash(ctx context.Context, querier Querier, tokenHash string) (*models.PasswordResetToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1
	`

	var token models.PasswordResetToken
	err := querier.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UsedAt,
		&token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}

		return nil, fmt.Errorf("get password reset token by hash: %w", err)
	}

	return &token, nil
}

func (repo *Repository) MarkPasswordResetTokenUsed(ctx context.Context, querier Querier, tokenID uuid.UUID) error {
	query := `UPDATE password_reset_tokens SET used_at = now() WHERE id = $1 AND used_at IS NULL`
	_, err := querier.Exec(ctx, query, tokenID)
	if err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	return nil
}

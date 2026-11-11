package auth

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) CreatePasswordResetToken(ctx context.Context, querier Querier, token PasswordResetToken) (*PasswordResetToken, error) {
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

func (r *Repository) GetPasswordResetTokenByHash(ctx context.Context, querier Querier, tokenHash string) (*PasswordResetToken, error) {
	query := `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_reset_tokens
		WHERE token_hash = $1
	`

	var token PasswordResetToken
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

// ConsumePasswordResetToken marks the token with tokenHash used if it is
// unused and unexpired, and returns it; otherwise it returns nil. Being one
// conditional UPDATE, only one of several concurrent consumers gets the row
// (see ConsumeRefreshToken for how the losers are resolved).
func (r *Repository) ConsumePasswordResetToken(ctx context.Context, querier Querier, tokenHash string) (*PasswordResetToken, error) {
	query := `
		UPDATE password_reset_tokens
		SET used_at = now()
		WHERE token_hash = $1
		  AND used_at IS NULL
		  AND expires_at > now()
		RETURNING id, user_id, token_hash, expires_at, used_at, created_at
	`
	var token PasswordResetToken
	err := querier.QueryRow(ctx, query, tokenHash).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
		&token.ExpiresAt,
		&token.UsedAt,
		&token.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("consume password reset token: %w", err)
	}
	return &token, nil
}

// InvalidateUserPasswordResetTokens retires every unused reset token of a
// user. It sets used_at, the column the rest of the code already treats as
// "no longer redeemable", so no schema change is needed.
func (r *Repository) InvalidateUserPasswordResetTokens(ctx context.Context, querier Querier, userID uuid.UUID) error {
	query := `UPDATE password_reset_tokens SET used_at = now() WHERE user_id = $1 AND used_at IS NULL`
	if _, err := querier.Exec(ctx, query, userID); err != nil {
		return fmt.Errorf("invalidate password reset tokens: %w", err)
	}
	return nil
}

package middleware

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

type CommonModuleDeps struct {
	DB                *pgxpool.Pool
	PermissionChecker PermissionChecker
	TokenParser       TokenParser
	Logger            *slog.Logger
}

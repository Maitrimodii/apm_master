// Package module holds the dependencies every domain module receives.
package module

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// Deps are the shared dependencies passed to each module's NewModule.
type Deps struct {
	DB                *pgxpool.Pool
	PermissionChecker authz.Checker
	TokenParser       middleware.TokenParser
	Logger            *slog.Logger
	// RateLimit is mounted on authenticated route groups, after the auth
	// middleware, so limits apply per organization.
	RateLimit gin.HandlerFunc
}

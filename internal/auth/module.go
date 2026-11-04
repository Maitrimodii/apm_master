package auth

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/Maitrimodii/apm_master/internal/authz"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// Module wires the auth services and exposes the pieces other modules'
// middleware needs: permission checks, JWT parsing and API-key validation.
type Module struct {
	handler           *Handler
	PermissionChecker authz.Checker
	TokenParser       middleware.TokenParser
	APIKeyValidator   middleware.APIKeyValidator
}

func NewModule(db *pgxpool.Pool, rdb *redis.Client, cfg *config.Config, notifications *notification.Service, log *slog.Logger) *Module {
	repo := NewRepository(db)
	svc := Services{
		Sessions: NewSessionService(repo, cfg, log),
		Accounts: NewAccountService(repo, cfg, notifications, log),
		Invites:  NewInviteService(repo, cfg, notifications, log),
		Access:   NewAccessService(repo, log),
		APIKeys:  NewAPIKeyService(repo, rdb, log),
		Authz:    NewAuthzService(repo, log),
	}
	return &Module{
		handler:           NewHandler(svc, nil),
		PermissionChecker: svc.Authz,
		TokenParser:       svc.Sessions,
		APIKeyValidator:   svc.APIKeys,
	}
}

// WithRateLimit mounts a per-org rate limit on the authenticated auth routes.
func (m *Module) WithRateLimit(limit gin.HandlerFunc) *Module {
	m.handler.rateLimit = limit
	return m
}

func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	m.handler.RegisterRoutes(rg)
}

var (
	_ authz.Checker              = (*AuthzService)(nil)
	_ middleware.TokenParser     = (*SessionService)(nil)
	_ middleware.APIKeyValidator = (*APIKeyService)(nil)
)

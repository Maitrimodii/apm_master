package auth

import (
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/auth/repositories"
	"github.com/Maitrimodii/apm_master/internal/auth/services"
	authTransport "github.com/Maitrimodii/apm_master/internal/auth/transport"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/notification"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type Module struct {
	handler           *authTransport.Handler
	IdentityService   *services.IdentityService
	PermissionChecker middleware.PermissionChecker
	TokenParser       middleware.TokenParser
	APIKeyValidator   middleware.APIKeyValidator
}

func NewModule(
	database *pgxpool.Pool,
	rdb *redis.Client,
	cfg *config.Config,
	notificationService *notification.Service,
	log *slog.Logger,
) *Module {

	repo := repositories.NewRepository(
		database,
	)

	identityService :=
		services.NewIdentityService(
			repo,
			rdb,
			cfg,
			notificationService,
			log,
		)

	inviteService :=
		services.NewInviteService(
			repo,
			cfg,
			notificationService,
			log,
		)

	accessService :=
		services.NewAccessService(
			repo,
			rdb,
			log,
		)

	return &Module{
		handler: authTransport.NewHandler(
			identityService,
			inviteService,
			accessService,
		),
		IdentityService:   identityService,
		PermissionChecker: identityService,
		TokenParser:       identityService,
		APIKeyValidator:   identityService,
	}
}

func (module *Module) RegisterRoutes(
	routerGroup *gin.RouterGroup,
) {
	module.handler.RegisterRoutes(routerGroup)
}

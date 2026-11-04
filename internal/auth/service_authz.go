package auth

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/authz"
)

// AuthzService answers permission checks. It implements authz.Checker.
type AuthzService struct {
	repo *Repository
	log  *slog.Logger
}

func NewAuthzService(repo *Repository, log *slog.Logger) *AuthzService {
	if log == nil {
		log = slog.Default()
	}
	return &AuthzService{repo: repo, log: log}
}

func (s *AuthzService) CheckPermission(
	ctx context.Context,
	userID uuid.UUID,
	orgID uuid.UUID,
	resource authz.Resource,
	action authz.Action,
) (bool, error) {
	return s.repo.HasPermission(ctx, s.repo.Query(), userID, orgID, string(resource), action)
}

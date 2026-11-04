package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// APIKeyService creates, lists, revokes and validates machine API keys. It
// implements middleware.APIKeyValidator.
type APIKeyService struct {
	repo *Repository
	rdb  *redis.Client
	log  *slog.Logger
}

func NewAPIKeyService(repo *Repository, rdb *redis.Client, log *slog.Logger) *APIKeyService {
	if log == nil {
		log = slog.Default()
	}
	return &APIKeyService{repo: repo, rdb: rdb, log: log}
}

func (s *APIKeyService) ValidateAPIKey(
	ctx context.Context,
	rawToken string,
) (uuid.UUID, error) {

	tokenHash := HashToken(rawToken)
	cacheKey := "apikey:org:" + tokenHash

	// 1. Try to get cached org_id and api_key_id from Redis
	if val, err := s.rdb.Get(ctx, cacheKey).Result(); err == nil {
		parts := strings.Split(val, ":")
		if len(parts) == 2 {
			orgID, parseErr1 := uuid.Parse(parts[0])
			apiKeyID, parseErr2 := uuid.Parse(parts[1])
			if parseErr1 == nil && parseErr2 == nil {
				s.throttleLastUsedUpdate(ctx, apiKeyID)
				return orgID, nil
			}
		}
	}

	// 2. Cache miss -> query DB
	apiKey, err := s.repo.GetAPIKeyByHash(
		ctx,
		s.repo.Query(),
		tokenHash,
	)
	if err != nil {
		return uuid.UUID{}, err
	}

	if apiKey == nil {
		return uuid.UUID{}, apperr.Unauthorized(
			apperr.CodeAPIKeyInvalid,
			"api key not found",
		)
	}

	if apiKey.RevokedAt != nil {
		return uuid.UUID{}, apperr.Unauthorized(
			apperr.CodeAPIKeyInvalid,
			"api key revoked",
		)
	}

	// 3. Cache the result in Redis for 5 minutes (Format: orgID:apiKeyID)
	cacheValue := fmt.Sprintf("%s:%s", apiKey.OrgID.String(), apiKey.ID.String())
	_ = s.rdb.Set(ctx, cacheKey, cacheValue, 5*time.Minute).Err()

	// 4. Update last used with throttling
	s.throttleLastUsedUpdate(ctx, apiKey.ID)

	s.log.Debug("api key validated", "org_id", apiKey.OrgID, "api_key_id", apiKey.ID)

	return apiKey.OrgID, nil
}

func (s *APIKeyService) throttleLastUsedUpdate(ctx context.Context, apiKeyID uuid.UUID) {
	throttleKey := "apikey:lastused_throttle:" + apiKeyID.String()
	// Try to set the throttle key with a 1-minute TTL
	set, err := s.rdb.SetNX(ctx, throttleKey, "1", time.Minute).Result()
	if err == nil && set {
		// Only update the database if we successfully set the key (meaning it wasn't set already)
		if err := s.repo.UpdateAPIKeyLastUsed(ctx, s.repo.Query(), apiKeyID); err != nil {
			s.log.Warn("failed to update api key last used time", "api_key_id", apiKeyID, "error", err)
		}
	}
}

func (s *APIKeyService) CreateAPIKey(
	ctx context.Context,
	input CreateAPIKeyRequest,
) (*CreateAPIKeyResponse, error) {
	rawKey, prefix, err := GenerateAPIKey()
	if err != nil {
		return nil, apperr.Internal(err)
	}

	hashedKey := HashToken(rawKey)

	apiKey, err := s.repo.CreateAPIKey(
		ctx,
		s.repo.Query(),
		APIKey{
			OrgID:     input.OrgID,
			Name:      input.Name,
			KeyHash:   hashedKey,
			KeyPrefix: prefix,
		},
	)

	if err != nil {
		return nil, apperr.Internal(err)
	}

	s.log.Info("api key created", "org_id", input.OrgID, "api_key_id", apiKey.ID)

	return &CreateAPIKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       rawKey,
		Prefix:    apiKey.KeyPrefix,
		CreatedAt: apiKey.CreatedAt,
	}, nil
}

func (s *APIKeyService) GetAPIKeys(
	ctx context.Context,
	input GetAPIKeysRequest,
) ([]APIKeyResponse, error) {
	apiKeys, err := s.repo.GetAPIKeys(
		ctx,
		s.repo.Query(),
		input.OrgID,
	)
	if err != nil {
		return nil, apperr.Internal(err)
	}

	resp := make(
		[]APIKeyResponse,
		0,
		len(apiKeys),
	)

	for _, apiKey := range apiKeys {
		resp = append(
			resp,
			APIKeyResponse{
				ID:         apiKey.ID,
				Name:       apiKey.Name,
				KeyPrefix:  apiKey.KeyPrefix,
				LastUsedAt: apiKey.LastUsedAt,
				RevokedAt:  apiKey.RevokedAt,
				CreatedAt:  apiKey.CreatedAt,
			},
		)
	}

	return resp, nil
}

func (s *APIKeyService) RevokeAPIKey(
	ctx context.Context,
	input RevokeAPIKeyRequest,
) error {

	apiKey, err := s.repo.GetAPIKeyByID(
		ctx,
		s.repo.Query(),
		input.APIKeyID,
		input.OrgID,
	)
	if err != nil {
		return apperr.Internal(err)
	}

	if apiKey == nil {
		return apperr.NotFound(
			apperr.CodeNotFound,
			"api key not found",
		)
	}

	if apiKey.RevokedAt != nil {
		return apperr.BadRequest(
			apperr.CodeValidation,
			"api key already revoked",
		)
	}

	err = s.repo.RevokeAPIKey(
		ctx,
		s.repo.Query(),
		apiKey.ID,
	)
	if err != nil {
		return apperr.Internal(err)
	}

	// Proactively invalidate cached API key
	cacheKey := "apikey:org:" + apiKey.KeyHash
	if err := s.rdb.Del(ctx, cacheKey).Err(); err != nil {
		s.log.Warn("failed to invalidate api key cache on revocation", "api_key_id", apiKey.ID, "error", err)
	}

	s.log.Info("api key revoked", "org_id", input.OrgID, "api_key_id", apiKey.ID)

	return nil
}

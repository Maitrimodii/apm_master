package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

func (h *Handler) CreateAPIKey(
	c *gin.Context,
) {
	var req CreateAPIKeyRequest

	if err := c.ShouldBindJSON(
		&req,
	); err != nil {

		httpx.Handle(
			c,
			apperr.BadRequest(
				apperr.CodeValidation,
				"invalid request payload",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		c.GetString("org_id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.Unauthorized(
				apperr.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	req.OrgID = orgID

	resp, err := h.apiKeys.CreateAPIKey(
		c.Request.Context(),
		req,
	)
	if err != nil {
		httpx.Handle(
			c,
			err,
		)
		return
	}

	httpx.Created(
		c,
		resp,
	)
}

func (h *Handler) GetAPIKeys(
	c *gin.Context,
) {
	orgID, err := uuid.Parse(
		c.GetString("org_id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.Unauthorized(
				apperr.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	resp, err := h.apiKeys.GetAPIKeys(
		c.Request.Context(),
		GetAPIKeysRequest{
			OrgID: orgID,
		},
	)
	if err != nil {
		httpx.Handle(
			c,
			err,
		)
		return
	}

	httpx.OK(
		c,
		resp,
	)
}

func (h *Handler) RevokeAPIKey(
	c *gin.Context,
) {

	apiKeyID, err := uuid.Parse(
		c.Param("id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.BadRequest(
				apperr.CodeValidation,
				"invalid api key id",
			),
		)
		return
	}

	orgID, err := uuid.Parse(
		c.GetString("org_id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.Unauthorized(
				apperr.CodeInvalidToken,
				"invalid token",
			),
		)
		return
	}

	err = h.apiKeys.RevokeAPIKey(
		c.Request.Context(),
		RevokeAPIKeyRequest{
			APIKeyID: apiKeyID,
			OrgID:    orgID,
		},
	)
	if err != nil {
		httpx.Handle(
			c,
			err,
		)
		return
	}

	httpx.OK(
		c,
		"api key revoked successfully",
	)
}

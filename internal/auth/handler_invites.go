package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

func (h *Handler) CreateInvite(
	c *gin.Context,
) {
	var req InviteUserRequest

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

	userID, err := uuid.Parse(
		c.GetString(
			"user_id",
		),
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

	orgID, err := uuid.Parse(
		c.GetString(
			"org_id",
		),
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

	req.UserID = userID
	req.OrgID = orgID

	resp, err := h.invites.CreateInvite(
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

func (h *Handler) GetInviteByToken(
	c *gin.Context,
) {
	var req GetInviteRequest

	if err := c.ShouldBindQuery(
		&req,
	); err != nil {

		httpx.Handle(
			c,
			apperr.BadRequest(
				apperr.CodeValidation,
				"invalid request query",
			),
		)
		return
	}

	resp, err := h.invites.GetInviteByToken(
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

	httpx.OK(
		c,
		resp,
	)
}

func (h *Handler) AcceptInvite(
	c *gin.Context,
) {
	var req AcceptInviteRequest

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

	resp, err := h.invites.AcceptInvite(
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

	httpx.OK(
		c,
		resp,
	)
}

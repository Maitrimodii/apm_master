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

// invitesPageSize is the invite list's default page size.
const invitesPageSize = 50

// ListInvites lists the caller's org's invites, optionally filtered by
// ?status=pending|accepted|revoked|expired.
func (h *Handler) ListInvites(c *gin.Context) {
	orgID, apiErr := httpx.OrgID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	page, apiErr := httpx.ParsePage(c, invitesPageSize)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	status := InviteStatus(c.Query("status"))
	switch status {
	case "", InviteStatusPending, InviteStatusAccepted, InviteStatusRevoked, InviteStatusExpired:
	default:
		httpx.Handle(c, apperr.ValidationError("status", "must be one of pending, accepted, revoked, expired"))
		return
	}

	invites, total, err := h.invites.ListInvites(c.Request.Context(), orgID, status, page.Size, page.Offset())
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.List(c, invites, total, page.Number, page.Size)
}

// ResendInvite emails an invite again with a new link.
func (h *Handler) ResendInvite(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	inviteID, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	resp, err := h.invites.ResendInvite(c.Request.Context(), orgID, userID, inviteID)
	if err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.OK(c, resp)
}

// CancelInvite revokes an invite.
func (h *Handler) CancelInvite(c *gin.Context) {
	orgID, userID, apiErr := httpx.OrgAndUser(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	inviteID, apiErr := httpx.PathUUID(c, "id")
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	if err := h.invites.CancelInvite(c.Request.Context(), orgID, userID, inviteID); err != nil {
		httpx.Handle(c, err)
		return
	}
	httpx.NoContent(c)
}

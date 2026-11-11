package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

func (h *Handler) GetPermissions(
	c *gin.Context,
) {
	resp, err := h.access.GetPermissions(
		c.Request.Context(),
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

func (h *Handler) CreateRole(
	c *gin.Context,
) {
	var req CreateRoleRequest

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
	userID, apiErr := httpx.UserID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	req.UserID = userID

	resp, err := h.access.CreateRole(
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

func (h *Handler) GetRoles(
	c *gin.Context,
) {
	OrgID, err := uuid.Parse(
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

	resp, err := h.access.GetRoles(
		c.Request.Context(),
		GetRolesRequest{
			OrgID: OrgID,
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

func (h *Handler) GetRole(
	c *gin.Context,
) {
	roleID, err := uuid.Parse(
		c.Param("id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.BadRequest(
				apperr.CodeValidation,
				"invalid role id",
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

	resp, err := h.access.GetRole(
		c.Request.Context(),
		GetRoleRequest{
			RoleID: roleID,
			OrgID:  orgID,
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

func (h *Handler) UpdateRole(
	c *gin.Context,
) {
	var req UpdateRoleRequest

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

	roleID, err := uuid.Parse(
		c.Param("id"),
	)
	if err != nil {
		httpx.Handle(
			c,
			apperr.BadRequest(
				apperr.CodeValidation,
				"invalid role id",
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

	req.RoleID = roleID
	req.OrgID = orgID
	userID, apiErr := httpx.UserID(c)
	if apiErr != nil {
		httpx.Handle(c, apiErr)
		return
	}
	req.UserID = userID

	resp, err := h.access.UpdateRole(
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

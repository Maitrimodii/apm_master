package auth

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

func (h *Handler) RegisterOrgOwner(c *gin.Context) {
	var req RegisterOrgOwnerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := h.accounts.RegisterOrgOwner(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.Created(
		c,
		resp,
	)
}

func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	req.UserAgent = &userAgent
	req.IPAddress = &ipAddress

	resp, err := h.sessions.Login(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.OK(
		c,
		resp,
	)
}

func (h *Handler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	userAgent := c.Request.UserAgent()
	ipAddress := c.ClientIP()

	req.UserAgent = &userAgent
	req.IPAddress = &ipAddress

	resp, err := h.sessions.RefreshToken(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.OK(
		c,
		resp,
	)
}

func (h *Handler) Logout(c *gin.Context) {
	var req LogoutRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	err := h.sessions.Logout(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.OK(
		c,
		"logged out successfully",
	)
}

func (h *Handler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := h.accounts.ForgotPassword(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.OK(c, resp)
}

func (h *Handler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid request payload"))
		return
	}

	resp, err := h.accounts.ResetPassword(c.Request.Context(), req)
	if err != nil {
		httpx.Handle(c, err)
		return
	}

	httpx.OK(c, resp)
}

func (h *Handler) Me(
	c *gin.Context,
) {
	userIDStr := c.GetString(
		"user_id",
	)

	orgIDStr := c.GetString(
		"org_id",
	)

	userID, err := uuid.Parse(
		userIDStr,
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
		orgIDStr,
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

	resp, err := h.accounts.Me(
		c.Request.Context(),
		MeRequest{
			UserID: userID,
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

package investigation

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service           *Service
	permissionChecker middleware.PermissionChecker
	tokenParser       middleware.TokenParser
}

func NewHandler(
	service *Service,
	permissionChecker middleware.PermissionChecker,
	tokenParser middleware.TokenParser,
) *Handler {
	return &Handler{
		service:           service,
		permissionChecker: permissionChecker,
		tokenParser:       tokenParser,
	}
}

func (h *Handler) RegisterRoutes(routerGroup *gin.RouterGroup) {
	// Incident-scoped routes
	incidents := routerGroup.Group("/incidents")
	incidents.Use(middleware.JWTAuth(h.tokenParser))
	incidents.POST("/:incident_id/investigations", middleware.RequirePermission(h.permissionChecker, "investigations", models.Write), h.Create)
	incidents.GET("/:incident_id/investigations", middleware.RequirePermission(h.permissionChecker, "investigations", models.Read), h.ListByIncident)

	// Investigation-scoped routes
	investigations := routerGroup.Group("/investigations")
	investigations.Use(middleware.JWTAuth(h.tokenParser))
	investigations.GET("/:id", middleware.RequirePermission(h.permissionChecker, "investigations", models.Read), h.Get)
	investigations.GET("/:id/events", middleware.RequirePermission(h.permissionChecker, "investigations", models.Read), h.ListEvents)
	investigations.GET("/:id/report", middleware.RequirePermission(h.permissionChecker, "investigations", models.Read), h.GetReport)
	investigations.GET("/:id/evidence/:evidence_id", middleware.RequirePermission(h.permissionChecker, "investigations", models.Read), h.GetEvidence)
	investigations.POST("/:id/cancel", middleware.RequirePermission(h.permissionChecker, "investigations", models.Write), h.Cancel)
	investigations.POST("/:id/retry", middleware.RequirePermission(h.permissionChecker, "investigations", models.Write), h.Retry)
}

func (h *Handler) Create(c *gin.Context) {
	orgID, userID, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	incidentID, err := uuid.Parse(c.Param("incident_id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid incident_id"))
		return
	}

	var req CreateInvestigationRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			server.Handle(c, server.BadRequest(server.CodeValidation, "invalid request payload"))
			return
		}
	}

	correlationID := getOrGenerateCorrelationID(c)

	inv, err := h.service.Create(c.Request.Context(), orgID, userID, incidentID, correlationID, req)
	if err != nil {
		if errors.Is(err, ErrIncidentNotFound) {
			server.Handle(c, server.NotFound(server.CodeNotFound, "incident not found"))
			return
		}
		if errors.Is(err, ErrTimeWindowInvalid) || errors.Is(err, ErrTimeWindowTooLarge) {
			server.Handle(c, server.BadRequest(server.CodeValidation, err.Error()))
			return
		}
		server.Handle(c, server.Internal(err))
		return
	}

	c.Header("X-Correlation-ID", correlationID.String())
	c.JSON(http.StatusAccepted, inv)
}

func (h *Handler) ListByIncident(c *gin.Context) {
	orgID, _, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	incidentID, err := uuid.Parse(c.Param("incident_id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid incident_id"))
		return
	}

	page, limit := parsePagination(c)
	list, total, err := h.service.List(c.Request.Context(), orgID, incidentID, page, limit)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"investigations": list,
		"total":          total,
		"page":           page,
		"limit":          limit,
	})
}

func (h *Handler) Get(c *gin.Context) {
	orgID, _, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	inv, err := h.service.Get(c.Request.Context(), orgID, id)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}
	if inv == nil {
		server.Handle(c, server.NotFound(server.CodeNotFound, "investigation not found"))
		return
	}

	c.JSON(http.StatusOK, inv)
}

func (h *Handler) ListEvents(c *gin.Context) {
	orgID, _, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	events, err := h.service.ListEvents(c.Request.Context(), orgID, id)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"total":  len(events),
	})
}

func (h *Handler) GetReport(c *gin.Context) {
	orgID, _, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	report, err := h.service.GetReport(c.Request.Context(), orgID, id)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}
	if report == nil {
		server.Handle(c, server.NotFound(server.CodeNotFound, "report not yet available"))
		return
	}

	c.JSON(http.StatusOK, report)
}

func (h *Handler) GetEvidence(c *gin.Context) {
	orgID, _, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	evidenceID, err := uuid.Parse(c.Param("evidence_id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid evidence id"))
		return
	}

	ev, err := h.service.GetEvidence(c.Request.Context(), orgID, id, evidenceID)
	if err != nil {
		server.Handle(c, server.Internal(err))
		return
	}
	if ev == nil {
		server.Handle(c, server.NotFound(server.CodeNotFound, "evidence not found"))
		return
	}

	c.JSON(http.StatusOK, ev)
}

func (h *Handler) Cancel(c *gin.Context) {
	orgID, userID, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	inv, err := h.service.Cancel(c.Request.Context(), orgID, userID, id)
	if err != nil {
		if errors.Is(err, ErrInvestigationNotFound) {
			server.Handle(c, server.NotFound(server.CodeNotFound, "investigation not found"))
			return
		}
		server.Handle(c, server.Internal(err))
		return
	}

	c.JSON(http.StatusOK, inv)
}

func (h *Handler) Retry(c *gin.Context) {
	orgID, userID, err := authContext(c)
	if err != nil {
		server.Handle(c, err)
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		server.Handle(c, server.BadRequest(server.CodeValidation, "invalid investigation id"))
		return
	}

	correlationID := getOrGenerateCorrelationID(c)

	inv, err := h.service.Retry(c.Request.Context(), orgID, userID, id, correlationID)
	if err != nil {
		if errors.Is(err, ErrInvestigationNotFound) {
			server.Handle(c, server.NotFound(server.CodeNotFound, "investigation not found"))
			return
		}
		if errors.Is(err, ErrCannotRetryActive) {
			server.Handle(c, server.BadRequest(server.CodeValidation, err.Error()))
			return
		}
		server.Handle(c, server.Internal(err))
		return
	}

	c.Header("X-Correlation-ID", correlationID.String())
	c.JSON(http.StatusAccepted, inv)
}

func authContext(c *gin.Context) (orgID, userID uuid.UUID, err error) {
	orgIDStr := c.GetString("org_id")
	orgID, err = uuid.Parse(orgIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, server.Unauthorized(server.CodeInvalidToken, "invalid org id in context")
	}

	userIDStr := c.GetString("user_id")
	userID, err = uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, server.Unauthorized(server.CodeInvalidToken, "invalid user id in context")
	}

	return orgID, userID, nil
}

func getOrGenerateCorrelationID(c *gin.Context) uuid.UUID {
	headerVal := c.GetHeader("X-Correlation-ID")
	if headerVal != "" {
		if parsed, err := uuid.Parse(headerVal); err == nil {
			return parsed
		}
	}
	return uuid.New()
}

func parsePagination(c *gin.Context) (page, limit int) {
	page, _ = strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ = strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}
	return page, limit
}

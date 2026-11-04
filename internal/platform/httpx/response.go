package httpx

import (
	"net/http"
	"time"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Meta is present on every response — success and error.
type Meta struct {
	RequestID string `json:"request_id"`
	Timestamp string `json:"timestamp"`
}

// Pagination is included on list responses.
type Pagination struct {
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// Response is the unified envelope for ALL API responses.
// Success and error use the same shape — frontend writes one handler.
//
// Success: { success: true,  data: {...},  meta: {...} }
// Error:   { success: false, error: {...}, meta: {...} }
// List:    { success: true,  data: [...],  pagination: {...}, meta: {...} }
type Response struct {
	Success    bool          `json:"success"`
	Data       any           `json:"data,omitempty"`
	Error      *apperr.Error `json:"error,omitempty"`
	Pagination *Pagination   `json:"pagination,omitempty"`
	Meta       Meta          `json:"meta"`
}

func newMeta(c *gin.Context) Meta {
	rid := c.Writer.Header().Get(HeaderRequestID)
	if rid == "" {
		rid = c.GetHeader(HeaderRequestID)
	}
	if rid == "" {
		rid = uuid.New().String()
	}
	c.Header(HeaderRequestID, rid) // echo it back
	return Meta{RequestID: rid, Timestamp: time.Now().UTC().Format(time.RFC3339)}
}

// OK sends 200 with data.
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Meta:    newMeta(c),
	})
}

// Created sends 201 with data.
func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    data,
		Meta:    newMeta(c),
	})
}

// List sends 200 with data and pagination.
// Use for all list endpoints — traces, metrics, logs etc.
func List(c *gin.Context, data any, total, page, limit int) {
	c.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Pagination: &Pagination{
			Total: total,
			Page:  page,
			Limit: limit,
		},
		Meta: newMeta(c),
	})
}

// NoContent sends 204 with no body.
// Use for DELETE endpoints.
func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

// Accepted sends 202 with data, for requests that start asynchronous work.
func Accepted(c *gin.Context, data any) {
	c.JSON(http.StatusAccepted, Response{
		Success: true,
		Data:    data,
		Meta:    newMeta(c),
	})
}

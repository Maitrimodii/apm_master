package server

import (
	"net/http"
	"time"

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
	Success    bool        `json:"success"`
	Data       any         `json:"data,omitempty"`
	Error      *Error      `json:"error,omitempty"`
	Pagination *Pagination `json:"pagination,omitempty"`
	Meta       Meta        `json:"meta"`
}

func newMeta(ginContext *gin.Context) Meta {
	rid := ginContext.Writer.Header().Get("X-Request-ID")
	if rid == "" {
		rid = ginContext.GetHeader("X-Request-ID")
	}
	if rid == "" {
		rid = uuid.New().String()
	}
	ginContext.Header("X-Request-ID", rid) // echo it back
	return Meta{RequestID: rid, Timestamp: time.Now().UTC().Format(time.RFC3339)}
}

// OK sends 200 with data.
func OK(ginContext *gin.Context, data any) {
	ginContext.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Meta:    newMeta(ginContext),
	})
}

// Created sends 201 with data.
func Created(ginContext *gin.Context, data any) {
	ginContext.JSON(http.StatusCreated, Response{
		Success: true,
		Data:    data,
		Meta:    newMeta(ginContext),
	})
}

// List sends 200 with data and pagination.
// Use for all list endpoints — traces, metrics, logs etc.
func List(ginContext *gin.Context, data any, total, page, limit int) {
	ginContext.JSON(http.StatusOK, Response{
		Success: true,
		Data:    data,
		Pagination: &Pagination{
			Total: total,
			Page:  page,
			Limit: limit,
		},
		Meta: newMeta(ginContext),
	})
}

// NoContent sends 204 with no body.
// Use for DELETE endpoints.
func NoContent(ginContext *gin.Context) {
	ginContext.Status(http.StatusNoContent)
}

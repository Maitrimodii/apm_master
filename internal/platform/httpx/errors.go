package httpx

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// Handle renders err as the API envelope. Errors that are not *apperr.Error
// become a generic 500 so internals never leak to clients.
func Handle(c *gin.Context, err error) {
	_ = c.Error(err) // Ensure error is logged by middleware
	meta := newMeta(c)

	var apiErr *apperr.Error
	if !errors.As(err, &apiErr) {
		c.Set(CtxKeyErrorType, string(apperr.TypeInternal))
		c.Set(CtxKeyErrorCode, string(apperr.CodeInternal))
		// unknown error — log internally, never expose
		c.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error: &apperr.Error{
				Type:    apperr.TypeInternal,
				Code:    apperr.CodeInternal,
				Message: "internal server error",
			},
			Meta: meta,
		})
		return
	}

	c.Set(CtxKeyErrorType, string(apiErr.Type))
	c.Set(CtxKeyErrorCode, string(apiErr.Code))

	// A throttled request that says when to retry also gets the standard header.
	if apiErr.Type == apperr.TypeRateLimited {
		if wait, ok := apiErr.Data["retry_after_seconds"].(int); ok && wait > 0 {
			c.Header("Retry-After", strconv.Itoa(wait))
		}
	}

	c.JSON(httpStatus(apiErr.Type), Response{
		Success: false,
		Error:   apiErr,
		Meta:    meta,
	})
}

func httpStatus(typ apperr.Type) int {
	switch typ {
	case apperr.TypeBadRequest:
		return http.StatusBadRequest
	case apperr.TypeUnauthorized:
		return http.StatusUnauthorized
	case apperr.TypeForbidden:
		return http.StatusForbidden
	case apperr.TypeNotFound:
		return http.StatusNotFound
	case apperr.TypeAlreadyExists, apperr.TypeConflict:
		return http.StatusConflict
	case apperr.TypeRateLimited:
		return http.StatusTooManyRequests
	default:
		return http.StatusInternalServerError
	}
}

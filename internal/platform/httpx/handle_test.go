package httpx

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"

	"github.com/gin-gonic/gin"
)

func TestHTTPStatusMapsTypedErrors(testRunner *testing.T) {
	tests := []struct {
		name string
		typ  apperr.Type
		want int
	}{
		{name: "bad request", typ: apperr.TypeBadRequest, want: http.StatusBadRequest},
		{name: "unauthorized", typ: apperr.TypeUnauthorized, want: http.StatusUnauthorized},
		{name: "forbidden", typ: apperr.TypeForbidden, want: http.StatusForbidden},
		{name: "not found", typ: apperr.TypeNotFound, want: http.StatusNotFound},
		{name: "already exists", typ: apperr.TypeAlreadyExists, want: http.StatusConflict},
		{name: "internal", typ: apperr.TypeInternal, want: http.StatusInternalServerError},
		{name: "unknown", typ: apperr.Type("Other"), want: http.StatusInternalServerError},
	}

	for _, testCases := range tests {
		testRunner.Run(testCases.name, func(testRunner *testing.T) {
			if got := httpStatus(testCases.typ); got != testCases.want {
				testRunner.Fatalf("httpStatus(%q) = %d, want %d", testCases.typ, got, testCases.want)
			}
		})
	}
}

func TestErrorCanBeUnwrappedAfterWrapping(testRunner *testing.T) {
	err := fmt.Errorf("service boundary: %w", apperr.NotFound(apperr.CodeNotFound, "missing"))

	var apiErr *apperr.Error
	if !errors.As(err, &apiErr) {
		testRunner.Fatal("expected wrapped server.Error to be discoverable")
	}

	if apiErr.Type != apperr.TypeNotFound {
		testRunner.Fatalf("apperr.Type = %q, want %q", apiErr.Type, apperr.TypeNotFound)
	}
}

func TestHandleSetsSanitizedErrorMetadata(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	Handle(c, apperr.BadRequest(apperr.CodeValidation, "invalid field"))

	if rec.Code != http.StatusBadRequest {
		testRunner.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := c.GetString("error_type"); got != string(apperr.TypeBadRequest) {
		testRunner.Fatalf("error_type = %q, want %q", got, apperr.TypeBadRequest)
	}
	if got := c.GetString("error_code"); got != string(apperr.CodeValidation) {
		testRunner.Fatalf("error_code = %q, want %q", got, apperr.CodeValidation)
	}
}

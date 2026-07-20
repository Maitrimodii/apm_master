package server

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTTPStatusMapsTypedErrors(testRunner *testing.T) {
	tests := []struct {
		name string
		typ  Type
		want int
	}{
		{name: "bad request", typ: TypeBadRequest, want: http.StatusBadRequest},
		{name: "unauthorized", typ: TypeUnauthorized, want: http.StatusUnauthorized},
		{name: "forbidden", typ: TypeForbidden, want: http.StatusForbidden},
		{name: "not found", typ: TypeNotFound, want: http.StatusNotFound},
		{name: "already exists", typ: TypeAlreadyExists, want: http.StatusConflict},
		{name: "internal", typ: TypeInternal, want: http.StatusInternalServerError},
		{name: "unknown", typ: Type("Other"), want: http.StatusInternalServerError},
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
	err := fmt.Errorf("service boundary: %w", NotFound(CodeNotFound, "missing"))

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		testRunner.Fatal("expected wrapped server.Error to be discoverable")
	}

	if apiErr.Type != TypeNotFound {
		testRunner.Fatalf("Type = %q, want %q", apiErr.Type, TypeNotFound)
	}
}

func TestHandleSetsSanitizedErrorMetadata(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(rec)
	ginContext.Request = httptest.NewRequest(http.MethodGet, "/test", nil)

	Handle(ginContext, BadRequest(CodeValidation, "invalid field"))

	if rec.Code != http.StatusBadRequest {
		testRunner.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := ginContext.GetString("error_type"); got != string(TypeBadRequest) {
		testRunner.Fatalf("error_type = %q, want %q", got, TypeBadRequest)
	}
	if got := ginContext.GetString("error_code"); got != string(CodeValidation) {
		testRunner.Fatalf("error_code = %q, want %q", got, CodeValidation)
	}
}

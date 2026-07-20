package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// ─── LAYER — internal only, never sent to client ──────────────────────────────
type Layer string

const (
	LayerHandler    Layer = "handler"
	LayerService    Layer = "service"
	LayerRepository Layer = "repository"
)

// ─── TYPE — sent to client, maps to HTTP status ───────────────────────────────
type Type string

const (
	TypeBadRequest    Type = "BadRequest"
	TypeUnauthorized  Type = "Unauthorized"
	TypeForbidden     Type = "Forbidden"
	TypeNotFound      Type = "NotFound"
	TypeAlreadyExists Type = "AlreadyExists"
	TypeInternal      Type = "InternalError"
)

// ─── CODE — unique per error scenario ─────────────────────────────────────────
type Code string

const (
	// system
	CodeInternal   Code = "SYS001"
	CodeDatabase   Code = "SYS002"
	CodeValidation Code = "SYS003"
	CodeNotFound   Code = "SYS004"

	// auth
	CodeEmailExists           Code = "AUTH001"
	CodeInvalidCredentials    Code = "AUTH002"
	CodeInvalidToken          Code = "AUTH003"
	CodeTokenExpired          Code = "AUTH004"
	CodeUserNotFound          Code = "AUTH005"
	CodeOrgNotFound           Code = "AUTH006"
	CodeRoleNotFound          Code = "AUTH007"
	CodePermissionDenied      Code = "AUTH008"
	CodeAPIKeyInvalid         Code = "AUTH009"
	CodeForbidden             Code = "AUTH010"
	CodeOrgSlugAlreadyExists  Code = "AUTH011"
	CodeInviteAlreadyExists   Code = "AUTH012"
	CodeInviteAlreadyAccepted Code = "AUTH013"
	CodeInviteExpired         Code = "AUTH014"
	CodeRoleAlreadyExists     Code = "AUTH015"

	// ingest
	CodeInvalidPayload Code = "INGEST001"
	CodeIngestFailed   Code = "INGEST002"
)

// ─── ERROR ────────────────────────────────────────────────────────────────────
// Layer and Cause: json:"-" means never serialized
// never sent to client under any circumstances
type Error struct {
	Layer   Layer          `json:"-"`
	Cause   error          `json:"-"`
	Type    Type           `json:"type"`
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (errVal *Error) Error() string {
	if errVal.Cause != nil {
		return fmt.Sprintf("[%s][%s] %s: %v",
			errVal.Layer, errVal.Code, errVal.Message, errVal.Cause)
	}
	return fmt.Sprintf("[%s][%s] %s", errVal.Layer, errVal.Code, errVal.Message)
}

func (errVal *Error) Unwrap() error { return errVal.Cause }

func (errVal *Error) WithData(data map[string]any) *Error {
	errVal.Data = data
	return errVal
}

// ─── TYPED CONSTRUCTORS ───────────────────────────────────────────────────────
// Layer not exposed — set internally per constructor.
// Typed constructors are the ONLY public API.

func BadRequest(code Code, message string) *Error {
	return &Error{Layer: LayerHandler, Type: TypeBadRequest,
		Code: code, Message: message}
}

func Unauthorized(code Code, message string) *Error {
	return &Error{Layer: LayerHandler, Type: TypeUnauthorized,
		Code: code, Message: message}
}

func Forbidden(code Code, message string) *Error {
	return &Error{Layer: LayerService, Type: TypeForbidden,
		Code: code, Message: message}
}

func NotFound(code Code, message string) *Error {
	return &Error{Layer: LayerService, Type: TypeNotFound,
		Code: code, Message: message}
}

func AlreadyExists(code Code, message string) *Error {
	return &Error{Layer: LayerService, Type: TypeAlreadyExists,
		Code: code, Message: message}
}

func Internal(cause error) *Error {
	if cause != nil {
		causeStr := strings.ToLower(cause.Error())
		if strings.Contains(causeStr, "sql:") ||
			strings.Contains(causeStr, "tx:") ||
			strings.Contains(causeStr, "database") ||
			strings.Contains(causeStr, "pgx") ||
			strings.Contains(causeStr, "conn") {
			return DatabaseError(cause)
		}
	}
	return &Error{Layer: LayerService, Type: TypeInternal,
		Code: CodeInternal, Message: "internal server error", Cause: cause}
}

func DatabaseError(cause error) *Error {
	return &Error{Layer: LayerRepository, Type: TypeInternal,
		Code: CodeDatabase, Message: "database error", Cause: cause}
}

// ValidationError creates a field-level validation error.
// Use when a specific field fails validation.
func ValidationError(field, reason string) *Error {
	return &Error{
		Layer:   LayerHandler,
		Type:    TypeBadRequest,
		Code:    CodeValidation,
		Message: "validation failed",
		Data:    map[string]any{"field": field, "reason": reason},
	}
}

// ─── HTTP HANDLER ─────────────────────────────────────────────────────────────
// Handle belongs here — it is about handling errors.

func Handle(ginContext *gin.Context, err error) {
	_ = ginContext.Error(err) // Ensure error is logged by middleware
	meta := newMeta(ginContext)

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		ginContext.Set("error_type", string(TypeInternal))
		ginContext.Set("error_code", string(CodeInternal))
		// unknown error — log internally, never expose
		ginContext.JSON(http.StatusInternalServerError, Response{
			Success: false,
			Error: &Error{
				Type:    TypeInternal,
				Code:    CodeInternal,
				Message: "internal server error",
			},
			Meta: meta,
		})
		return
	}

	ginContext.Set("error_type", string(apiErr.Type))
	ginContext.Set("error_code", string(apiErr.Code))

	ginContext.JSON(httpStatus(apiErr.Type), Response{
		Success: false,
		Error:   apiErr,
		Meta:    meta,
	})
}

func httpStatus(testRunner Type) int {
	switch testRunner {
	case TypeBadRequest:
		return http.StatusBadRequest
	case TypeUnauthorized:
		return http.StatusUnauthorized
	case TypeForbidden:
		return http.StatusForbidden
	case TypeNotFound:
		return http.StatusNotFound
	case TypeAlreadyExists:
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

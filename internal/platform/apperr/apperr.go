// Package apperr is the application error type returned by services,
// stores and background tasks. It carries a kind (Type), a stable code and a
// client-safe message; transports decide how to render it.
package apperr

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
)

// Layer records where an error was raised. It is logged, never sent to clients.
type Layer string

const (
	LayerHandler    Layer = "handler"
	LayerService    Layer = "service"
	LayerRepository Layer = "repository"
)

// Type is the error kind sent to clients; transports map it to a status code.
type Type string

const (
	TypeBadRequest    Type = "BadRequest"
	TypeUnauthorized  Type = "Unauthorized"
	TypeForbidden     Type = "Forbidden"
	TypeNotFound      Type = "NotFound"
	TypeAlreadyExists Type = "AlreadyExists"
	TypeConflict      Type = "Conflict"
	TypeRateLimited   Type = "RateLimited"
	TypeInternal      Type = "InternalError"
)

// Code identifies one error scenario, stable across releases.
type Code string

const (
	// system
	CodeInternal   Code = "SYS001"
	CodeDatabase   Code = "SYS002"
	CodeValidation Code = "SYS003"
	CodeNotFound   Code = "SYS004"
	CodeRateLimit  Code = "SYS005"
	CodeConflict   Code = "SYS006"

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
	CodeInviteRevoked         Code = "AUTH016"

	// ingest
	CodeInvalidPayload Code = "INGEST001"
	CodeIngestFailed   Code = "INGEST002"
)

// Error is the application error. Layer and Cause are never serialized, so
// internals never reach clients.
type Error struct {
	Layer   Layer          `json:"-"`
	Cause   error          `json:"-"`
	Type    Type           `json:"type"`
	Code    Code           `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s][%s] %s: %v",
			e.Layer, e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s][%s] %s", e.Layer, e.Code, e.Message)
}

func (e *Error) Unwrap() error { return e.Cause }

func (e *Error) WithData(data map[string]any) *Error {
	e.Data = data
	return e
}

// Typed constructors are the only way to build an Error; each sets its Layer.
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

// Conflict reports a request that is valid but clashes with current state
// (for example an illegal state transition).
func Conflict(code Code, message string) *Error {
	return &Error{Layer: LayerService, Type: TypeConflict,
		Code: code, Message: message}
}

// RateLimited reports that the caller exceeded its request budget.
func RateLimited() *Error {
	return &Error{Layer: LayerHandler, Type: TypeRateLimited,
		Code: CodeRateLimit, Message: "rate limit exceeded"}
}

// Internal wraps an unexpected failure. Postgres errors are reported with
// CodeDatabase so operators can tell storage problems apart.
func Internal(cause error) *Error {
	if isDatabaseError(cause) {
		return DatabaseError(cause)
	}
	return &Error{Layer: LayerService, Type: TypeInternal,
		Code: CodeInternal, Message: "internal server error", Cause: cause}
}

func isDatabaseError(err error) bool {
	var pgErr *pgconn.PgError
	var connErr *pgconn.ConnectError
	return errors.As(err, &pgErr) || errors.As(err, &connErr) || pgconn.Timeout(err)
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

// Mapping translates a module's sentinel errors into application errors. It
// keeps services free of transport concerns: they return plain sentinels and
// the module's handler maps them in one place.
type Mapping map[error]*Error

// Wrap returns the mapped error for err (matched with errors.Is), keeping err
// as the cause. *Error values pass through; anything else becomes Internal.
func (m Mapping) Wrap(err error) error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr
	}
	for sentinel, mapped := range m {
		if errors.Is(err, sentinel) {
			out := *mapped
			out.Cause = err
			return &out
		}
	}
	return Internal(err)
}

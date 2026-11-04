package httpx

// Gin context keys set by the auth middlewares.
const (
	CtxKeyUserID        = "user_id"
	CtxKeyOrgID         = "org_id"
	CtxKeyEmail         = "email"
	CtxKeyRequestID     = "request_id"
	CtxKeyCorrelationID = "correlation_id"
	CtxKeyErrorType     = "error_type"
	CtxKeyErrorCode     = "error_code"
)

// HTTP header names.
const (
	HeaderAuthorization = "Authorization"
	HeaderRequestID     = "X-Request-ID"
	HeaderCorrelationID = "X-Correlation-ID"
	HeaderAPIKey        = "X-API-Key"
	BearerScheme        = "Bearer"
)

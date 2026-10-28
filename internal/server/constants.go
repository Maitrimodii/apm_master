package server

// Context key constants for Gin context and standard Go context propagation.
const (
	CtxKeyUserID    = "user_id"
	CtxKeyOrgID     = "org_id"
	CtxKeyEmail     = "email"
	CtxKeyRequestID = "request_id"
)

// Header constants for standard HTTP requests and responses.
const (
	HeaderAuthorization = "Authorization"
	HeaderRequestID     = "X-Request-ID"
	HeaderServiceToken  = "X-Service-Token"
	BearerPrefix        = "Bearer "
)

// Route group prefix constants.
const (
	RoutePrefixAPIV1      = "/api/v1"
	RoutePrefixInternalV1 = "/internal/v1"
)

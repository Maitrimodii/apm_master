package trace

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newTraceTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ginContext
}

func TestParseListParams(testRunner *testing.T) {
	from := "2026-01-01T10:00:00Z"
	timeout := "2026-01-01T11:00:00Z"

	params, err := parseListParams(newTraceTestContext("/traces?from=" + from + "&to=" + timeout + "&page=3&limit=25&service=api&status=ERROR&trace_id=trace-1&endpoint=/api/articles&min_duration=10.5&max_duration=500"))
	if err != nil {
		testRunner.Fatalf("parseListParams() error = %v", err)
	}

	wantFrom, _ := time.Parse(time.RFC3339, from)
	wantTo, _ := time.Parse(time.RFC3339, timeout)
	if !params.From.Equal(wantFrom) || !params.To.Equal(wantTo) {
		testRunner.Fatalf("range = (%v, %v), want (%v, %v)", params.From, params.To, wantFrom, wantTo)
	}
	if params.Page != 3 {
		testRunner.Fatalf("Page = %d, want 3", params.Page)
	}
	if params.Offset != 50 {
		testRunner.Fatalf("Offset = %d, want 50", params.Offset)
	}
	if params.Limit != 25 {
		testRunner.Fatalf("Limit = %d, want 25", params.Limit)
	}
	if params.ServiceName != "api" || params.StatusCode != "ERROR" || params.TraceID != "trace-1" || params.Endpoint != "/api/articles" {
		testRunner.Fatalf("filters = (%q, %q, %q, %q), want (api, ERROR, trace-1, /api/articles)", params.ServiceName, params.StatusCode, params.TraceID, params.Endpoint)
	}
	if params.MinDurationMS != 10.5 || params.MaxDurationMS != 500 {
		testRunner.Fatalf("duration filters = (%v, %v), want (10.5, 500)", params.MinDurationMS, params.MaxDurationMS)
	}
}

func TestParseListParamsRejectsInvalidRange(testRunner *testing.T) {
	if _, err := parseListParams(newTraceTestContext("/traces?from=2026-01-01T11:00:00Z&to=2026-01-01T10:00:00Z")); err == nil {
		testRunner.Fatal("expected error when to is before from")
	}
}

func TestRegisterRoutesIncludesTraceLookup(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := router.Group("/api/v1")
	NewHandler(nil, nil, nil).RegisterRoutes(api)

	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/traces/:traceID" {
			return
		}
	}

	testRunner.Fatal("expected GET /api/v1/traces/:traceID to be registered")
}

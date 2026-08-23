package log

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newLogTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ginContext
}

func TestParseListParams(testRunner *testing.T) {
	from := "2026-01-01T10:00:00Z"
	timeout := "2026-01-01T11:00:00Z"

	params, err := parseListParams(newLogTestContext("/logs?from=" + from + "&to=" + timeout + "&page=2&limit=20&service=api&min_severity=13&trace_id=trace-1&q=timeout"))
	if err != nil {
		testRunner.Fatalf("parseListParams() error = %v", err)
	}

	wantFrom, _ := time.Parse(time.RFC3339, from)
	wantTo, _ := time.Parse(time.RFC3339, timeout)
	if params.From == nil || !params.From.Equal(wantFrom) {
		testRunner.Fatalf("From = %v, want %v", params.From, wantFrom)
	}
	if params.To == nil || !params.To.Equal(wantTo) {
		testRunner.Fatalf("To = %v, want %v", params.To, wantTo)
	}
	if params.Page != 2 || params.Offset != 20 || params.Limit != 20 {
		testRunner.Fatalf("pagination = (page %d, offset %d, limit %d), want (2, 20, 20)", params.Page, params.Offset, params.Limit)
	}
	if params.ServiceName != "api" || params.MinSeverity != 13 || params.TraceID != "trace-1" || params.Search != "timeout" {
		testRunner.Fatalf("unexpected filters: %+v", params)
	}
}

func TestParseListParamsFallsBackInvalidPagination(testRunner *testing.T) {
	params, err := parseListParams(newLogTestContext("/logs?page=-1&limit=2000"))
	if err != nil {
		testRunner.Fatalf("parseListParams() error = %v", err)
	}
	if params.Page != 1 || params.Limit != 100 || params.Offset != 0 {
		testRunner.Fatalf("pagination = (page %d, limit %d, offset %d), want (1, 100, 0)", params.Page, params.Limit, params.Offset)
	}
}

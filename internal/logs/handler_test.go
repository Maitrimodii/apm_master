package logs

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func newLogTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func TestParseListFilter(testRunner *testing.T) {
	from := "2026-01-01T10:00:00Z"
	timeout := "2026-01-01T11:00:00Z"

	params, page, err := parseListFilter(newLogTestContext("/logs?from=" + from + "&to=" + timeout + "&page=2&limit=20&service=api&min_severity=13&trace_id=trace-1&q=timeout"))
	if err != nil {
		testRunner.Fatalf("parseListFilter() error = %v", err)
	}

	wantFrom, _ := time.Parse(time.RFC3339, from)
	wantTo, _ := time.Parse(time.RFC3339, timeout)
	if !params.From.Equal(wantFrom) {
		testRunner.Fatalf("From = %v, want %v", params.From, wantFrom)
	}
	if !params.To.Equal(wantTo) {
		testRunner.Fatalf("To = %v, want %v", params.To, wantTo)
	}
	if page.Number != 2 || params.Offset != 20 || params.Limit != 20 {
		testRunner.Fatalf("pagination = (page %d, offset %d, limit %d), want (2, 20, 20)", page.Number, params.Offset, params.Limit)
	}
	if params.ServiceName != "api" || params.MinSeverity != 13 || params.TraceID != "trace-1" || params.Search != "timeout" {
		testRunner.Fatalf("unexpected filters: %+v", params)
	}
}

func TestParseListFilterRejectsInvalidPagination(testRunner *testing.T) {
	for _, target := range []string{"/logs?page=-1", "/logs?limit=2000", "/logs?limit=abc"} {
		if _, _, err := parseListFilter(newLogTestContext(target)); err == nil {
			testRunner.Errorf("parseListFilter(%q) error = nil, want validation error", target)
		}
	}
}

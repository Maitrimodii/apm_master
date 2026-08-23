package metric

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func newTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)

	ginContext, _ := gin.CreateTestContext(httptest.NewRecorder())
	ginContext.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return ginContext
}

func TestParsePagination(testRunner *testing.T) {
	tests := []struct {
		name      string
		target    string
		wantPage  int
		wantLimit int
	}{
		{name: "defaults", target: "/metrics", wantPage: 1, wantLimit: 50},
		{name: "configured", target: "/metrics?page=3&limit=100", wantPage: 3, wantLimit: 100},
		{name: "invalid values fallback", target: "/metrics?page=0&limit=999", wantPage: 1, wantLimit: 50},
	}

	for _, testCases := range tests {
		testRunner.Run(testCases.name, func(testRunner *testing.T) {
			page, limit := parsePagination(newTestContext(testCases.target))
			if page != testCases.wantPage || limit != testCases.wantLimit {
				testRunner.Fatalf("parsePagination() = (%d, %d), want (%d, %d)", page, limit, testCases.wantPage, testCases.wantLimit)
			}
		})
	}
}

func TestParseTimeRange(testRunner *testing.T) {
	from := "2026-01-01T10:00:00Z"
	timeout := "2026-01-01T11:00:00Z"

	got, err := parseTimeRange(newTestContext("/metrics?from=" + from + "&to=" + timeout))
	if err != nil {
		testRunner.Fatalf("parseTimeRange() error = %v", err)
	}

	wantFrom, _ := time.Parse(time.RFC3339, from)
	wantTo, _ := time.Parse(time.RFC3339, timeout)
	if !got.from.Equal(wantFrom) || !got.endTime.Equal(wantTo) {
		testRunner.Fatalf("parseTimeRange() = (%v, %v), want (%v, %v)", got.from, got.endTime, wantFrom, wantTo)
	}

	if _, err := parseTimeRange(newTestContext("/metrics?from=" + timeout + "&to=" + from)); err == nil {
		testRunner.Fatal("expected error when to is before from")
	}
}

func TestParseOrgID(testRunner *testing.T) {
	orgID := uuid.New()
	ginContext := newTestContext("/metrics")
	ginContext.Set("org_id", orgID.String())

	got, err := parseOrgID(ginContext)
	if err != nil {
		testRunner.Fatalf("parseOrgID() error = %v", err)
	}
	if got != orgID {
		testRunner.Fatalf("parseOrgID() = %s, want %s", got, orgID)
	}
}

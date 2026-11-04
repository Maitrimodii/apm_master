package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMetricTestContext(target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, target, nil)
	return c
}

func TestParseLabelFilters(t *testing.T) {
	got := parseLabelFilters(newMetricTestContext("/metrics/x/series?label[service.name]=api&label[env]=prod&other=1&label[bad=x"))
	if len(got) != 2 || got["service.name"] != "api" || got["env"] != "prod" {
		t.Fatalf("parseLabelFilters() = %v, want service.name=api, env=prod", got)
	}
}

func TestRegisterRoutesIncludesValuesRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	NewHandler(nil, nil, nil, nil).RegisterRoutes(router.Group("/api/v1"))

	want := map[string]bool{
		"/api/v1/metrics":                          false,
		"/api/v1/metrics/:metricName/values":       false,
		"/api/v1/metrics/:metricName/series":       false,
		"/api/v1/metrics/:metricName/aggregations": false,
		"/api/v1/metrics/:metricName/histogram":    false,
	}
	for _, r := range router.Routes() {
		if _, ok := want[r.Path]; ok && r.Method == http.MethodGet {
			want[r.Path] = true
		}
	}
	for path, found := range want {
		if !found {
			t.Errorf("GET %s not registered", path)
		}
	}
}

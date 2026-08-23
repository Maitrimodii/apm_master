package alert

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutesIncludesAlertInstances(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	api := router.Group("/api/v1")
	NewHandler(nil, nil, nil).RegisterRoutes(api)

	expectedRoutes := map[string]string{
		"GET":  "/api/v1/alert-instances",
		"GET2": "/api/v1/alert-instances/:id",
		"POST": "/api/v1/alert-instances/:id/acknowledge",
	}

	foundRoutes := make(map[string]bool)
	for _, route := range router.Routes() {
		if route.Method == http.MethodGet && route.Path == "/api/v1/alert-instances" {
			foundRoutes["GET"] = true
		}
		if route.Method == http.MethodGet && route.Path == "/api/v1/alert-instances/:id" {
			foundRoutes["GET2"] = true
		}
		if route.Method == http.MethodPost && route.Path == "/api/v1/alert-instances/:id/acknowledge" {
			foundRoutes["POST"] = true
		}
	}

	for key, path := range expectedRoutes {
		if !foundRoutes[key] {
			testRunner.Fatalf("expected route %s to be registered", path)
		}
	}
}

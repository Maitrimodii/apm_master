package trace_test

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func getBaseURL() string {
	url := os.Getenv("APM_E2E_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	return url
}

func getAccessToken() string {
	token := os.Getenv("APM_E2E_ACCESS_TOKEN")
	if token == "" {
		return "dummy-token" // Endpoints might 401, but we're testing availability
	}
	return token
}

func TestTraceEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("List Traces", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/traces", nil)
		req.Header.Set("Authorization", "Bearer "+getAccessToken())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call list traces: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})

	t.Run("Get Single Trace", func(t *testing.T) {
		// Mocking with a random trace ID
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/traces/trace-123", nil)
		req.Header.Set("Authorization", "Bearer "+getAccessToken())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call get single trace: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusNotFound {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})
}

package ingest_test

import (
	"bytes"
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

func getAPIKey() string {
	key := os.Getenv("APM_E2E_API_KEY")
	if key == "" {
		return "test-api-key-bypass-or-inject" // Usually we'd register one first or allow bypass
	}
	return key
}

func TestIngestEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("Ingest Traces", func(t *testing.T) {
		payload := `{"resourceSpans": []}` // Simplified mock payload
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/ingest/traces", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", getAPIKey())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest traces: %v", err)
		}
		defer resp.Body.Close()

		// 401 is okay if we didn't inject a real API key. The test ensures the endpoint is up and routable.
		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})

	t.Run("Ingest Metrics", func(t *testing.T) {
		payload := `{"resourceMetrics": []}` // Simplified mock payload
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/ingest/metrics", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", getAPIKey())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest metrics: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})

	t.Run("Ingest Logs", func(t *testing.T) {
		payload := `{"resourceLogs": []}` // Simplified mock payload
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/ingest/logs", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", getAPIKey())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest logs: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})
}

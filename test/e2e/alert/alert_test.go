package alert_test

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

func getAccessToken() string {
	token := os.Getenv("APM_E2E_ACCESS_TOKEN")
	if token == "" {
		return "dummy-token"
	}
	return token
}

func TestAlertEndpoints(t *testing.T) {
	baseURL := getBaseURL()
	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("List Alerts", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/api/v1/alerts", nil)
		req.Header.Set("Authorization", "Bearer "+getAccessToken())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call list alerts: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})

	t.Run("Create Alert Definition", func(t *testing.T) {
		payload := `{"name": "High Error Rate", "condition": "error_rate(spans{service_name=\"web\"}) > 5 for 5m", "severity": "CRITICAL"}`
		req, _ := http.NewRequest("POST", baseURL+"/api/v1/alerts", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+getAccessToken())

		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Failed to call create alert: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Unexpected status code: %d", resp.StatusCode)
		}
	})
}

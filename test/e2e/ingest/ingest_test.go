//go:build e2e

package ingest_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/Maitrimodii/apm_master/test/e2e/framework"
)

func TestIngestFlow(t *testing.T) {
	ctx := framework.SetupOrgAndUser(t)

	// Valid Payloads (Success cases)
	t.Run("Ingest Traces Success", func(t *testing.T) {
		payload := `{"resourceSpans": [{"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "e2e-service"}}]}, "scopeSpans": [{"spans": [{"traceId": "5b8aa5a2d2c872e8321cf37308d69df2", "spanId": "5159146c24388e67", "name": "e2e-span", "startTimeUnixNano": "1690000000000000000", "endTimeUnixNano": "1690000001000000000"}]}]}]}`
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/traces", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", ctx.APIKey)

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest traces: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 202/200 on trace ingest, got %d", resp.StatusCode)
		}
	})

	t.Run("Ingest Metrics Success", func(t *testing.T) {
		payload := `{"resourceMetrics": [{"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "e2e-service"}}]}, "scopeMetrics": [{"metrics": [{"name": "e2e_cpu_usage", "sum": {"dataPoints": [{"asDouble": 42.5, "timeUnixNano": "1690000000000000000"}]}}]}]}]}`
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/metrics", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", ctx.APIKey)

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest metrics: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 202/200 on metrics ingest, got %d", resp.StatusCode)
		}
	})

	t.Run("Ingest Logs Success", func(t *testing.T) {
		payload := `{"resourceLogs": [{"resource": {"attributes": [{"key": "service.name", "value": {"stringValue": "e2e-service"}}]}, "scopeLogs": [{"logRecords": [{"timeUnixNano": "1690000000000000000", "severityText": "INFO", "body": {"stringValue": "E2E Log Message"}}]}]}]}`
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/logs", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", ctx.APIKey)

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest logs: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 202/200 on logs ingest, got %d", resp.StatusCode)
		}
	})

	// Edge Cases
	t.Run("Ingest With Invalid API Key", func(t *testing.T) {
		payload := `{"resourceSpans": []}`
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/traces", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "invalid-or-fake-key")

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest with invalid key: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 on invalid api key, got %d", resp.StatusCode)
		}
	})

	t.Run("Ingest With Missing API Key", func(t *testing.T) {
		payload := `{"resourceSpans": []}`
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/traces", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest with missing key: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 on missing api key, got %d", resp.StatusCode)
		}
	})

	t.Run("Ingest Malformed Payload", func(t *testing.T) {
		payload := `{"resourceSpans": [ { invalid json... `
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/ingest/traces", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", ctx.APIKey)

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call ingest with malformed payload: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 on malformed payload, got %d", resp.StatusCode)
		}
	})
}

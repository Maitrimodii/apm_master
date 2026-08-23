package alert_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Maitrimodii/apm_master/test/e2e/framework"
)

func TestAlertFlow(t *testing.T) {
	ctxA := framework.SetupOrgAndUser(t)
	ctxB := framework.SetupOrgAndUser(t) // Second tenant for isolation checks

	var alertID string

	t.Run("Create Alert Definition Success", func(t *testing.T) {
		payload := `{"name": "E2E High Error Rate", "condition": "error_rate(spans{service_name=\"web\"}) > 5 for 5m", "severity": "CRITICAL"}`
		req, _ := http.NewRequest("POST", ctxA.BaseURL+"/api/v1/alerts", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ctxA.AccessToken)

		resp, err := ctxA.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call create alert: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected 201/200 on create alert, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if data, ok := result["data"].(map[string]interface{}); ok {
			if id, ok := data["id"].(string); ok {
				alertID = id
			}
		}
		if alertID == "" {
			t.Log("Warning: Could not extract alert ID from creation response. Isolation tests might skip.")
		}
	})

	t.Run("List Alerts Success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ctxA.BaseURL+"/api/v1/alerts", nil)
		req.Header.Set("Authorization", "Bearer "+ctxA.AccessToken)

		resp, err := ctxA.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call list alerts: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("Expected 200 on list alerts, got %d", resp.StatusCode)
		}
	})

	t.Run("Create Alert Invalid Syntax", func(t *testing.T) {
		payload := `{"name": "Bad Alert", "condition": "invalid syntax for an alert", "severity": "HIGH"}`
		req, _ := http.NewRequest("POST", ctxA.BaseURL+"/api/v1/alerts", bytes.NewBufferString(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+ctxA.AccessToken)

		resp, err := ctxA.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call create invalid alert: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 on invalid condition syntax, got %d", resp.StatusCode)
		}
	})

	t.Run("List Alerts Missing Token", func(t *testing.T) {
		req, _ := http.NewRequest("GET", ctxA.BaseURL+"/api/v1/alerts", nil)

		resp, err := ctxA.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call list alerts missing token: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 on missing token, got %d", resp.StatusCode)
		}
	})

	// Cross Tenant Isolation Tests
	if alertID != "" {
		t.Run("Cross Tenant Isolation - Fetch Alert", func(t *testing.T) {
			// Tenant B attempts to fetch Tenant A's alert
			req, _ := http.NewRequest("GET", ctxB.BaseURL+"/api/v1/alerts/"+alertID, nil)
			req.Header.Set("Authorization", "Bearer "+ctxB.AccessToken)

			resp, err := ctxB.HTTPClient.Do(req)
			if err != nil {
				t.Fatalf("Failed cross tenant fetch: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected 404/403 when fetching another org's alert, got %d", resp.StatusCode)
			}
		})

		t.Run("Cross Tenant Isolation - Delete Alert", func(t *testing.T) {
			// Tenant B attempts to delete Tenant A's alert
			req, _ := http.NewRequest("DELETE", ctxB.BaseURL+"/api/v1/alerts/"+alertID, nil)
			req.Header.Set("Authorization", "Bearer "+ctxB.AccessToken)

			resp, err := ctxB.HTTPClient.Do(req)
			if err != nil {
				t.Fatalf("Failed cross tenant delete: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusForbidden {
				t.Errorf("Expected 404/403 when deleting another org's alert, got %d", resp.StatusCode)
			}
		})
	} else {
		t.Log("Skipping cross tenant isolation tests because alertID could not be captured")
	}
}

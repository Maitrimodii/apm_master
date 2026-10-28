//go:build e2e

package auth_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/Maitrimodii/apm_master/test/e2e/framework"
	"github.com/google/uuid"
)

func TestAuthFlow(t *testing.T) {
	// 1. Success cases are covered implicitly by SetupOrgAndUser, but let's test the negative/edge cases explicitly.
	ctx := framework.SetupOrgAndUser(t)

	t.Run("Register With Existing Email", func(t *testing.T) {
		registerPayload := map[string]string{
			"org_name":   "Another Org",
			"org_slug":   "another-org-" + uuid.New().String()[:8],
			"first_name": "Test",
			"last_name":  "User",
			"email":      ctx.UserEmail, // Re-use the existing email
			"password":   "StrongPass123!",
		}
		body, _ := json.Marshal(registerPayload)
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call register: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 409/400 on register with existing email, got %d", resp.StatusCode)
		}
	})

	t.Run("Register With Weak Password", func(t *testing.T) {
		runID := uuid.New().String()[:8]
		registerPayload := map[string]string{
			"org_name":   "Weak Pass Org",
			"org_slug":   "weak-org-" + runID,
			"first_name": "Test",
			"last_name":  "User",
			"email":      fmt.Sprintf("weak-%s@example.com", runID),
			"password":   "123", // Weak password
		}
		body, _ := json.Marshal(registerPayload)
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/auth/register", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call register: %v", err)
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected 400 on register with weak password, got %d", resp.StatusCode)
		}
	})

	t.Run("Login With Incorrect Password", func(t *testing.T) {
		loginPayload := map[string]string{
			"org_slug": ctx.OrgSlug,
			"email":    ctx.UserEmail,
			"password": "WrongPassword!123",
		}
		body, _ := json.Marshal(loginPayload)
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		
		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call login: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 on login with incorrect password, got %d", resp.StatusCode)
		}
	})

	t.Run("Create API Key With Invalid Token", func(t *testing.T) {
		apiKeyPayload := map[string]string{
			"name": "invalid-token-key",
		}
		body, _ := json.Marshal(apiKeyPayload)
		req, _ := http.NewRequest("POST", ctx.BaseURL+"/api/v1/auth/api-keys", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer invalid.jwt.token")

		resp, err := ctx.HTTPClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to call create api key: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("Expected 401 on api key creation with invalid token, got %d", resp.StatusCode)
		}
	})
}

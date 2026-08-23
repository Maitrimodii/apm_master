package auth_test

import (
	"bytes"
	"encoding/json"
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

func TestAuthFlow(t *testing.T) {
	baseURL := getBaseURL()
	client := &http.Client{Timeout: 5 * time.Second}

	// 1. Register User
	registerPayload := map[string]string{
		"org_name":   "E2E Org",
		"org_slug":   "e2e-org-" + time.Now().Format("20060102150405"),
		"first_name": "E2E",
		"last_name":  "User",
		"email":      "user-" + time.Now().Format("20060102150405") + "@example.com",
		"password":   "StrongPass123!",
	}
	registerBody, _ := json.Marshal(registerPayload)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/register", bytes.NewBuffer(registerBody))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call register: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200/201 on register, got %d", resp.StatusCode)
	}

	// 2. Login User
	loginPayload := map[string]string{
		"org_slug": registerPayload["org_slug"],
		"email":    registerPayload["email"],
		"password": registerPayload["password"],
	}
	loginBody, _ := json.Marshal(loginPayload)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewBuffer(loginBody))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call login: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected 200 on login, got %d", resp.StatusCode)
	}

	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("Failed to decode login response: %v", err)
	}
	if loginResp.AccessToken == "" {
		t.Fatalf("Expected access token in login response")
	}

	// 3. Create API Key
	apiKeyPayload := map[string]string{
		"name": "e2e-api-key",
	}
	apiKeyBody, _ := json.Marshal(apiKeyPayload)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/auth/api-keys", bytes.NewBuffer(apiKeyBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to call create api key: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Errorf("Expected 200/201 on api key creation, got %d", resp.StatusCode)
	}
}

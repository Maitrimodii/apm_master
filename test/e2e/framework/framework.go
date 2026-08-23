package framework

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestContext holds the data for a single isolated tenant (org + user + tokens)
type TestContext struct {
	BaseURL     string
	HTTPClient  *http.Client
	
	OrgName     string
	OrgSlug     string
	UserEmail   string
	Password    string
	
	AccessToken string
	APIKey      string
}

// GetBaseURL returns the configured E2E URL or defaults to localhost:8080
func GetBaseURL() string {
	url := os.Getenv("APM_E2E_URL")
	if url == "" {
		url = "http://localhost:8080"
	}
	return url
}

// SetupOrgAndUser creates a completely fresh, isolated organization, user, JWT token, and API key.
func SetupOrgAndUser(t *testing.T) *TestContext {
	t.Helper()
	baseURL := GetBaseURL()
	client := &http.Client{Timeout: 10 * time.Second}
	
	// 1. Generate unique deterministic-per-run data
	runID := uuid.New().String()[:8]
	orgName := fmt.Sprintf("E2E Org %s", runID)
	orgSlug := fmt.Sprintf("e2e-org-%s", runID)
	email := fmt.Sprintf("user-%s@example.com", runID)
	password := "StrongPass123!"

	// 2. Register
	registerPayload := map[string]string{
		"org_name":   orgName,
		"org_slug":   orgSlug,
		"first_name": "E2E",
		"last_name":  "User",
		"email":      email,
		"password":   password,
	}
	body, _ := json.Marshal(registerPayload)
	req, _ := http.NewRequest("POST", baseURL+"/api/v1/auth/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Framework: Failed to register user: %v", err)
	}
	resp.Body.Close()
	
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("Framework: Expected 200/201 on register, got %d", resp.StatusCode)
	}

	// 3. Login
	loginPayload := map[string]string{
		"org_slug": orgSlug,
		"email":    email,
		"password": password,
	}
	body, _ = json.Marshal(loginPayload)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Framework: Failed to login: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Framework: Expected 200 on login, got %d", resp.StatusCode)
	}
	
	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		t.Fatalf("Framework: Failed to decode login response: %v", err)
	}
	
	// 4. Create API Key
	apiKeyPayload := map[string]string{
		"name": fmt.Sprintf("e2e-api-key-%s", runID),
	}
	body, _ = json.Marshal(apiKeyPayload)
	req, _ = http.NewRequest("POST", baseURL+"/api/v1/auth/api-keys", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+loginResp.AccessToken)
	
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Framework: Failed to create api key: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		t.Fatalf("Framework: Expected 200/201 on api key creation, got %d", resp.StatusCode)
	}
	
	var apiKeyResp struct {
		APIKey string `json:"api_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiKeyResp); err != nil {
		t.Fatalf("Framework: Failed to decode api key response: %v", err)
	}
	
	return &TestContext{
		BaseURL:     baseURL,
		HTTPClient:  client,
		OrgName:     orgName,
		OrgSlug:     orgSlug,
		UserEmail:   email,
		Password:    password,
		AccessToken: loginResp.AccessToken,
		APIKey:      apiKeyResp.APIKey,
	}
}

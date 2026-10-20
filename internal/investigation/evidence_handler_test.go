package investigation

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestServiceTokenAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		AIServiceToken: "valid-secret-token",
	}

	router := gin.New()
	handler := NewEvidenceHandler(nil, nil, nil, nil, cfg, testLogger())
	handler.RegisterRoutes(router.Group("/internal/v1"))

	// 1. Missing Authorization header -> 401
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/internal/v1/incidents/"+uuid.NewString()+"/context", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", w.Code)
	}

	// 2. Wrong token -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/internal/v1/incidents/"+uuid.NewString()+"/context", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong token, got %d", w.Code)
	}

	// 3. Invalid header format -> 401
	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodGet, "/internal/v1/incidents/"+uuid.NewString()+"/context", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer format, got %d", w.Code)
	}
}

func TestQueryEvidenceValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := &config.Config{
		AIServiceToken: "valid-secret-token",
	}

	router := gin.New()
	handler := NewEvidenceHandler(nil, nil, nil, nil, cfg, testLogger())
	handler.RegisterRoutes(router.Group("/internal/v1"))

	// 1. Time window: start > end -> 400
	now := time.Now()
	body := EvidenceQueryRequest{
		InvestigationID: uuid.New(),
		TenantID:        uuid.New(),
		QueryType:       "metric_baseline",
		TimeRangeStart:  now.Add(1 * time.Hour),
		TimeRangeEnd:    now,
	}
	bodyBytes, _ := json.Marshal(body)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/internal/v1/investigation-evidence/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer valid-secret-token")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for start > end, got %d", w.Code)
	}

	// 2. Time window > 24h -> 400
	body = EvidenceQueryRequest{
		InvestigationID: uuid.New(),
		TenantID:        uuid.New(),
		QueryType:       "metric_baseline",
		TimeRangeStart:  now,
		TimeRangeEnd:    now.Add(25 * time.Hour),
	}
	bodyBytes, _ = json.Marshal(body)

	w = httptest.NewRecorder()
	req, _ = http.NewRequest(http.MethodPost, "/internal/v1/investigation-evidence/query", bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer valid-secret-token")
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for window > 24h, got %d", w.Code)
	}
}

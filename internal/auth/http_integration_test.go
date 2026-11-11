package auth

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

// apiServer runs the real auth routes behind the real request logger, so
// these tests cover routing, JWT and permission middleware and access logs.
type apiServer struct {
	engine *gin.Engine
	module *Module
	logs   *bytes.Buffer
}

func (f *authFixture) apiServer(t *testing.T) *apiServer {
	t.Helper()
	gin.SetMode(gin.TestMode)
	logs := &bytes.Buffer{}
	module := NewModule(f.pool, nil, f.cfg, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	engine := gin.New()
	engine.Use(middleware.RequestID(), middleware.Logger(slog.New(slog.NewJSONHandler(logs, nil))))
	module.RegisterRoutes(engine.Group("/api/v1"))
	return &apiServer{engine: engine, module: module, logs: logs}
}

func (s *apiServer) do(t *testing.T, method, path, accessToken string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	rec := httptest.NewRecorder()
	s.engine.ServeHTTP(rec, req)
	return rec
}

func (f *authFixture) accessToken(t *testing.T, u testUser) string {
	t.Helper()
	return f.mustLogin(t, u).AccessToken
}

// settingsEndpoints are the authenticated invite and role operations.
func settingsEndpoints(someID uuid.UUID) [][2]string {
	id := someID.String()
	return [][2]string{
		{http.MethodPost, "/api/v1/auth/invite"},
		{http.MethodGet, "/api/v1/auth/invites"},
		{http.MethodPost, "/api/v1/auth/invites/" + id + "/resend"},
		{http.MethodDelete, "/api/v1/auth/invites/" + id},
		{http.MethodPost, "/api/v1/auth/roles"},
		{http.MethodPut, "/api/v1/auth/roles/" + id},
	}
}

func TestSettingsEndpointsRequireAuthentication(t *testing.T) {
	f := newAuthFixture(t)
	api := f.apiServer(t)
	for _, ep := range settingsEndpoints(uuid.New()) {
		if rec := api.do(t, ep[0], ep[1], "", map[string]any{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without a token = %d, want 401", ep[0], ep[1], rec.Code)
		}
		if rec := api.do(t, ep[0], ep[1], "not-a-jwt", map[string]any{}); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a bad token = %d, want 401", ep[0], ep[1], rec.Code)
		}
	}
}

func TestSettingsEndpointsRequirePermission(t *testing.T) {
	f := newAuthFixture(t)
	api := f.apiServer(t)
	admin := f.createAdmin(t, "http-perms")
	viewer := f.createMember(t, admin, "viewer", f.createRole(t, admin.orgID, "viewer", "traces:read"))
	token := f.accessToken(t, viewer)

	for _, ep := range settingsEndpoints(uuid.New()) {
		if rec := api.do(t, ep[0], ep[1], token, map[string]any{}); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s as a viewer = %d, want 403", ep[0], ep[1], rec.Code)
		}
	}
	// The admin passes the permission check on the same routes.
	if rec := api.do(t, http.MethodGet, "/api/v1/auth/invites", f.accessToken(t, admin), nil); rec.Code != http.StatusOK {
		t.Fatalf("GET /auth/invites as admin = %d: %s", rec.Code, rec.Body)
	}
}

// The org always comes from the access token: an admin cannot reach another
// org's invites by ID.
func TestInviteEndpointsAreTenantIsolated(t *testing.T) {
	f := newAuthFixture(t)
	api := f.apiServer(t)
	orgA := f.createAdmin(t, "http-tenant-a")
	orgB := f.createAdmin(t, "http-tenant-b")
	theirs, err := f.invite(t, orgB, "b@example.com", f.createRole(t, orgB.orgID, "viewer", "traces:read"))
	if err != nil {
		t.Fatal(err)
	}
	tokenA := f.accessToken(t, orgA)

	if rec := api.do(t, http.MethodDelete, "/api/v1/auth/invites/"+theirs.InviteID.String(), tokenA, nil); rec.Code != http.StatusNotFound {
		t.Errorf("cancel another org's invite = %d, want 404", rec.Code)
	}
	if rec := api.do(t, http.MethodPost, "/api/v1/auth/invites/"+theirs.InviteID.String()+"/resend", tokenA, nil); rec.Code != http.StatusNotFound {
		t.Errorf("resend another org's invite = %d, want 404", rec.Code)
	}
	rec := api.do(t, http.MethodGet, "/api/v1/auth/invites", tokenA, nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "b@example.com") {
		t.Errorf("org A's invite list = %d %s; must not include org B's invite", rec.Code, rec.Body)
	}
}

// Invite tokens appear in no API response and no access-log line, including
// the public lookup that carries the token in its query string.
func TestInviteTokenNeverInResponsesOrAccessLogs(t *testing.T) {
	f := newAuthFixture(t)
	api := f.apiServer(t)
	admin := f.createAdmin(t, "http-token")
	const token = "very-secret-invite-token"
	api.module.handler.invites.newToken = func() (string, error) { return token, nil }

	roleID := f.createRole(t, admin.orgID, "viewer", "traces:read")
	created := api.do(t, http.MethodPost, "/api/v1/auth/invite", f.accessToken(t, admin), map[string]any{
		"first_name": "Logged", "last_name": "Never", "email": "logged.never@example.com", "role_ids": []string{roleID.String()},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("POST /auth/invite = %d: %s", created.Code, created.Body)
	}
	lookup := api.do(t, http.MethodGet, "/api/v1/auth/invite?token="+token, "", nil)
	if lookup.Code != http.StatusOK {
		t.Fatalf("GET /auth/invite = %d: %s", lookup.Code, lookup.Body)
	}
	accepted := api.do(t, http.MethodPost, "/api/v1/auth/invite/accept", "", map[string]any{"token": token, "password": "logged-password"})
	if accepted.Code != http.StatusOK {
		t.Fatalf("POST /auth/invite/accept = %d: %s", accepted.Code, accepted.Body)
	}

	for name, body := range map[string]string{
		"create response": created.Body.String(),
		"lookup response": lookup.Body.String(),
		"accept response": accepted.Body.String(),
		"access log":      api.logs.String(),
	} {
		if strings.Contains(body, token) || strings.Contains(body, "invite_link") {
			t.Errorf("%s exposes the invite token: %s", name, body)
		}
	}
	if !strings.Contains(api.logs.String(), `"path":"/api/v1/auth/invite"`) {
		t.Fatalf("expected the lookup in the access log (without its query): %s", api.logs.String())
	}
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Maitrimodii/apm_master/internal/platform/httpx"
)

func serve(t *testing.T, mw gin.HandlerFunc, method string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(mw)
	r.Any("/x", func(c *gin.Context) { c.String(http.StatusOK, c.GetString(httpx.CtxKeyCorrelationID)) })
	req := httptest.NewRequest(method, "/x", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCORSAllowsOnlyListedOrigins(t *testing.T) {
	mw := CORS([]string{"https://app.example.com/"})

	w := serve(t, mw, http.MethodGet, map[string]string{"Origin": "https://app.example.com"})
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("allowed origin: Allow-Origin = %q", got)
	}

	w = serve(t, mw, http.MethodGet, map[string]string{"Origin": "https://evil.example"})
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unlisted origin: Allow-Origin = %q, want empty", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("unlisted origin: Allow-Credentials = %q, want empty", got)
	}

	w = serve(t, mw, http.MethodOptions, map[string]string{"Origin": "https://app.example.com"})
	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
}

func TestServiceTokenAuth(t *testing.T) {
	mw := ServiceTokenAuth("secret-token")
	tests := []struct {
		header string
		want   int
	}{
		{"Bearer secret-token", http.StatusOK},
		{"Bearer wrong", http.StatusUnauthorized},
		{"secret-token", http.StatusUnauthorized},
		{"", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		w := serve(t, mw, http.MethodGet, map[string]string{httpx.HeaderAuthorization: tc.header})
		if w.Code != tc.want {
			t.Errorf("Authorization %q: status = %d, want %d", tc.header, w.Code, tc.want)
		}
	}
}

func TestCorrelationID(t *testing.T) {
	id := uuid.NewString()
	w := serve(t, CorrelationID(), http.MethodGet, map[string]string{httpx.HeaderCorrelationID: id})
	if w.Body.String() != id || w.Header().Get(httpx.HeaderCorrelationID) != id {
		t.Errorf("valid id not propagated: body %q header %q", w.Body.String(), w.Header().Get(httpx.HeaderCorrelationID))
	}

	w = serve(t, CorrelationID(), http.MethodGet, map[string]string{httpx.HeaderCorrelationID: "not-a-uuid"})
	if _, err := uuid.Parse(w.Body.String()); err != nil || w.Body.String() == "not-a-uuid" {
		t.Errorf("invalid id should be replaced with a new UUID, got %q", w.Body.String())
	}
}

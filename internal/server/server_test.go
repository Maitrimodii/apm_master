package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/Maitrimodii/apm_master/internal/config"
)

// clientIPSeen serves any request and reports the client IP the server
// resolved, the same value rate limiting and request logs use.
func clientIPSeen(t *testing.T, proxies []string, remoteAddr string, headers map[string]string) string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	srv, err := NewServer(ServerParams{
		HTTP: config.HTTPConfig{TrustedProxies: proxies},
		Middlewares: []gin.HandlerFunc{func(c *gin.Context) {
			c.Header("X-Seen-Client-IP", c.ClientIP())
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return rec.Header().Get("X-Seen-Client-IP")
}

func TestSpoofedForwardedHeadersAreIgnoredByDefault(t *testing.T) {
	spoofed := map[string]string{"X-Forwarded-For": "1.2.3.4", "X-Real-IP": "5.6.7.8"}
	if got := clientIPSeen(t, nil, "203.0.113.7:51000", spoofed); got != "203.0.113.7" {
		t.Fatalf("client IP = %q, want the peer address 203.0.113.7", got)
	}
}

func TestForwardedHeadersTrustedOnlyFromConfiguredProxies(t *testing.T) {
	proxies := []string{"10.0.0.0/8"}
	headers := map[string]string{"X-Forwarded-For": "198.51.100.20"}

	if got := clientIPSeen(t, proxies, "10.1.2.3:443", headers); got != "198.51.100.20" {
		t.Errorf("through a trusted proxy: client IP = %q, want the forwarded 198.51.100.20", got)
	}
	if got := clientIPSeen(t, proxies, "203.0.113.7:443", headers); got != "203.0.113.7" {
		t.Errorf("from an untrusted peer: client IP = %q, want the peer 203.0.113.7", got)
	}
	// A client prepending a fake hop is still resolved to its real address:
	// the right-most untrusted entry is the client.
	chained := map[string]string{"X-Forwarded-For": "1.2.3.4, 198.51.100.20"}
	if got := clientIPSeen(t, proxies, "10.1.2.3:443", chained); got != "198.51.100.20" {
		t.Errorf("chained header: client IP = %q, want 198.51.100.20", got)
	}
}

func TestInvalidTrustedProxyIsRejected(t *testing.T) {
	if _, err := NewServer(ServerParams{HTTP: config.HTTPConfig{TrustedProxies: []string{"not-an-ip"}}}); err == nil {
		t.Fatal("NewServer accepted an invalid trusted proxy")
	}
}

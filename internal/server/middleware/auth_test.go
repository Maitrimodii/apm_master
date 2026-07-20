package middleware

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fakeTokenParser struct {
	userID string
	orgID  string
	email  string
	err    error
}

func (p fakeTokenParser) ParseToken(tokenString string) (userID, orgID, email string, err error) {
	if p.err != nil {
		return "", "", "", p.err
	}
	return p.userID, p.orgID, p.email, nil
}

type fakeAPIKeyValidator struct {
	orgID uuid.UUID
	err   error
}

func (value fakeAPIKeyValidator) ValidateAPIKey(ctx context.Context, rawToken string) (uuid.UUID, error) {
	if value.err != nil {
		return uuid.UUID{}, value.err
	}
	return value.orgID, nil
}

type fakePermissionChecker struct {
	has       bool
	err       error
	lastOrgID uuid.UUID
	seen      bool
}

func (ginContext *fakePermissionChecker) CheckPermission(ctx context.Context, userID uuid.UUID, orgID uuid.UUID, name string, action models.PermissionAction) (bool, error) {
	ginContext.lastOrgID = orgID
	ginContext.seen = true
	if ginContext.err != nil {
		return false, ginContext.err
	}
	return ginContext.has, nil
}

func TestJWTAuth(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	userID := uuid.New().String()
	orgID := uuid.New().String()
	router := gin.New()
	router.GET("/protected", JWTAuth(fakeTokenParser{
		userID: userID,
		orgID:  orgID,
		email:  "user@example.com",
	}), func(ginContext *gin.Context) {
		if ginContext.GetString("user_id") != userID {
			testRunner.Fatalf("user_id = %q, want %q", ginContext.GetString("user_id"), userID)
		}
		ginContext.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}

func TestJWTAuthRejectsMissingHeader(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/protected", JWTAuth(fakeTokenParser{}), func(ginContext *gin.Context) {
		testRunner.Fatal("handler should not be called")
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if resp.Code != http.StatusUnauthorized {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestAPIKeyAuth(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	orgID := uuid.New()
	router := gin.New()
	router.POST("/ingest", APIKeyAuth(fakeAPIKeyValidator{orgID: orgID}), func(ginContext *gin.Context) {
		if ginContext.GetString("org_id") != orgID.String() {
			testRunner.Fatalf("org_id = %q, want %q", ginContext.GetString("org_id"), orgID.String())
		}
		ginContext.Status(http.StatusAccepted)
	})

	req := httptest.NewRequest(http.MethodPost, "/ingest", nil)
	req.Header.Set("X-API-Key", "apm_sk_test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusAccepted)
	}
}

func TestAPIKeyAuthRejectsInvalidKey(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/ingest", APIKeyAuth(fakeAPIKeyValidator{err: errors.New("invalid key")}), func(ginContext *gin.Context) {
		testRunner.Fatal("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodPost, "/ingest", nil)
	req.Header.Set("X-API-Key", "apm_sk_test")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}

func TestRequirePermission(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := &fakePermissionChecker{has: true}
	router := gin.New()
	router.GET("/roles", func(ginContext *gin.Context) {
		ginContext.Set("user_id", uuid.New().String())
		ginContext.Set("org_id", uuid.New().String())
	}, RequirePermission(checker, "role", models.Read), func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/roles", nil))

	if resp.Code != http.StatusNoContent {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
}

func TestRequirePermissionPassesOrgID(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := &fakePermissionChecker{has: true}
	orgID := uuid.New()
	router := gin.New()
	router.GET("/roles", func(ginContext *gin.Context) {
		ginContext.Set("user_id", uuid.New().String())
		ginContext.Set("org_id", orgID.String())
	}, RequirePermission(checker, "role", models.Read), func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/roles", nil))

	if resp.Code != http.StatusNoContent {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusNoContent)
	}
	if !checker.seen {
		testRunner.Fatal("expected permission checker to be called")
	}
	if checker.lastOrgID != orgID {
		testRunner.Fatalf("org_id = %s, want %s", checker.lastOrgID, orgID)
	}
}

func TestRequirePermissionRejectsDeniedAccess(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	checker := &fakePermissionChecker{has: false}
	router := gin.New()
	router.GET("/roles", func(ginContext *gin.Context) {
		ginContext.Set("user_id", uuid.New().String())
		ginContext.Set("org_id", uuid.New().String())
	}, RequirePermission(checker, "role", models.Read), func(ginContext *gin.Context) {
		testRunner.Fatal("handler should not be called")
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/roles", nil))

	if resp.Code != http.StatusForbidden {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusForbidden)
	}
}

func TestRequestIDPreservesIncomingHeader(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/health", RequestID(), func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(RequestIDHeader, "req-123")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if got := resp.Header().Get(RequestIDHeader); got != "req-123" {
		testRunner.Fatalf("%s = %q, want req-123", RequestIDHeader, got)
	}
}

func TestRequestIDGeneratesMissingHeader(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/health", RequestID(), func(ginContext *gin.Context) {
		ginContext.Status(http.StatusNoContent)
	})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got := resp.Header().Get(RequestIDHeader); got == "" {
		testRunner.Fatalf("%s should be generated", RequestIDHeader)
	}
}

func TestBodyLimitCapsRequestBody(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/payload", BodyLimit(4), func(ginContext *gin.Context) {
		_, err := io.ReadAll(ginContext.Request.Body)
		if err == nil {
			testRunner.Fatal("expected body read to fail after limit")
		}
		ginContext.Status(http.StatusRequestEntityTooLarge)
	})

	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/payload", strings.NewReader("12345"))
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusRequestEntityTooLarge {
		testRunner.Fatalf("status = %d, want %d", resp.Code, http.StatusRequestEntityTooLarge)
	}
}

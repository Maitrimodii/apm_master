package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterOrgOwnerRequestBindingDoesNotPanic(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"org_name":"Smoke Org","org_slug":"smoke","first_name":"Smoke","last_name":"User","email":"smoke@example.com","password":"Password123!"}`
	responseWriter := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(responseWriter)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	var req RegisterOrgOwnerRequest
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()

		if err := c.ShouldBindJSON(&req); err != nil {
			testRunner.Fatalf("binding failed: %v", err)
		}
	}()

	if panicked {
		testRunner.Fatal("binding panicked")
	}

	if req.Email != "smoke@example.com" {
		testRunner.Fatalf("email = %q, want smoke@example.com", req.Email)
	}
}

func TestLoginRequestBindingDoesNotPanic(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	body := `{"org_slug":"smoke","email":"smoke@example.com","password":"Password123!"}`
	responseWriter := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(responseWriter)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	var req LoginRequest
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()

		if err := c.ShouldBindJSON(&req); err != nil {
			testRunner.Fatalf("binding failed: %v", err)
		}
	}()

	if panicked {
		testRunner.Fatal("binding panicked")
	}

	if req.OrgSlug != "smoke" {
		testRunner.Fatalf("org_slug = %q, want smoke", req.OrgSlug)
	}
}

// GET /auth/invite carries the token in the query string. Binding must read
// the lowercase ?token= parameter the invite email and the UI send.
func TestGetInviteRequestBindsTokenFromQuery(testRunner *testing.T) {
	gin.SetMode(gin.TestMode)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/invite?token=abc123", nil)

	var req GetInviteRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		testRunner.Fatalf("ShouldBindQuery() error = %v", err)
	}
	if req.Token != "abc123" {
		testRunner.Fatalf("token = %q, want abc123", req.Token)
	}
}

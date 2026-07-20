package dto

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
	ginContext, _ := gin.CreateTestContext(responseWriter)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	ginContext.Request.Header.Set("Content-Type", "application/json")

	var req RegisterOrgOwnerRequest
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()

		if err := ginContext.ShouldBindJSON(&req); err != nil {
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
	ginContext, _ := gin.CreateTestContext(responseWriter)
	ginContext.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	ginContext.Request.Header.Set("Content-Type", "application/json")

	var req LoginRequest
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()

		if err := ginContext.ShouldBindJSON(&req); err != nil {
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

package services

import (
	"strings"
	"testing"
	"time"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGenerateAccessTokenIncludesExpectedClaims(testRunner *testing.T) {
	user := models.User{
		ID:    uuid.New(),
		OrgID: uuid.New(),
		Email: "owner@example.com",
	}
	secret := "0123456789abcdef0123456789abcdef"

	tokenString, err := GenerateAccessToken(user, 15*time.Minute, secret)
	if err != nil {
		testRunner.Fatalf("GenerateAccessToken() error = %v", err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil {
		testRunner.Fatalf("ParseWithClaims() error = %v", err)
	}
	if !token.Valid {
		testRunner.Fatal("expected token to be valid")
	}

	if claims.UserID != user.ID.String() {
		testRunner.Fatalf("UserID = %q, want %q", claims.UserID, user.ID.String())
	}
	if claims.OrgID != user.OrgID.String() {
		testRunner.Fatalf("OrgID = %q, want %q", claims.OrgID, user.OrgID.String())
	}
	if claims.Email != user.Email {
		testRunner.Fatalf("Email = %q, want %q", claims.Email, user.Email)
	}
}

func TestGenerateTokenAndAPIKey(testRunner *testing.T) {
	token, err := GenerateToken()
	if err != nil {
		testRunner.Fatalf("GenerateToken() error = %v", err)
	}
	if token == "" {
		testRunner.Fatal("GenerateToken() returned empty token")
	}

	hash := HashToken(token)
	if len(hash) != 64 {
		testRunner.Fatalf("HashToken() length = %d, want 64", len(hash))
	}
	if hash != HashToken(token) {
		testRunner.Fatal("HashToken() should be deterministic")
	}

	rawKey, prefix, err := GenerateAPIKey()
	if err != nil {
		testRunner.Fatalf("GenerateAPIKey() error = %v", err)
	}
	if !strings.HasPrefix(rawKey, "apm_sk_") {
		testRunner.Fatalf("raw key = %q, want apm_sk_ prefix", rawKey)
	}
	if prefix != rawKey[:12] {
		testRunner.Fatalf("prefix = %q, want %q", prefix, rawKey[:12])
	}
}

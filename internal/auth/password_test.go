package auth

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

func TestHashPasswordEnforcesByteLimit(t *testing.T) {
	euro := "€" // 3 bytes in UTF-8
	cases := []struct {
		name     string
		password string
		ok       bool
	}{
		{"72 ASCII bytes", strings.Repeat("a", 72), true},
		{"73 ASCII bytes", strings.Repeat("a", 73), false},
		// 24 characters but 72 bytes: at the limit.
		{"24 three-byte characters", strings.Repeat(euro, 24), true},
		// Only 25 characters, yet 75 bytes: over the limit. A character
		// count (what binding:"max=" would check) would accept it.
		{"25 three-byte characters", strings.Repeat(euro, 25), false},
		{"emoji over the limit", strings.Repeat("🔑", 19), false}, // 4 bytes each = 76
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := hashPassword(tc.password)
			if tc.ok {
				if err != nil {
					t.Fatalf("hashPassword(%d bytes) error = %v", len(tc.password), err)
				}
				// The whole password is significant: no silent truncation.
				if bcrypt.CompareHashAndPassword([]byte(hash), []byte(tc.password)) != nil {
					t.Fatal("hash does not match the password")
				}
				return
			}
			var appErr *apperr.Error
			if !errors.As(err, &appErr) || appErr.Code != apperr.CodeValidation || appErr.Data["field"] != "password" {
				t.Fatalf("hashPassword(%d bytes) error = %v, want a password validation error", len(tc.password), err)
			}
		})
	}
}

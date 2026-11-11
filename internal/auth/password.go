package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/Maitrimodii/apm_master/internal/platform/apperr"
)

// maxPasswordBytes is bcrypt's input limit. bcrypt ignores everything after
// it, so the library refuses longer input rather than silently truncating;
// we check first so the caller gets a validation error, not a 500. The limit
// is in bytes: a password of multi-byte characters reaches it sooner.
const maxPasswordBytes = 72

// hashPassword validates a new password's length and bcrypt-hashes it. The
// minimum length is enforced by the request DTOs (binding:"min=8").
func hashPassword(password string) (string, error) {
	if len(password) > maxPasswordBytes {
		return "", apperr.ValidationError("password", fmt.Sprintf("must be at most %d bytes", maxPasswordBytes))
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", apperr.Internal(fmt.Errorf("hash password: %w", err))
	}
	return string(hash), nil
}

package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/Maitrimodii/apm_master/internal/auth/models"
	"github.com/golang-jwt/jwt/v5"
)

func GenerateAccessToken(
	user models.User,
	expiryTime time.Duration,
	secret string,
) (string, error) {
	now := time.Now()

	expiry := now.Add(expiryTime)

	claims := Claims{
		UserID: user.ID.String(),
		OrgID:  user.OrgID.String(),
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiry),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	signedToken, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf(
			"generate access token: %w",
			err,
		)
	}

	return signedToken, nil
}

func GenerateToken() (
	string,
	error,
) {
	randStr := make([]byte, 32)

	_, err := rand.Read(randStr)
	if err != nil {
		return "", fmt.Errorf(
			"generate random string: %w",
			err,
		)
	}

	return base64.RawURLEncoding.EncodeToString(randStr), nil
}

func HashToken(
	token string,
) string {
	hash := sha256.Sum256(
		[]byte(token),
	)

	return hex.EncodeToString(
		hash[:],
	)
}

func GenerateAPIKey() (
	rawKey string,
	prefix string,
	err error,
) {
	token, err := GenerateToken()
	if err != nil {
		return "", "", fmt.Errorf(
			"generate API key: %w",
			err,
		)
	}

	rawKey = "apm_sk_" + token

	if len(rawKey) >= 12 {
		prefix = rawKey[:12]
	} else {
		prefix = rawKey
	}

	return rawKey, prefix, nil
}

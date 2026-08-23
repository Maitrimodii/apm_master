package logger

import (
	"context"
	"log/slog"
	"testing"
)

func TestNewHonorsLogLevel(testRunner *testing.T) {
	log := New("apm_master", "test", "debug")

	if !log.Enabled(context.Background(), slog.LevelDebug) {
		testRunner.Fatal("expected debug logs to be enabled")
	}
}

func TestNewDefaultsToInfoLevel(testRunner *testing.T) {
	log := New("apm_master", "test", "invalid")

	if log.Enabled(context.Background(), slog.LevelDebug) {
		testRunner.Fatal("debug logs should be disabled for invalid level fallback")
	}
	if !log.Enabled(context.Background(), slog.LevelInfo) {
		testRunner.Fatal("info logs should be enabled for invalid level fallback")
	}
}

func TestSetLevelChangesLevel(testRunner *testing.T) {
	log := New("apm_master", "test", "info")

	if log.Enabled(context.Background(), slog.LevelDebug) {
		testRunner.Fatal("expected debug logs to be disabled initially")
	}

	err := SetLevel("debug")
	if err != nil {
		testRunner.Fatalf("expected no error setting level, got %v", err)
	}

	if !log.Enabled(context.Background(), slog.LevelDebug) {
		testRunner.Fatal("expected debug logs to be enabled after SetLevel")
	}

	err = SetLevel("invalid")
	if err == nil {
		testRunner.Fatal("expected error setting invalid level")
	}
}

func TestRedactorMasksSensitiveKeys(testRunner *testing.T) {
	// Initialize redactor with default SensitiveKeys and one custom key
	customKeys := []string{"custom_key"}
	allKeys := append(append([]string{}, SensitiveKeys...), customKeys...)
	r := redactor(allKeys)

	tests := []struct {
		key      string
		val      string
		expected string
	}{
		// Default SensitiveKeys checks
		{key: "recipient", val: "user@example.com", expected: "[REDACTED]"},
		{key: "email", val: "user@example.com", expected: "[REDACTED]"},
		{key: "token", val: "my-token", expected: "[REDACTED]"},
		{key: "password", val: "my-password", expected: "[REDACTED]"},
		{key: "api_key", val: "my-key", expected: "[REDACTED]"},
		{key: "access_token", val: "my-token", expected: "[REDACTED]"},
		{key: "refresh_token", val: "my-token", expected: "[REDACTED]"},
		{key: "key", val: "my-key", expected: "[REDACTED]"},
		{key: "password_hash", val: "my-hash", expected: "[REDACTED]"},
		{key: "token_hash", val: "my-hash", expected: "[REDACTED]"},
		{key: "key_hash", val: "my-hash", expected: "[REDACTED]"},
		
		// Custom key check
		{key: "custom_key", val: "custom-val", expected: "[REDACTED]"},

		// Non-sensitive keys check
		{key: "username", val: "john_doe", expected: "john_doe"},
		{key: "status", val: "active", expected: "active"},
	}

	for _, tc := range tests {
		attr := r(nil, slog.String(tc.key, tc.val))
		if attr.Value.String() != tc.expected {
			testRunner.Errorf("key %q: expected value %q, got %q", tc.key, tc.expected, attr.Value.String())
		}
	}
}

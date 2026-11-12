package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testNotificationKey is a fixed 32-byte test key (base64), not a secret.
var testNotificationKey = base64.StdEncoding.EncodeToString([]byte(strings.Repeat("t", 32)))

func validEnv() map[string]string {
	return map[string]string{
		"POSTGRES_PASSWORD":              "secret",
		"JWT_SECRET":                     strings.Repeat("k", 32),
		"SMTP_HOST":                      "smtp.example.com",
		"NOTIFICATION_ENCRYPTION_KEYS":   "test:" + testNotificationKey,
		"NOTIFICATION_ENCRYPTION_KEY_ID": "test",
	}
}

// validProductionEnv satisfies every rule that applies outside development.
func validProductionEnv() map[string]string {
	env := validEnv()
	env["APP_ENV"] = "production"
	env["AI_SERVICE_TOKEN"] = strings.Repeat("a", 40)
	env["REDIS_URL"] = "redis://:" + strings.Repeat("r", 24) + "@redis:6379/0"
	env["SMTP_USERNAME"] = "mailer"
	env["SMTP_PASSWORD"] = "mail-password"
	return env
}

// validationError returns Validate's error for env, or "" if it passes.
func validationError(t *testing.T, env map[string]string) string {
	t.Helper()
	cfg, err := parse(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err != nil {
		return err.Error()
	}
	return ""
}

func TestValidProductionEnvPasses(t *testing.T) {
	if msg := validationError(t, validProductionEnv()); msg != "" {
		t.Fatalf("Validate() = %s", msg)
	}
}

func TestNotificationKeysRequiredInEveryEnvironment(t *testing.T) {
	for _, appEnv := range []string{"development", "test", "production"} {
		env := validProductionEnv()
		env["APP_ENV"] = appEnv
		delete(env, "NOTIFICATION_ENCRYPTION_KEYS")
		if msg := validationError(t, env); !strings.Contains(msg, "NOTIFICATION_ENCRYPTION_KEYS") {
			t.Errorf("%s without keys: Validate() = %q, want a key error", appEnv, msg)
		}
	}
	env := validEnv()
	env["NOTIFICATION_ENCRYPTION_KEYS"] = "test:" + base64.StdEncoding.EncodeToString([]byte("short"))
	if msg := validationError(t, env); !strings.Contains(msg, "must be 32 bytes") {
		t.Errorf("short key: Validate() = %q", msg)
	}
	env = validEnv()
	env["NOTIFICATION_ENCRYPTION_KEY_ID"] = "missing"
	if msg := validationError(t, env); !strings.Contains(msg, "not among the configured keys") {
		t.Errorf("unknown active key: Validate() = %q", msg)
	}
	// The scheduler sends email too and needs the same keys.
	env = validEnv()
	env["APP_MODE"] = "scheduler"
	delete(env, "NOTIFICATION_ENCRYPTION_KEYS")
	cfg, err := parse(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.ValidateScheduler(); err == nil || !strings.Contains(err.Error(), "NOTIFICATION_ENCRYPTION_KEYS") {
		t.Errorf("ValidateScheduler without keys = %v", err)
	}
}

func TestStaleProcessingThresholdHasAFloor(t *testing.T) {
	env := validEnv()
	env["NOTIFICATION_STALE_PROCESSING_MINUTES"] = "1"
	if msg := validationError(t, env); !strings.Contains(msg, "NOTIFICATION_STALE_PROCESSING_MINUTES") {
		t.Fatalf("Validate() = %q, want a stale-processing error", msg)
	}
}

func TestRedisAuthenticationRequiredOutsideDevelopment(t *testing.T) {
	for url, want := range map[string]string{
		"redis:6379":                  "must include a password",
		"redis://redis:6379/0":        "must include a password",
		"redis://:short@redis:6379/0": "at least 16 characters",
		"redis://user:@redis:6379/0":  "must include a password",
	} {
		env := validProductionEnv()
		env["REDIS_URL"] = url
		if msg := validationError(t, env); !strings.Contains(msg, want) {
			t.Errorf("production REDIS_URL %q: Validate() = %q, want %q", url, msg, want)
		}
	}
	// Development may use a loopback Redis without a password.
	env := validEnv()
	env["REDIS_URL"] = "localhost:6379"
	if msg := validationError(t, env); msg != "" {
		t.Errorf("development without a Redis password: Validate() = %s", msg)
	}
}

func TestTrustedProxiesAreValidated(t *testing.T) {
	env := validEnv()
	env["TRUSTED_PROXIES"] = "10.0.0.0/8, 192.168.1.10,::1"
	if msg := validationError(t, env); msg != "" {
		t.Fatalf("valid proxies: Validate() = %s", msg)
	}
	env["TRUSTED_PROXIES"] = "10.0.0.0/8,proxy.internal"
	if msg := validationError(t, env); !strings.Contains(msg, `TRUSTED_PROXIES entry "proxy.internal"`) {
		t.Fatalf("invalid proxy: Validate() = %q", msg)
	}
	env["TRUSTED_PROXIES"] = "10.0.0.0/33"
	if msg := validationError(t, env); !strings.Contains(msg, "TRUSTED_PROXIES") {
		t.Fatalf("invalid CIDR: Validate() = %q", msg)
	}
}

func TestSMTPSecurityPolicy(t *testing.T) {
	cases := []struct {
		name string
		edit func(env map[string]string)
		want string // "" means valid
	}{
		{"production with credentials", func(map[string]string) {}, ""},
		{"production insecure", func(e map[string]string) { e["SMTP_ALLOW_INSECURE"] = "true" }, "SMTP_ALLOW_INSECURE=true is only allowed"},
		{"production without credentials", func(e map[string]string) { delete(e, "SMTP_USERNAME"); delete(e, "SMTP_PASSWORD") }, "SMTP_TRUSTED_RELAY"},
		{"production trusted relay", func(e map[string]string) {
			delete(e, "SMTP_USERNAME")
			delete(e, "SMTP_PASSWORD")
			e["SMTP_TRUSTED_RELAY"] = "true"
		}, ""},
		{"username without password", func(e map[string]string) { delete(e, "SMTP_PASSWORD") }, "must be set together"},
		{"development mail catcher", func(e map[string]string) {
			e["APP_ENV"] = "development"
			e["SMTP_ALLOW_INSECURE"] = "true"
			delete(e, "SMTP_USERNAME")
			delete(e, "SMTP_PASSWORD")
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := validProductionEnv()
			tc.edit(env)
			msg := validationError(t, env)
			if tc.want == "" && msg != "" {
				t.Fatalf("Validate() = %s, want valid", msg)
			}
			if tc.want != "" && !strings.Contains(msg, tc.want) {
				t.Fatalf("Validate() = %q, want %q", msg, tc.want)
			}
		})
	}
}

func TestParseUsesDefaultsAndMilliseconds(t *testing.T) {
	env := validEnv()
	env["HTTP_READ_TIMEOUT_MS"] = "2500"
	env["KAFKA_BROKERS"] = "a:9092, b:9092"
	cfg, err := parse(env)
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.HTTP.ReadTimeout != 2500*time.Millisecond {
		t.Errorf("ReadTimeout = %v, want 2.5s", cfg.HTTP.ReadTimeout)
	}
	if cfg.HTTP.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout default = %v, want 30s", cfg.HTTP.WriteTimeout)
	}
	if len(cfg.Kafka.Brokers) != 2 || cfg.Kafka.Brokers[1] != "b:9092" {
		t.Errorf("Brokers = %q", cfg.Kafka.Brokers)
	}
	if cfg.Kafka.TracesTopic != "apm.telemetry.traces.v1" {
		t.Errorf("TracesTopic default = %q, want the plural name documented in .env.example", cfg.Kafka.TracesTopic)
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error = %v", err)
	}
}

func TestParseRejectsMalformedNumbers(t *testing.T) {
	env := validEnv()
	env["HTTP_READ_TIMEOUT_MS"] = "ten seconds"
	if _, err := parse(env); err == nil {
		t.Fatal("parse() error = nil for a non-numeric *_MS value; it must not fall back to the default")
	}
}

// B01: values followed by an inline comment in .env used to be dropped.
func TestLoadReadsEnvFileWithInlineComments(t *testing.T) {
	dir := t.TempDir()
	content := "MAX_BODY_BYTES=2048 # 2KB\nALERT_EVALUATION_INTERVAL_MS=5000 # 5s\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	os.Unsetenv("MAX_BODY_BYTES")
	os.Unsetenv("ALERT_EVALUATION_INTERVAL_MS")
	t.Cleanup(func() {
		os.Unsetenv("MAX_BODY_BYTES")
		os.Unsetenv("ALERT_EVALUATION_INTERVAL_MS")
	})

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.HTTP.MaxBodyBytes != 2048 {
		t.Errorf("MaxBodyBytes = %d, want 2048", cfg.HTTP.MaxBodyBytes)
	}
	if cfg.Alerting.EvaluationInterval != 5*time.Second {
		t.Errorf("EvaluationInterval = %v, want 5s", cfg.Alerting.EvaluationInterval)
	}
}

func TestAppURLPrefersNewName(t *testing.T) {
	env := validEnv()
	env["APPURL"] = "http://old"
	env["APP_URL"] = "http://new"
	cfg, err := parse(env)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.App.URL != "http://new" {
		t.Errorf("App.URL = %q, want http://new", cfg.App.URL)
	}
	if got := cfg.App.Origins(); len(got) != 1 || got[0] != "http://new" {
		t.Errorf("Origins() = %v, want the app URL by default", got)
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	env := validEnv()
	env["JWT_SECRET"] = "short"
	env["APP_MODE"] = "everything"
	env["KAFKA_COMPRESSION"] = "brotli"
	cfg, err := parse(env)
	if err != nil {
		t.Fatal(err)
	}
	err = cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	for _, want := range []string{"JWT_SECRET", "APP_MODE", "KAFKA_COMPRESSION"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error %q does not mention %s", err, want)
		}
	}
}

func TestRefreshSessionMaxDays(t *testing.T) {
	cfg, err := parse(validEnv())
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Auth.RefreshSessionMaxTTL(); got != 30*24*time.Hour {
		t.Errorf("RefreshSessionMaxTTL() default = %v, want 30 days", got)
	}

	env := validEnv()
	env["REFRESH_TOKEN_EXPIRY_DAYS"] = "14"
	env["REFRESH_SESSION_MAX_DAYS"] = "7"
	cfg, err = parse(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "REFRESH_SESSION_MAX_DAYS") {
		t.Fatalf("Validate() = %v, want a REFRESH_SESSION_MAX_DAYS error when it is shorter than a token", err)
	}
}

func TestValidateRejectsPlaceholderTokenOutsideDevelopment(t *testing.T) {
	env := validEnv()
	env["APP_ENV"] = "production"
	cfg, err := parse(env)
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "AI_SERVICE_TOKEN") {
		t.Fatalf("Validate() = %v, want an AI_SERVICE_TOKEN error in production", err)
	}
	env["APP_ENV"] = "development"
	cfg, _ = parse(env)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() in development = %v, want nil", err)
	}
}

func TestAutoMigrateDefaultsToDevelopmentOnly(t *testing.T) {
	for envName, want := range map[string]bool{"development": true, "production": false} {
		env := validEnv()
		env["APP_ENV"] = envName
		cfg, _ := parse(env)
		if got := cfg.App.ShouldAutoMigrate(); got != want {
			t.Errorf("APP_ENV=%s: ShouldAutoMigrate() = %v, want %v", envName, got, want)
		}
	}
}

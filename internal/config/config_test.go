package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validEnv() map[string]string {
	return map[string]string{
		"POSTGRES_PASSWORD": "secret",
		"JWT_SECRET":        strings.Repeat("k", 32),
		"SMTP_HOST":         "smtp.example.com",
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

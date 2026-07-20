package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	envAppURL   = "APPURL"
	envAppEnv   = "APP_ENV"
	envLogLevel = "LOG_LEVEL"

	envHTTPPort          = "HTTP_PORT"
	envHTTPReadTimeout   = "HTTP_READ_TIMEOUT_MS"
	envHTTPWriteTimeout  = "HTTP_WRITE_TIMEOUT_MS"
	envHTTPIdleTimeout   = "HTTP_IDLE_TIMEOUT_MS"
	envHTTPHeaderTimeout = "HTTP_HEADER_TIMEOUT_MS"
	envMaxBodyBytes      = "MAX_BODY_BYTES"

	envPostgresHost     = "POSTGRES_HOST"
	envRedisURL         = "REDIS_URL"
	envPostgresPort     = "POSTGRES_PORT"
	envPostgresUser     = "POSTGRES_USER"
	envPostgresPassword = "POSTGRES_PASSWORD"
	envPostgresDBName   = "POSTGRES_DB"
	envPostgresTimeout  = "POSTGRES_TIMEOUT_MS"
	envRedisTimeout     = "REDIS_TIMEOUT_MS"

	envJWTSecret               = "JWT_SECRET"
	envAccessTokenExpiryMins   = "ACCESS_TOKEN_EXPIRY_MINS"
	envRefreshTokenExpiryDays  = "REFRESH_TOKEN_EXPIRY_DAYS"
	envInviteExpiryDays        = "INVITE_EXPIRY_DAYS"
	envPasswordResetExpiryMins = "PASSWORD_RESET_EXPIRY_MINS"

	envEmailFrom    = "EMAIL_FROM"
	envSMTPHost     = "SMTP_HOST"
	envSMTPPort     = "SMTP_PORT"
	envSMTPUsername = "SMTP_USERNAME"
	envSMTPPassword = "SMTP_PASSWORD"

	envRateLimitLimit = "RATE_LIMIT_LIMIT"
	envRateLimitBurst = "RATE_LIMIT_BURST"

	defaultAppURL   = "http://localhost:3000"
	defaultAppEnv   = "development"
	defaultLogLevel = "info"

	defaultHTTPPort            = "8080"
	defaultHTTPReadTimeoutMS   = 10000
	defaultHTTPWriteTimeoutMS  = 30000
	defaultHTTPIdleTimeoutMS   = 120000
	defaultHTTPHeaderTimeoutMS = 5000
	defaultMaxBodyBytes        = 10 * 1024 * 1024

	defaultPostgresHost      = "localhost"
	defaultRedisURL          = "localhost:6379"
	defaultPostgresPort      = "5432"
	defaultPostgresUser      = "apm_user"
	defaultPostgresPassword  = ""
	defaultPostgresDBName    = "apm_db"
	defaultPostgresTimeoutMS = 5000
	defaultRedisTimeoutMS    = 5000

	defaultJWTSecret               = ""
	defaultAccessTokenExpiryMins   = 15
	defaultRefreshTokenExpiryDays  = 7
	defaultInviteExpiryDays        = 10
	defaultPasswordResetExpiryMins = 30

	defaultEmailFrom      = "no-reply@apm-master.local"
	defaultSMTPHost       = ""
	defaultSMTPPort       = "587"
	defaultSMTPUsername   = ""
	defaultSMTPPassword   = ""
	defaultRateLimitLimit = 100.0
	defaultRateLimitBurst = 200
)

// Config holds all configuration values for the application.
type Config struct {
	AppUrl   string
	AppEnv   string
	LogLevel string

	// Server settings
	HTTPPort          string
	HTTPReadTimeout   time.Duration
	HTTPWriteTimeout  time.Duration
	HTTPIdleTimeout   time.Duration
	HTTPHeaderTimeout time.Duration
	MaxBodyBytes      int64

	// Database settings
	PostgresHost     string
	RedisURL         string
	PostgresPort     string
	PostgresUser     string
	PostgresPassword string
	PostgresDBName   string
	PostgresTimeout  time.Duration
	RedisTimeout     time.Duration

	//Redis settings

	// Auth settings
	JWTSecret               string
	AccessTokenExpiryMins   int
	RefreshTokenExpiryDays  int
	InviteExpiryDays        int
	PasswordResetExpiryMins int

	// Email settings
	EmailFrom    string
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string

	RateLimitLimit float64
	RateLimitBurst int
}

func loadEnvFile() (err error) {
	file, err := os.Open(".env")
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open .env: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			if err != nil {
				err = errors.Join(err, fmt.Errorf("close .env: %w", closeErr))
				return
			}
			err = fmt.Errorf("close .env: %w", closeErr)
		}
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
			(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
			val = val[1 : len(val)-1]
		}
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, val); err != nil {
				return fmt.Errorf("set environment variable %s from .env: %w", key, err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan .env: %w", err)
	}

	return nil
}

// Load reads configuration from environment variables.
// If an environment variable is not set, the default value is used.
func Load() (*Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	return &Config{
		AppUrl:   getEnv(envAppURL, defaultAppURL),
		AppEnv:   getEnv(envAppEnv, defaultAppEnv),
		LogLevel: getEnv(envLogLevel, defaultLogLevel),

		HTTPPort:          getEnv(envHTTPPort, defaultHTTPPort),
		HTTPReadTimeout:   getEnvDurationMS(envHTTPReadTimeout, defaultHTTPReadTimeoutMS),
		HTTPWriteTimeout:  getEnvDurationMS(envHTTPWriteTimeout, defaultHTTPWriteTimeoutMS),
		HTTPIdleTimeout:   getEnvDurationMS(envHTTPIdleTimeout, defaultHTTPIdleTimeoutMS),
		HTTPHeaderTimeout: getEnvDurationMS(envHTTPHeaderTimeout, defaultHTTPHeaderTimeoutMS),
		MaxBodyBytes:      int64(getEnvInt(envMaxBodyBytes, defaultMaxBodyBytes)),

		PostgresHost:     getEnv(envPostgresHost, defaultPostgresHost),
		RedisURL:         getEnv(envRedisURL, defaultRedisURL),
		PostgresPort:     getEnv(envPostgresPort, defaultPostgresPort),
		PostgresUser:     getEnv(envPostgresUser, defaultPostgresUser),
		PostgresPassword: getEnv(envPostgresPassword, defaultPostgresPassword),
		PostgresDBName:   getEnv(envPostgresDBName, defaultPostgresDBName),
		PostgresTimeout:  getEnvDurationMS(envPostgresTimeout, defaultPostgresTimeoutMS),
		RedisTimeout:     getEnvDurationMS(envRedisTimeout, defaultRedisTimeoutMS),

		JWTSecret:               getEnv(envJWTSecret, defaultJWTSecret),
		AccessTokenExpiryMins:   getEnvInt(envAccessTokenExpiryMins, defaultAccessTokenExpiryMins),
		RefreshTokenExpiryDays:  getEnvInt(envRefreshTokenExpiryDays, defaultRefreshTokenExpiryDays),
		InviteExpiryDays:        getEnvInt(envInviteExpiryDays, defaultInviteExpiryDays),
		PasswordResetExpiryMins: getEnvInt(envPasswordResetExpiryMins, defaultPasswordResetExpiryMins),

		EmailFrom:    getEnv(envEmailFrom, defaultEmailFrom),
		SMTPHost:     getEnv(envSMTPHost, defaultSMTPHost),
		SMTPPort:     getEnv(envSMTPPort, defaultSMTPPort),
		SMTPUsername: getEnv(envSMTPUsername, defaultSMTPUsername),
		SMTPPassword: getEnv(envSMTPPassword, defaultSMTPPassword),

		RateLimitLimit: getEnvFloat(envRateLimitLimit, defaultRateLimitLimit),
		RateLimitBurst: getEnvInt(envRateLimitBurst, defaultRateLimitBurst),
	}, nil
}

// AccessTokenDuration returns the current access token lifespan.
func (config *Config) AccessTokenDuration() time.Duration {
	return time.Duration(config.AccessTokenExpiryMins) * time.Minute
}

// RefreshTokenDuration returns the current refresh token lifespan.
func (config *Config) RefreshTokenDuration() time.Duration {
	return time.Duration(config.RefreshTokenExpiryDays) * 24 * time.Hour
}

// InviteDuration returns the current invite expiration window.
func (config *Config) InviteDuration() time.Duration {
	return time.Duration(config.InviteExpiryDays) * 24 * time.Hour
}

// PasswordResetDuration returns the current password reset expiration window.
func (config *Config) PasswordResetDuration() time.Duration {
	return time.Duration(config.PasswordResetExpiryMins) * time.Minute
}

// GetPostgresDSN returns DSN for golang-migrate.
func (config *Config) GetPostgresDSN() string {
	return fmt.Sprintf("pgx5://%s:%s@%s:%s/%s?sslmode=disable",
		config.PostgresUser,
		config.PostgresPassword,
		config.PostgresHost,
		config.PostgresPort,
		config.PostgresDBName,
	)
}

// GetPoolDSN returns DSN for pgxpool connection.
func (config *Config) GetPoolDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		config.PostgresUser,
		config.PostgresPassword,
		config.PostgresHost,
		config.PostgresPort,
		config.PostgresDBName,
	)
}

// getEnv reads an environment variable by key.
// Returns the fallback value if the variable is not set.
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

type validationRule struct {
	ok  bool
	err error
}

func (config *Config) Validate() error {

	rules := []validationRule{
		// --- Server settings ---
		{config.AppEnv != "", fmt.Errorf("APP_ENV must be set")},
		{config.LogLevel != "", fmt.Errorf("LOG_LEVEL must be set")},
		{config.HTTPPort != "", fmt.Errorf("HTTP_PORT must be set")},
		{config.HTTPReadTimeout > 0, fmt.Errorf("HTTP_READ_TIMEOUT_MS must be greater than 0")},
		{config.HTTPWriteTimeout > 0, fmt.Errorf("HTTP_WRITE_TIMEOUT_MS must be greater than 0")},
		{config.HTTPIdleTimeout > 0, fmt.Errorf("HTTP_IDLE_TIMEOUT_MS must be greater than 0")},
		{config.HTTPHeaderTimeout > 0, fmt.Errorf("HTTP_HEADER_TIMEOUT_MS must be greater than 0")},
		{config.MaxBodyBytes > 0, fmt.Errorf("MAX_BODY_BYTES must be greater than 0")},

		// --- Database settings ---
		{config.PostgresHost != "", fmt.Errorf("POSTGRES_HOST must be set")},
		{config.RedisURL != "", fmt.Errorf("REDIS_URL must be set")},
		{config.PostgresPort != "", fmt.Errorf("POSTGRES_PORT must be set")},
		{config.PostgresUser != "", fmt.Errorf("POSTGRES_USER must be set")},
		{config.PostgresDBName != "", fmt.Errorf("POSTGRES_DB must be set")},
		{config.PostgresTimeout > 0, fmt.Errorf("POSTGRES_TIMEOUT_MS must be greater than 0")},
		{config.RedisTimeout > 0, fmt.Errorf("REDIS_TIMEOUT_MS must be greater than 0")},
		{config.PostgresPassword != "", fmt.Errorf("POSTGRES_PASSWORD must be set")},

		// --- Auth settings ---
		{config.JWTSecret != "", fmt.Errorf("JWT_SECRET must be set")},
		{len(config.JWTSecret) >= 32, fmt.Errorf("JWT_SECRET must be at least 32 characters, got %d", len(config.JWTSecret))},
		{config.AccessTokenExpiryMins > 0, fmt.Errorf("ACCESS_TOKEN_EXPIRY_MINS must be greater than 0")},
		{config.RefreshTokenExpiryDays > 0, fmt.Errorf("REFRESH_TOKEN_EXPIRY_DAYS must be greater than 0")},
		{config.InviteExpiryDays > 0, fmt.Errorf("INVITE_EXPIRY_DAYS must be greater than 0")},
		{config.PasswordResetExpiryMins > 0, fmt.Errorf("PASSWORD_RESET_EXPIRY_MINS must be greater than 0")},

		// --- Email settings ---
		{config.EmailFrom != "", fmt.Errorf("EMAIL_FROM must be set")},
		{config.SMTPHost != "", fmt.Errorf("SMTP_HOST must be set")},
		{config.SMTPPort != "", fmt.Errorf("SMTP_PORT must be set")},

		// --- Rate Limiting ---
		{config.RateLimitLimit > 0, fmt.Errorf("RATE_LIMIT_LIMIT must be greater than 0")},
		{config.RateLimitBurst > 0, fmt.Errorf("RATE_LIMIT_BURST must be greater than 0")},
	}

	var errs []error
	for _, r := range rules {
		if !r.ok {
			errs = append(errs, r.err)
		}
	}
	return errors.Join(errs...)
}

func getEnvInt(key string, fallback int) int {
	if value, exists := os.LookupEnv(key); exists {
		if index, err := strconv.Atoi(value); err == nil {
			return index
		}
	}
	return fallback
}

func getEnvDurationMS(key string, fallbackMS int) time.Duration {
	return time.Duration(getEnvInt(key, fallbackMS)) * time.Millisecond
}

func getEnvFloat(key string, fallback float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if val, err := strconv.ParseFloat(value, 64); err == nil {
			return val
		}
	}
	return fallback
}

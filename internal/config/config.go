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
	envAppMode  = "APP_MODE"
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

	envKafkaBrokers                = "KAFKA_BROKERS"
	envKafkaClientID               = "KAFKA_CLIENT_ID"
	envKafkaConsumerGroup          = "KAFKA_CONSUMER_GROUP"
	envKafkaTracesTopic            = "KAFKA_TRACES_TOPIC"
	envKafkaLogsTopic              = "KAFKA_LOGS_TOPIC"
	envKafkaMetricsTopic           = "KAFKA_METRICS_TOPIC"
	envKafkaTracesDLQTopic         = "KAFKA_TRACES_DLQ_TOPIC"
	envKafkaLogsDLQTopic           = "KAFKA_LOGS_DLQ_TOPIC"
	envKafkaMetricsDLQTopic        = "KAFKA_METRICS_DLQ_TOPIC"
	envKafkaTopicPartitions        = "KAFKA_TOPIC_PARTITIONS"
	envKafkaTopicReplicationFactor = "KAFKA_TOPIC_REPLICATION_FACTOR"
	envKafkaTopicRetention         = "KAFKA_TOPIC_RETENTION_MS"
	envKafkaCompression            = "KAFKA_COMPRESSION"
	envKafkaProducerBatchSize      = "KAFKA_PRODUCER_BATCH_SIZE"
	envKafkaProducerBatchBytes     = "KAFKA_PRODUCER_BATCH_BYTES"
	envKafkaProducerBatchTimeout   = "KAFKA_PRODUCER_BATCH_TIMEOUT_MS"
	envKafkaProducerWriteTimeout   = "KAFKA_PRODUCER_WRITE_TIMEOUT_MS"
	envKafkaProducerRequiredAcks   = "KAFKA_PRODUCER_REQUIRED_ACKS"
	envKafkaProducerMaxAttempts    = "KAFKA_PRODUCER_MAX_ATTEMPTS"
	envKafkaConsumerWorkers        = "KAFKA_CONSUMER_WORKERS"
	envKafkaConsumerBatchRecords   = "KAFKA_CONSUMER_BATCH_RECORDS"
	envKafkaConsumerBatchBytes     = "KAFKA_CONSUMER_BATCH_BYTES"
	envKafkaConsumerFlushInterval  = "KAFKA_CONSUMER_FLUSH_INTERVAL_MS"
	envKafkaConsumerReadMaxBytes   = "KAFKA_CONSUMER_READ_MAX_BYTES"
	envKafkaConsumerMaxAttempts    = "KAFKA_CONSUMER_MAX_ATTEMPTS"
	envKafkaConsumerRetryInitial   = "KAFKA_CONSUMER_RETRY_INITIAL_MS"
	envKafkaConsumerRetryMax       = "KAFKA_CONSUMER_RETRY_MAX_MS"
	envAlertEvaluationInterval     = "ALERT_EVALUATION_INTERVAL_MS"
	envRateLimitLimit              = "RATE_LIMIT_LIMIT"
	envRateLimitBurst              = "RATE_LIMIT_BURST"
	envRateLimitWindow             = "RATE_LIMIT_WINDOW_MS"
	envTelemetryRetentionDays      = "TELEMETRY_RETENTION_DAYS"
	envNotificationRetentionDays   = "NOTIFICATION_RETENTION_DAYS"
	envRefreshTokenRetentionDays   = "REFRESH_TOKEN_RETENTION_DAYS"

	envKafkaAIInvestigationTopic         = "KAFKA_AI_INVESTIGATION_TOPIC"
	envKafkaAIRetryTopic                 = "KAFKA_AI_RETRY_TOPIC"
	envKafkaAIDLQTopic                   = "KAFKA_AI_DLQ_TOPIC"
	envAIServiceToken                    = "AI_SERVICE_TOKEN"
	envInvestigationOutboxPollIntervalMS = "INVESTIGATION_OUTBOX_POLL_INTERVAL_MS"
	envInvestigationOutboxBatchSize      = "INVESTIGATION_OUTBOX_BATCH_SIZE"

	defaultAppURL   = "http://localhost:3000"
	defaultAppEnv   = "development"
	defaultAppMode  = "all"
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

	defaultEmailFrom    = "no-reply@apm-master.local"
	defaultSMTPHost     = ""
	defaultSMTPPort     = "587"
	defaultSMTPUsername = ""
	defaultSMTPPassword = ""

	defaultKafkaBrokers                 = "localhost:9092"
	defaultKafkaClientID                = "apm-master-ingest"
	defaultKafkaConsumerGroup           = "apm-master-ingest-writers"
	defaultKafkaTracesTopic             = "apm.telemetry.trace.v1"
	defaultKafkaLogsTopic               = "apm.telemetry.log.v1"
	defaultKafkaMetricsTopic            = "apm.telemetry.metric.v1"
	defaultKafkaTracesDLQTopic          = "apm.telemetry.trace.dlq.v1"
	defaultKafkaLogsDLQTopic            = "apm.telemetry.log.dlq.v1"
	defaultKafkaMetricsDLQTopic         = "apm.telemetry.metric.dlq.v1"
	defaultKafkaTopicPartitions         = 12
	defaultKafkaTopicReplicationFactor  = 1
	defaultKafkaTopicRetentionMS        = 604800000
	defaultKafkaCompression             = "lz4"
	defaultKafkaProducerBatchSize       = 1000
	defaultKafkaProducerBatchBytes      = 1048576
	defaultKafkaProducerBatchTimeoutMS  = 10
	defaultKafkaProducerWriteTimeoutMS  = 10000
	defaultKafkaProducerRequiredAcks    = -1
	defaultKafkaProducerMaxAttempts     = 10
	defaultKafkaConsumerWorkers         = 3
	defaultKafkaConsumerBatchRecords    = 1000
	defaultKafkaConsumerBatchBytes      = 4194304
	defaultKafkaConsumerFlushIntervalMS = 1000
	defaultKafkaConsumerReadMaxBytes    = 10485760
	defaultKafkaConsumerMaxAttempts     = 5
	defaultKafkaConsumerRetryInitialMS  = 250
	defaultKafkaConsumerRetryMaxMS      = 5000
	defaultAlertEvaluationIntervalMS    = 60000
	defaultRateLimitLimit               = 100.0
	defaultRateLimitBurst               = 200
	defaultRateLimitWindowMS            = 1000
	defaultTelemetryRetentionDays       = 30
	defaultNotificationRetentionDays    = 90
	defaultRefreshTokenRetentionDays    = 30

	defaultKafkaAIInvestigationTopic         = "ai.investigation.jobs"
	defaultKafkaAIRetryTopic                 = "ai.investigation.retry"
	defaultKafkaAIDLQTopic                   = "ai.investigation.dlq"
	defaultAIServiceToken                    = "apm-ai-secret-token-change-in-production"
	defaultInvestigationOutboxPollIntervalMS = 2000
	defaultInvestigationOutboxBatchSize      = 50
)

// Config holds all configuration values for the application.
type Config struct {
	AppUrl   string
	AppEnv   string
	AppMode  string
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

	KafkaBrokers                string
	KafkaClientID               string
	KafkaConsumerGroup          string
	KafkaTracesTopic            string
	KafkaLogsTopic              string
	KafkaMetricsTopic           string
	KafkaTracesDLQTopic         string
	KafkaLogsDLQTopic           string
	KafkaMetricsDLQTopic        string
	KafkaTopicPartitions        int
	KafkaTopicReplicationFactor int
	KafkaTopicRetention         time.Duration
	KafkaCompression            string
	KafkaProducerBatchSize      int
	KafkaProducerBatchBytes     int
	KafkaProducerBatchTimeout   time.Duration
	KafkaProducerWriteTimeout   time.Duration
	KafkaProducerRequiredAcks   int
	KafkaProducerMaxAttempts    int
	KafkaConsumerWorkers        int
	KafkaConsumerBatchRecords   int
	KafkaConsumerBatchBytes     int
	KafkaConsumerFlushInterval  time.Duration
	KafkaConsumerReadMaxBytes   int
	KafkaConsumerMaxAttempts    int
	KafkaConsumerRetryInitial   time.Duration
	KafkaConsumerRetryMax       time.Duration
	AlertEvaluationInterval     time.Duration
	RateLimitLimit              float64
	RateLimitBurst              int
	RateLimitWindow             time.Duration

	// Retention settings — separate windows per data class
	TelemetryRetentionDays    int // spans, metrics, log_records
	NotificationRetentionDays int // notification_jobs, notification_events
	RefreshTokenRetentionDays int // revoked/expired refresh_tokens

	// AI Investigation settings
	KafkaAIInvestigationTopic       string
	KafkaAIRetryTopic               string
	KafkaAIDLQTopic                 string
	AIServiceToken                  string
	InvestigationOutboxPollInterval time.Duration
	InvestigationOutboxBatchSize    int
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
		AppMode:  getEnv(envAppMode, defaultAppMode),
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

		KafkaBrokers:                getEnv(envKafkaBrokers, defaultKafkaBrokers),
		KafkaClientID:               getEnv(envKafkaClientID, defaultKafkaClientID),
		KafkaConsumerGroup:          getEnv(envKafkaConsumerGroup, defaultKafkaConsumerGroup),
		KafkaTracesTopic:            getEnv(envKafkaTracesTopic, defaultKafkaTracesTopic),
		KafkaLogsTopic:              getEnv(envKafkaLogsTopic, defaultKafkaLogsTopic),
		KafkaMetricsTopic:           getEnv(envKafkaMetricsTopic, defaultKafkaMetricsTopic),
		KafkaTracesDLQTopic:         getEnv(envKafkaTracesDLQTopic, defaultKafkaTracesDLQTopic),
		KafkaLogsDLQTopic:           getEnv(envKafkaLogsDLQTopic, defaultKafkaLogsDLQTopic),
		KafkaMetricsDLQTopic:        getEnv(envKafkaMetricsDLQTopic, defaultKafkaMetricsDLQTopic),
		KafkaTopicPartitions:        getEnvInt(envKafkaTopicPartitions, defaultKafkaTopicPartitions),
		KafkaTopicReplicationFactor: getEnvInt(envKafkaTopicReplicationFactor, defaultKafkaTopicReplicationFactor),
		KafkaTopicRetention:         getEnvDurationMS(envKafkaTopicRetention, defaultKafkaTopicRetentionMS),
		KafkaCompression:            getEnv(envKafkaCompression, defaultKafkaCompression),
		KafkaProducerBatchSize:      getEnvInt(envKafkaProducerBatchSize, defaultKafkaProducerBatchSize),
		KafkaProducerBatchBytes:     getEnvInt(envKafkaProducerBatchBytes, defaultKafkaProducerBatchBytes),
		KafkaProducerBatchTimeout:   getEnvDurationMS(envKafkaProducerBatchTimeout, defaultKafkaProducerBatchTimeoutMS),
		KafkaProducerWriteTimeout:   getEnvDurationMS(envKafkaProducerWriteTimeout, defaultKafkaProducerWriteTimeoutMS),
		KafkaProducerRequiredAcks:   getEnvInt(envKafkaProducerRequiredAcks, defaultKafkaProducerRequiredAcks),
		KafkaProducerMaxAttempts:    getEnvInt(envKafkaProducerMaxAttempts, defaultKafkaProducerMaxAttempts),
		KafkaConsumerWorkers:        getEnvInt(envKafkaConsumerWorkers, defaultKafkaConsumerWorkers),
		KafkaConsumerBatchRecords:   getEnvInt(envKafkaConsumerBatchRecords, defaultKafkaConsumerBatchRecords),
		KafkaConsumerBatchBytes:     getEnvInt(envKafkaConsumerBatchBytes, defaultKafkaConsumerBatchBytes),
		KafkaConsumerFlushInterval:  getEnvDurationMS(envKafkaConsumerFlushInterval, defaultKafkaConsumerFlushIntervalMS),
		KafkaConsumerReadMaxBytes:   getEnvInt(envKafkaConsumerReadMaxBytes, defaultKafkaConsumerReadMaxBytes),
		KafkaConsumerMaxAttempts:    getEnvInt(envKafkaConsumerMaxAttempts, defaultKafkaConsumerMaxAttempts),
		KafkaConsumerRetryInitial:   getEnvDurationMS(envKafkaConsumerRetryInitial, defaultKafkaConsumerRetryInitialMS),
		KafkaConsumerRetryMax:       getEnvDurationMS(envKafkaConsumerRetryMax, defaultKafkaConsumerRetryMaxMS),
		AlertEvaluationInterval:     getEnvDurationMS(envAlertEvaluationInterval, defaultAlertEvaluationIntervalMS),
		RateLimitLimit:              getEnvFloat(envRateLimitLimit, defaultRateLimitLimit),
		RateLimitBurst:              getEnvInt(envRateLimitBurst, defaultRateLimitBurst),
		RateLimitWindow:             getEnvDurationMS(envRateLimitWindow, defaultRateLimitWindowMS),
		TelemetryRetentionDays:      getEnvInt(envTelemetryRetentionDays, defaultTelemetryRetentionDays),
		NotificationRetentionDays:   getEnvInt(envNotificationRetentionDays, defaultNotificationRetentionDays),
		RefreshTokenRetentionDays:   getEnvInt(envRefreshTokenRetentionDays, defaultRefreshTokenRetentionDays),

		KafkaAIInvestigationTopic:       getEnv(envKafkaAIInvestigationTopic, defaultKafkaAIInvestigationTopic),
		KafkaAIRetryTopic:               getEnv(envKafkaAIRetryTopic, defaultKafkaAIRetryTopic),
		KafkaAIDLQTopic:                 getEnv(envKafkaAIDLQTopic, defaultKafkaAIDLQTopic),
		AIServiceToken:                  getEnv(envAIServiceToken, defaultAIServiceToken),
		InvestigationOutboxPollInterval: getEnvDurationMS(envInvestigationOutboxPollIntervalMS, defaultInvestigationOutboxPollIntervalMS),
		InvestigationOutboxBatchSize:    getEnvInt(envInvestigationOutboxBatchSize, defaultInvestigationOutboxBatchSize),
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
	validAcks := map[int]bool{-1: true, 0: true, 1: true}
	validCompression := map[string]bool{
		"lz4": true, "gzip": true, "snappy": true, "zstd": true, "none": true,
	}

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

		// --- Kafka settings ---
		{config.KafkaBrokers != "", fmt.Errorf("KAFKA_BROKERS must be set")},
		{config.KafkaClientID != "", fmt.Errorf("KAFKA_CLIENT_ID must be set")},
		{config.KafkaConsumerGroup != "", fmt.Errorf("KAFKA_CONSUMER_GROUP must be set")},
		{config.KafkaTracesTopic != "" && config.KafkaLogsTopic != "" && config.KafkaMetricsTopic != "",
			fmt.Errorf("KAFKA telemetry topics must be set")},
		{config.KafkaTracesDLQTopic != "" && config.KafkaLogsDLQTopic != "" && config.KafkaMetricsDLQTopic != "",
			fmt.Errorf("KAFKA DLQ topics must be set")},
		{config.KafkaTopicPartitions > 0, fmt.Errorf("KAFKA_TOPIC_PARTITIONS must be greater than 0")},
		{config.KafkaTopicReplicationFactor > 0, fmt.Errorf("KAFKA_TOPIC_REPLICATION_FACTOR must be greater than 0")},
		{config.KafkaTopicRetention > 0, fmt.Errorf("KAFKA_TOPIC_RETENTION_MS must be greater than 0")},
		{config.KafkaProducerBatchSize > 0, fmt.Errorf("KAFKA_PRODUCER_BATCH_SIZE must be greater than 0")},
		{config.KafkaProducerBatchBytes > 0, fmt.Errorf("KAFKA_PRODUCER_BATCH_BYTES must be greater than 0")},
		{config.KafkaProducerBatchTimeout > 0, fmt.Errorf("KAFKA_PRODUCER_BATCH_TIMEOUT_MS must be greater than 0")},
		{config.KafkaProducerWriteTimeout > 0, fmt.Errorf("KAFKA_PRODUCER_WRITE_TIMEOUT_MS must be greater than 0")},
		{config.KafkaProducerMaxAttempts > 0, fmt.Errorf("KAFKA_PRODUCER_MAX_ATTEMPTS must be greater than 0")},
		{validAcks[config.KafkaProducerRequiredAcks],
			fmt.Errorf("KAFKA_PRODUCER_REQUIRED_ACKS must be -1, 0, or 1, got %d", config.KafkaProducerRequiredAcks)},
		{validCompression[config.KafkaCompression],
			fmt.Errorf("KAFKA_COMPRESSION must be one of lz4/gzip/snappy/zstd/none, got %q", config.KafkaCompression)},
		{config.KafkaConsumerWorkers > 0, fmt.Errorf("KAFKA_CONSUMER_WORKERS must be greater than 0")},
		{config.KafkaConsumerBatchRecords > 0, fmt.Errorf("KAFKA_CONSUMER_BATCH_RECORDS must be greater than 0")},
		{config.KafkaConsumerBatchBytes > 0, fmt.Errorf("KAFKA_CONSUMER_BATCH_BYTES must be greater than 0")},
		{config.KafkaConsumerFlushInterval > 0, fmt.Errorf("KAFKA_CONSUMER_FLUSH_INTERVAL_MS must be greater than 0")},
		{config.KafkaConsumerReadMaxBytes > 0, fmt.Errorf("KAFKA_CONSUMER_READ_MAX_BYTES must be greater than 0")},
		{config.KafkaConsumerMaxAttempts > 0, fmt.Errorf("KAFKA_CONSUMER_MAX_ATTEMPTS must be greater than 0")},
		{config.KafkaConsumerRetryInitial > 0, fmt.Errorf("KAFKA_CONSUMER_RETRY_INITIAL_MS must be greater than 0")},
		{config.KafkaConsumerRetryMax > 0, fmt.Errorf("KAFKA_CONSUMER_RETRY_MAX_MS must be greater than 0")},

		// --- Alerting ---
		{config.AlertEvaluationInterval > 0, fmt.Errorf("ALERT_EVALUATION_INTERVAL_MS must be greater than 0")},

		// --- Rate Limiting ---
		{config.RateLimitLimit > 0, fmt.Errorf("RATE_LIMIT_LIMIT must be greater than 0")},
		{config.RateLimitBurst > 0, fmt.Errorf("RATE_LIMIT_BURST must be greater than 0")},

		// --- AI Investigation ---
		{config.KafkaAIInvestigationTopic != "", fmt.Errorf("KAFKA_AI_INVESTIGATION_TOPIC must be set")},
		{config.KafkaAIRetryTopic != "", fmt.Errorf("KAFKA_AI_RETRY_TOPIC must be set")},
		{config.KafkaAIDLQTopic != "", fmt.Errorf("KAFKA_AI_DLQ_TOPIC must be set")},
		{config.AIServiceToken != "", fmt.Errorf("AI_SERVICE_TOKEN must be set")},
		{config.InvestigationOutboxPollInterval > 0, fmt.Errorf("INVESTIGATION_OUTBOX_POLL_INTERVAL_MS must be greater than 0")},
		{config.InvestigationOutboxBatchSize > 0, fmt.Errorf("INVESTIGATION_OUTBOX_BATCH_SIZE must be greater than 0")},
	}

	var errs []error
	for _, r := range rules {
		if !r.ok {
			errs = append(errs, r.err)
		}
	}
	return errors.Join(errs...)
}

func (config *Config) ValidateScheduler() error {
	rules := []validationRule{
		// --- Server settings ---
		{config.AppEnv != "", fmt.Errorf("APP_ENV must be set")},
		{config.LogLevel != "", fmt.Errorf("LOG_LEVEL must be set")},
		{config.AppMode == "scheduler" || config.AppMode == "all", fmt.Errorf("APP_MODE must be scheduler or all")},

		// --- Database settings ---
		{config.PostgresHost != "", fmt.Errorf("POSTGRES_HOST must be set")},
		{config.RedisURL != "", fmt.Errorf("REDIS_URL must be set")},
		{config.PostgresPort != "", fmt.Errorf("POSTGRES_PORT must be set")},
		{config.PostgresUser != "", fmt.Errorf("POSTGRES_USER must be set")},
		{config.PostgresDBName != "", fmt.Errorf("POSTGRES_DB must be set")},
		{config.PostgresTimeout > 0, fmt.Errorf("POSTGRES_TIMEOUT_MS must be greater than 0")},
		{config.RedisTimeout > 0, fmt.Errorf("REDIS_TIMEOUT_MS must be greater than 0")},
		{config.PostgresPassword != "", fmt.Errorf("POSTGRES_PASSWORD must be set")},

		// --- Email settings ---
		{config.EmailFrom != "", fmt.Errorf("EMAIL_FROM must be set")},
		{config.SMTPHost != "", fmt.Errorf("SMTP_HOST must be set")},
		{config.SMTPPort != "", fmt.Errorf("SMTP_PORT must be set")},

		// --- Alerting ---
		{config.AlertEvaluationInterval > 0, fmt.Errorf("ALERT_EVALUATION_INTERVAL_MS must be greater than 0")},
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

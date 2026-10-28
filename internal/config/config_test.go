package config

import (
	"testing"
	"time"
)

func TestLoadUsesConfiguredDurations(testRunner *testing.T) {
	testRunner.Setenv("APP_ENV", "test")
	testRunner.Setenv("LOG_LEVEL", "debug")
	testRunner.Setenv("HTTP_READ_TIMEOUT_MS", "11000")
	testRunner.Setenv("HTTP_WRITE_TIMEOUT_MS", "31000")
	testRunner.Setenv("HTTP_IDLE_TIMEOUT_MS", "121000")
	testRunner.Setenv("HTTP_HEADER_TIMEOUT_MS", "6000")
	testRunner.Setenv("MAX_BODY_BYTES", "1048576")
	testRunner.Setenv("POSTGRES_TIMEOUT_MS", "7000")
	testRunner.Setenv("POSTGRES_MAX_CONNS", "25")
	testRunner.Setenv("POSTGRES_MIN_CONNS", "3")
	testRunner.Setenv("POSTGRES_MAX_CONN_IDLE_TIME_MS", "300000")
	testRunner.Setenv("POSTGRES_MAX_CONN_LIFETIME_MS", "1800000")
	testRunner.Setenv("REDIS_TIMEOUT_MS", "8000")
	testRunner.Setenv("ACCESS_TOKEN_EXPIRY_MINS", "25")
	testRunner.Setenv("REFRESH_TOKEN_EXPIRY_DAYS", "14")
	testRunner.Setenv("INVITE_EXPIRY_DAYS", "3")
	testRunner.Setenv("KAFKA_PRODUCER_WRITE_TIMEOUT_MS", "12000")
	testRunner.Setenv("KAFKA_CONSUMER_FLUSH_INTERVAL_MS", "2500")
	testRunner.Setenv("POSTGRES_PASSWORD", "super-secret-password")
	testRunner.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	testRunner.Setenv("SMTP_HOST", "localhost")
	testRunner.Setenv("SMTP_PORT", "587")
	testRunner.Setenv("RATE_LIMIT_LIMIT", "150.0")
	testRunner.Setenv("RATE_LIMIT_BURST", "300")

	cfg, err := Load()
	if err != nil {
		testRunner.Fatalf("Load() error = %v", err)
	}

	if got := cfg.AppEnv; got != "test" {
		testRunner.Fatalf("AppEnv = %q, want test", got)
	}

	if got := cfg.LogLevel; got != "debug" {
		testRunner.Fatalf("LogLevel = %q, want debug", got)
	}

	if got := cfg.HTTPReadTimeout; got != 11*time.Second {
		testRunner.Fatalf("HTTPReadTimeout = %v, want %v", got, 11*time.Second)
	}

	if got := cfg.HTTPWriteTimeout; got != 31*time.Second {
		testRunner.Fatalf("HTTPWriteTimeout = %v, want %v", got, 31*time.Second)
	}

	if got := cfg.HTTPIdleTimeout; got != 121*time.Second {
		testRunner.Fatalf("HTTPIdleTimeout = %v, want %v", got, 121*time.Second)
	}

	if got := cfg.HTTPHeaderTimeout; got != 6*time.Second {
		testRunner.Fatalf("HTTPHeaderTimeout = %v, want %v", got, 6*time.Second)
	}

	if got := cfg.MaxBodyBytes; got != 1048576 {
		testRunner.Fatalf("MaxBodyBytes = %d, want %d", got, 1048576)
	}

	if got := cfg.PostgresTimeout; got != 7*time.Second {
		testRunner.Fatalf("PostgresTimeout = %v, want %v", got, 7*time.Second)
	}

	if got := cfg.RedisTimeout; got != 8*time.Second {
		testRunner.Fatalf("RedisTimeout = %v, want %v", got, 8*time.Second)
	}

	if got := cfg.PostgresMaxConns; got != 25 {
		testRunner.Fatalf("PostgresMaxConns = %d, want 25", got)
	}

	if got := cfg.PostgresMinConns; got != 3 {
		testRunner.Fatalf("PostgresMinConns = %d, want 3", got)
	}

	if got := cfg.PostgresMaxConnIdleTime; got != 5*time.Minute {
		testRunner.Fatalf("PostgresMaxConnIdleTime = %v, want 5m", got)
	}

	if got := cfg.PostgresMaxConnLifetime; got != 30*time.Minute {
		testRunner.Fatalf("PostgresMaxConnLifetime = %v, want 30m", got)
	}

	poolCfg := cfg.GetPoolConfig()
	if poolCfg.MaxConns != 25 || poolCfg.MinConns != 3 {
		testRunner.Fatalf("GetPoolConfig mismatch: %+v", poolCfg)
	}

	if got := cfg.AccessTokenDuration(); got != 25*time.Minute {
		testRunner.Fatalf("AccessTokenDuration() = %v, want %v", got, 25*time.Minute)
	}

	if got := cfg.RefreshTokenDuration(); got != 14*24*time.Hour {
		testRunner.Fatalf("RefreshTokenDuration() = %v, want %v", got, 14*24*time.Hour)
	}

	if got := cfg.InviteDuration(); got != 3*24*time.Hour {
		testRunner.Fatalf("InviteDuration() = %v, want %v", got, 3*24*time.Hour)
	}

	if got := cfg.PasswordResetDuration(); got != 30*time.Minute {
		testRunner.Fatalf("PasswordResetDuration() = %v, want %v", got, 30*time.Minute)
	}

	if got := cfg.KafkaConsumerFlushInterval; got != 2500*time.Millisecond {
		testRunner.Fatalf("KafkaConsumerFlushInterval = %v, want %v", got, 2500*time.Millisecond)
	}

	if got := cfg.KafkaProducerWriteTimeout; got != 12*time.Second {
		testRunner.Fatalf("KafkaProducerWriteTimeout = %v, want %v", got, 12*time.Second)
	}

	if got := cfg.RateLimitLimit; got != 150.0 {
		testRunner.Fatalf("RateLimitLimit = %v, want %v", got, 150.0)
	}

	if got := cfg.RateLimitBurst; got != 300 {
		testRunner.Fatalf("RateLimitBurst = %d, want %d", got, 300)
	}
}

func TestValidateRejectsInvalidConfiguration(testRunner *testing.T) {
	testRunner.Run("missing password", func(testRunner *testing.T) {
		cfg := validConfig()
		cfg.PostgresPassword = ""
		if err := cfg.Validate(); err == nil {
			testRunner.Fatal("expected error for missing Postgres password")
		}
	})

	testRunner.Run("weak jwt secret", func(testRunner *testing.T) {
		cfg := validConfig()
		cfg.JWTSecret = "short"
		if err := cfg.Validate(); err == nil {
			testRunner.Fatal("expected error for weak JWT secret")
		}
	})

	testRunner.Run("invalid kafka consumer", func(testRunner *testing.T) {
		cfg := validConfig()
		cfg.KafkaConsumerBatchRecords = 0
		if err := cfg.Validate(); err == nil {
			testRunner.Fatal("expected error for invalid kafka batch size")
		}
	})

	testRunner.Run("invalid postgres pool max conns", func(testRunner *testing.T) {
		cfg := validConfig()
		cfg.PostgresMaxConns = 0
		if err := cfg.Validate(); err == nil {
			testRunner.Fatal("expected error for PostgresMaxConns = 0")
		}
	})

	testRunner.Run("invalid postgres pool min greater than max", func(testRunner *testing.T) {
		cfg := validConfig()
		cfg.PostgresMaxConns = 5
		cfg.PostgresMinConns = 10
		if err := cfg.Validate(); err == nil {
			testRunner.Fatal("expected error for PostgresMinConns > PostgresMaxConns")
		}
	})

	testRunner.Run("valid config", func(testRunner *testing.T) {
		cfg := validConfig()
		if err := cfg.Validate(); err != nil {
			testRunner.Fatalf("Validate() error = %v", err)
		}
	})
}

func validConfig() *Config {
	return &Config{
		AppUrl:                      "http://localhost:3000",
		AppEnv:                      "test",
		LogLevel:                    "info",
		HTTPPort:                    "8080",
		HTTPReadTimeout:             10 * time.Second,
		HTTPWriteTimeout:            30 * time.Second,
		HTTPIdleTimeout:             120 * time.Second,
		HTTPHeaderTimeout:           5 * time.Second,
		MaxBodyBytes:                10 * 1024 * 1024,
		PostgresHost:                "localhost",
		RedisURL:                    "localhost:6379",
		PostgresPort:                "5432",
		PostgresUser:                "apm_user",
		PostgresPassword:            "super-secret-password",
		PostgresDBName:              "apm_db",
		PostgresTimeout:             5 * time.Second,
		PostgresMaxConns:            50,
		PostgresMinConns:            5,
		PostgresMaxConnIdleTime:     15 * time.Minute,
		PostgresMaxConnLifetime:     1 * time.Hour,
		RedisTimeout:                5 * time.Second,
		JWTSecret:                   "0123456789abcdef0123456789abcdef",
		AccessTokenExpiryMins:       15,
		RefreshTokenExpiryDays:      7,
		InviteExpiryDays:            10,
		PasswordResetExpiryMins:     30,
		EmailFrom:                   "no-reply@example.com",
		SMTPHost:                    "localhost",
		SMTPPort:                    "587",
		KafkaBrokers:                "localhost:9092",
		KafkaClientID:               "apm-master-ingest",
		KafkaConsumerGroup:          "apm-master-ingest-writers",
		KafkaTracesTopic:            "apm.telemetry.trace.v1",
		KafkaLogsTopic:              "apm.telemetry.log.v1",
		KafkaMetricsTopic:           "apm.telemetry.metric.v1",
		KafkaTracesDLQTopic:         "apm.telemetry.trace.dlq.v1",
		KafkaLogsDLQTopic:           "apm.telemetry.log.dlq.v1",
		KafkaMetricsDLQTopic:        "apm.telemetry.metric.dlq.v1",
		KafkaTopicPartitions:        12,
		KafkaTopicReplicationFactor: 1,
		KafkaTopicRetention:         7 * 24 * time.Hour,
		KafkaCompression:            "lz4",
		KafkaProducerBatchSize:      1000,
		KafkaProducerBatchBytes:     1048576,
		KafkaProducerBatchTimeout:   10 * time.Millisecond,
		KafkaProducerWriteTimeout:   10 * time.Second,
		KafkaProducerRequiredAcks:   -1,
		KafkaProducerMaxAttempts:    10,
		KafkaConsumerWorkers:        3,
		KafkaConsumerBatchRecords:   1000,
		KafkaConsumerBatchBytes:     4194304,
		KafkaConsumerFlushInterval:  time.Second,
		KafkaConsumerReadMaxBytes:   10485760,
		KafkaConsumerMaxAttempts:    5,
		KafkaConsumerRetryInitial:   250 * time.Millisecond,
		KafkaConsumerRetryMax:       5 * time.Second,
		AlertEvaluationInterval:         60 * time.Second,
		RateLimitLimit:                  100.0,
		RateLimitBurst:                  200,
		KafkaAIInvestigationTopic:       "ai.investigation.jobs",
		KafkaAIRetryTopic:               "ai.investigation.retry",
		KafkaAIDLQTopic:                 "ai.investigation.dlq",
		AIServiceToken:                  "valid-ai-service-token",
		InvestigationOutboxPollInterval: 2 * time.Second,
		InvestigationOutboxBatchSize:    50,
	}
}

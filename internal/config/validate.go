package config

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

var (
	validAcks        = []int{-1, 0, 1}
	validCompression = []string{"lz4", "gzip", "snappy", "zstd", "none"}
	validModes       = []AppMode{ModeAll, ModeAPI, ModeScheduler}
)

// minJWTSecretLength is the shortest accepted HMAC signing secret.
const minJWTSecretLength = 32

// check collects validation failures so all of them are reported at once.
type check []error

func (c *check) require(ok bool, format string, args ...any) {
	if !ok {
		*c = append(*c, fmt.Errorf(format, args...))
	}
}

func (c *check) positive(name string, v int64) { c.require(v > 0, "%s must be greater than 0", name) }

func (c *check) set(name, v string) { c.require(strings.TrimSpace(v) != "", "%s must be set", name) }

// Validate checks everything the API process needs.
func (c *Config) Validate() error {
	var errs check
	c.App.validate(&errs)
	c.HTTP.validate(&errs)
	c.Postgres.validate(&errs)
	c.Redis.validate(&errs)
	c.Auth.validate(&errs)
	c.SMTP.validate(&errs)
	c.Kafka.validate(&errs)
	c.Alerting.validate(&errs)
	c.AI.validate(&errs, c.App.IsDevelopment())
	return errors.Join(errs...)
}

// ValidateScheduler checks what the standalone scheduler needs.
func (c *Config) ValidateScheduler() error {
	var errs check
	c.App.validate(&errs)
	errs.require(c.App.Mode.RunsScheduler(), "APP_MODE must be scheduler or all, got %q", c.App.Mode)
	c.Postgres.validate(&errs)
	c.SMTP.validate(&errs)
	c.Alerting.validate(&errs)
	return errors.Join(errs...)
}

func (c AppConfig) validate(errs *check) {
	errs.set("APP_ENV", c.Env)
	errs.set("LOG_LEVEL", c.LogLevel)
	errs.require(slices.Contains(validModes, c.Mode), "APP_MODE must be one of all, api, scheduler, got %q", c.Mode)
}

func (c HTTPConfig) validate(errs *check) {
	errs.set("HTTP_PORT", c.Port)
	errs.positive("HTTP_READ_TIMEOUT_MS", int64(c.ReadTimeout))
	errs.positive("HTTP_WRITE_TIMEOUT_MS", int64(c.WriteTimeout))
	errs.positive("HTTP_IDLE_TIMEOUT_MS", int64(c.IdleTimeout))
	errs.positive("HTTP_HEADER_TIMEOUT_MS", int64(c.HeaderTimeout))
	errs.positive("MAX_BODY_BYTES", c.MaxBodyBytes)
	errs.positive("RATE_LIMIT_BURST", int64(c.RateLimitBurst))
	errs.positive("RATE_LIMIT_WINDOW_MS", int64(c.RateLimitWindow))
}

func (c PostgresConfig) validate(errs *check) {
	errs.set("POSTGRES_HOST", c.Host)
	errs.set("POSTGRES_PORT", c.Port)
	errs.set("POSTGRES_USER", c.User)
	errs.set("POSTGRES_PASSWORD", c.Password)
	errs.set("POSTGRES_DB", c.DBName)
	errs.positive("POSTGRES_TIMEOUT_MS", int64(c.Timeout))
	errs.positive("POSTGRES_MAX_CONNS", int64(c.MaxConns))
	errs.require(c.MinConns >= 0, "POSTGRES_MIN_CONNS must be greater than or equal to 0")
	errs.require(c.MaxConns >= c.MinConns, "POSTGRES_MAX_CONNS must be greater than or equal to POSTGRES_MIN_CONNS")
	errs.positive("POSTGRES_MAX_CONN_IDLE_TIME_MS", int64(c.MaxConnIdleTime))
	errs.positive("POSTGRES_MAX_CONN_LIFETIME_MS", int64(c.MaxConnLifetime))
}

func (c RedisConfig) validate(errs *check) {
	errs.set("REDIS_URL", c.URL)
	errs.positive("REDIS_TIMEOUT_MS", int64(c.Timeout))
}

func (c AuthConfig) validate(errs *check) {
	errs.require(len(c.JWTSecret) >= minJWTSecretLength, "JWT_SECRET must be at least %d characters, got %d", minJWTSecretLength, len(c.JWTSecret))
	errs.positive("ACCESS_TOKEN_EXPIRY_MINS", int64(c.AccessTokenExpiryMins))
	errs.positive("REFRESH_TOKEN_EXPIRY_DAYS", int64(c.RefreshTokenExpiryDays))
	errs.require(c.RefreshSessionMaxDays >= c.RefreshTokenExpiryDays,
		"REFRESH_SESSION_MAX_DAYS (%d) must be at least REFRESH_TOKEN_EXPIRY_DAYS (%d)", c.RefreshSessionMaxDays, c.RefreshTokenExpiryDays)
	errs.positive("INVITE_EXPIRY_DAYS", int64(c.InviteExpiryDays))
	errs.positive("PASSWORD_RESET_EXPIRY_MINS", int64(c.PasswordResetExpiryMins))
	errs.require(c.PasswordResetCooldownSecs >= 0, "PASSWORD_RESET_COOLDOWN_SECS must not be negative")
}

func (c SMTPConfig) validate(errs *check) {
	errs.set("EMAIL_FROM", c.From)
	errs.set("SMTP_HOST", c.Host)
	errs.set("SMTP_PORT", c.Port)
}

func (c KafkaConfig) validate(errs *check) {
	errs.require(len(c.Brokers) > 0, "KAFKA_BROKERS must be set")
	errs.set("KAFKA_CLIENT_ID", c.ClientID)
	errs.set("KAFKA_CONSUMER_GROUP", c.ConsumerGroup)
	for name, v := range map[string]string{
		"KAFKA_TRACES_TOPIC": c.TracesTopic, "KAFKA_LOGS_TOPIC": c.LogsTopic, "KAFKA_METRICS_TOPIC": c.MetricsTopic,
		"KAFKA_TRACES_DLQ_TOPIC": c.TracesDLQTopic, "KAFKA_LOGS_DLQ_TOPIC": c.LogsDLQTopic, "KAFKA_METRICS_DLQ_TOPIC": c.MetricsDLQTopic,
	} {
		errs.set(name, v)
	}
	errs.positive("KAFKA_TOPIC_PARTITIONS", int64(c.TopicPartitions))
	errs.positive("KAFKA_TOPIC_REPLICATION_FACTOR", int64(c.TopicReplicationFactor))
	errs.positive("KAFKA_TOPIC_RETENTION_MS", int64(c.TopicRetention))
	errs.require(slices.Contains(validCompression, c.Compression), "KAFKA_COMPRESSION must be one of %s, got %q", strings.Join(validCompression, "/"), c.Compression)
	errs.positive("KAFKA_PRODUCER_BATCH_SIZE", int64(c.ProducerBatchSize))
	errs.positive("KAFKA_PRODUCER_BATCH_BYTES", int64(c.ProducerBatchBytes))
	errs.positive("KAFKA_PRODUCER_BATCH_TIMEOUT_MS", int64(c.ProducerBatchTimeout))
	errs.positive("KAFKA_PRODUCER_WRITE_TIMEOUT_MS", int64(c.ProducerWriteTimeout))
	errs.positive("KAFKA_PRODUCER_MAX_ATTEMPTS", int64(c.ProducerMaxAttempts))
	errs.require(slices.Contains(validAcks, c.ProducerRequiredAcks), "KAFKA_PRODUCER_REQUIRED_ACKS must be -1, 0, or 1, got %d", c.ProducerRequiredAcks)
	errs.positive("KAFKA_CONSUMER_WORKERS", int64(c.ConsumerWorkers))
	errs.positive("KAFKA_CONSUMER_BATCH_RECORDS", int64(c.ConsumerBatchRecords))
	errs.positive("KAFKA_CONSUMER_BATCH_BYTES", int64(c.ConsumerBatchBytes))
	errs.positive("KAFKA_CONSUMER_FLUSH_INTERVAL_MS", int64(c.ConsumerFlushInterval))
	errs.positive("KAFKA_CONSUMER_READ_MAX_BYTES", int64(c.ConsumerReadMaxBytes))
	errs.positive("KAFKA_CONSUMER_MAX_ATTEMPTS", int64(c.ConsumerMaxAttempts))
	errs.positive("KAFKA_CONSUMER_RETRY_INITIAL_MS", int64(c.ConsumerRetryInitial))
	errs.positive("KAFKA_CONSUMER_RETRY_MAX_MS", int64(c.ConsumerRetryMax))
}

func (c AlertingConfig) validate(errs *check) {
	errs.positive("ALERT_EVALUATION_INTERVAL_MS", int64(c.EvaluationInterval))
}

func (c AIConfig) validate(errs *check, development bool) {
	errs.set("KAFKA_AI_INVESTIGATION_TOPIC", c.InvestigationTopic)
	errs.set("KAFKA_AI_RETRY_TOPIC", c.RetryTopic)
	errs.set("KAFKA_AI_DLQ_TOPIC", c.DLQTopic)
	errs.set("AI_SERVICE_TOKEN", c.ServiceToken)
	errs.require(development || c.ServiceToken != PlaceholderServiceToken,
		"AI_SERVICE_TOKEN is still the example value; set a secret outside development")
	errs.positive("INVESTIGATION_OUTBOX_POLL_INTERVAL_MS", int64(c.OutboxPollInterval))
	errs.positive("INVESTIGATION_OUTBOX_BATCH_SIZE", int64(c.OutboxBatchSize))
}

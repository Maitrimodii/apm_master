// Package config loads apm_master settings from environment variables (and an
// optional .env file). Each setting is declared once, as a tagged field:
// `env` is the variable name and `envDefault` its default. Durations named
// *_MS are integer milliseconds.
package config

import (
	"fmt"
	"strings"
	"time"
)

// AppMode selects which components a process runs.
type AppMode string

const (
	// ModeAll runs the API, ingest consumers and the scheduler in one process.
	ModeAll AppMode = "all"
	// ModeAPI runs the API and ingest consumers only.
	ModeAPI AppMode = "api"
	// ModeScheduler runs alert evaluation and retention only.
	ModeScheduler AppMode = "scheduler"
)

// RunsScheduler reports whether background tasks run in this process.
func (m AppMode) RunsScheduler() bool { return m == ModeAll || m == ModeScheduler }

// EnvDevelopment is the APP_ENV for local development.
const EnvDevelopment = "development"

// PlaceholderServiceToken is the example AI_SERVICE_TOKEN; it is rejected
// outside development.
const PlaceholderServiceToken = "apm-ai-secret-token-change-in-production"

type Config struct {
	App       AppConfig
	HTTP      HTTPConfig
	Postgres  PostgresConfig
	Redis     RedisConfig
	Auth      AuthConfig
	SMTP      SMTPConfig
	Notify    NotificationConfig
	Kafka     KafkaConfig
	Alerting  AlertingConfig
	Retention RetentionConfig
	AI        AIConfig
}

type AppConfig struct {
	Env      string  `env:"APP_ENV" envDefault:"development"`
	Mode     AppMode `env:"APP_MODE" envDefault:"all"`
	LogLevel string  `env:"LOG_LEVEL" envDefault:"info"`
	// URL is the UI's base URL, used in email links. APP_URL wins over the
	// older APPURL name.
	URL string `env:"APPURL" envDefault:"http://localhost:3000"`
	// AllowedOrigins are the browser origins allowed by CORS. Empty means URL.
	AllowedOrigins []string `env:"CORS_ALLOWED_ORIGINS" envSeparator:","`
	// AutoMigrate applies database migrations at startup. Defaults to true
	// only in development; run cmd/migrate as a deploy step elsewhere.
	AutoMigrate *bool `env:"AUTO_MIGRATE"`
}

// IsDevelopment reports whether the process runs in local development.
func (c AppConfig) IsDevelopment() bool { return strings.EqualFold(c.Env, EnvDevelopment) }

// Origins returns the CORS allow-list.
func (c AppConfig) Origins() []string {
	if len(c.AllowedOrigins) > 0 {
		return c.AllowedOrigins
	}
	return []string{c.URL}
}

// ShouldAutoMigrate reports whether to apply migrations at startup.
func (c AppConfig) ShouldAutoMigrate() bool {
	if c.AutoMigrate != nil {
		return *c.AutoMigrate
	}
	return c.IsDevelopment()
}

type HTTPConfig struct {
	Port          string        `env:"HTTP_PORT" envDefault:"8080"`
	ReadTimeout   time.Duration `env:"HTTP_READ_TIMEOUT_MS" envDefault:"10000"`
	WriteTimeout  time.Duration `env:"HTTP_WRITE_TIMEOUT_MS" envDefault:"30000"`
	IdleTimeout   time.Duration `env:"HTTP_IDLE_TIMEOUT_MS" envDefault:"120000"`
	HeaderTimeout time.Duration `env:"HTTP_HEADER_TIMEOUT_MS" envDefault:"5000"`
	MaxBodyBytes  int64         `env:"MAX_BODY_BYTES" envDefault:"10485760"`
	// RateLimitBurst requests are allowed per RateLimitWindow, per client IP
	// and per organization.
	RateLimitBurst  int           `env:"RATE_LIMIT_BURST" envDefault:"200"`
	RateLimitWindow time.Duration `env:"RATE_LIMIT_WINDOW_MS" envDefault:"1000"`
	// TrustedProxies are the addresses (IPs or CIDRs) of reverse proxies in
	// front of the API. Only requests arriving from them may set the client
	// IP through X-Forwarded-For / X-Real-IP; everyone else's headers are
	// ignored. Empty (the default) trusts no proxy.
	TrustedProxies []string `env:"TRUSTED_PROXIES" envSeparator:","`
}

type PostgresConfig struct {
	Host            string        `env:"POSTGRES_HOST" envDefault:"localhost"`
	Port            string        `env:"POSTGRES_PORT" envDefault:"5432"`
	User            string        `env:"POSTGRES_USER" envDefault:"apm_user"`
	Password        string        `env:"POSTGRES_PASSWORD"`
	DBName          string        `env:"POSTGRES_DB" envDefault:"apm_db"`
	Timeout         time.Duration `env:"POSTGRES_TIMEOUT_MS" envDefault:"5000"`
	MaxConns        int32         `env:"POSTGRES_MAX_CONNS" envDefault:"50"`
	MinConns        int32         `env:"POSTGRES_MIN_CONNS" envDefault:"5"`
	MaxConnIdleTime time.Duration `env:"POSTGRES_MAX_CONN_IDLE_TIME_MS" envDefault:"900000"`
	MaxConnLifetime time.Duration `env:"POSTGRES_MAX_CONN_LIFETIME_MS" envDefault:"3600000"`
}

// MigrateDSN is the connection string golang-migrate expects.
func (c PostgresConfig) MigrateDSN() string { return c.dsn("pgx5") }

// PoolDSN is the connection string pgxpool expects.
func (c PostgresConfig) PoolDSN() string { return c.dsn("postgres") }

func (c PostgresConfig) dsn(scheme string) string {
	return fmt.Sprintf("%s://%s:%s@%s:%s/%s?sslmode=disable", scheme, c.User, c.Password, c.Host, c.Port, c.DBName)
}

type RedisConfig struct {
	// URL is redis://… or a bare host:port. Outside development it must carry
	// a password (redis://:<password>@host:6379/0): Redis holds the API-key
	// cache, so unauthenticated access could map a key to any organization.
	URL     string        `env:"REDIS_URL" envDefault:"localhost:6379"`
	Timeout time.Duration `env:"REDIS_TIMEOUT_MS" envDefault:"5000"`
}

// AuthConfig holds token lifetimes. RefreshSessionMaxDays is the absolute
// lifetime of a login session: refresh-token rotation never extends a session
// past it. PasswordResetCooldownSecs is how long after one reset link another
// request for the same account is ignored (0 disables the cooldown).
// InviteResendCooldownSecs is the minimum time between two emails for one
// invite; a resend inside it gets 429 (0 disables the cooldown).
type AuthConfig struct {
	JWTSecret                 string `env:"JWT_SECRET"`
	AccessTokenExpiryMins     int    `env:"ACCESS_TOKEN_EXPIRY_MINS" envDefault:"15"`
	RefreshTokenExpiryDays    int    `env:"REFRESH_TOKEN_EXPIRY_DAYS" envDefault:"7"`
	RefreshSessionMaxDays     int    `env:"REFRESH_SESSION_MAX_DAYS" envDefault:"30"`
	InviteExpiryDays          int    `env:"INVITE_EXPIRY_DAYS" envDefault:"10"`
	PasswordResetExpiryMins   int    `env:"PASSWORD_RESET_EXPIRY_MINS" envDefault:"30"`
	PasswordResetCooldownSecs int    `env:"PASSWORD_RESET_COOLDOWN_SECS" envDefault:"60"`
	InviteResendCooldownSecs  int    `env:"INVITE_RESEND_COOLDOWN_SECS" envDefault:"60"`
}

func (c AuthConfig) AccessTokenTTL() time.Duration {
	return time.Duration(c.AccessTokenExpiryMins) * time.Minute
}
func (c AuthConfig) RefreshTokenTTL() time.Duration {
	return time.Duration(c.RefreshTokenExpiryDays) * 24 * time.Hour
}
func (c AuthConfig) RefreshSessionMaxTTL() time.Duration {
	return time.Duration(c.RefreshSessionMaxDays) * 24 * time.Hour
}
func (c AuthConfig) InviteTTL() time.Duration {
	return time.Duration(c.InviteExpiryDays) * 24 * time.Hour
}
func (c AuthConfig) PasswordResetTTL() time.Duration {
	return time.Duration(c.PasswordResetExpiryMins) * time.Minute
}

// InviteResendCooldown is the minimum time between two emails for one invite
// (0 disables it).
func (c AuthConfig) InviteResendCooldown() time.Duration {
	return time.Duration(c.InviteResendCooldownSecs) * time.Second
}
func (c AuthConfig) PasswordResetCooldown() time.Duration {
	return time.Duration(c.PasswordResetCooldownSecs) * time.Second
}

// SMTPConfig is outgoing mail. STARTTLS is required: mail is never sent in
// plaintext unless AllowInsecure, which only development accepts (for a local
// mail catcher). Outside development the server must authenticate us
// (Username/Password) or be declared a TrustedRelay that accepts mail from
// this network without credentials.
type SMTPConfig struct {
	From          string `env:"EMAIL_FROM" envDefault:"no-reply@apm-master.local"`
	Host          string `env:"SMTP_HOST"`
	Port          string `env:"SMTP_PORT" envDefault:"587"`
	Username      string `env:"SMTP_USERNAME"`
	Password      string `env:"SMTP_PASSWORD"`
	AllowInsecure bool   `env:"SMTP_ALLOW_INSECURE" envDefault:"false"`
	TrustedRelay  bool   `env:"SMTP_TRUSTED_RELAY" envDefault:"false"`
}

type KafkaConfig struct {
	Brokers       []string `env:"KAFKA_BROKERS" envDefault:"localhost:9092" envSeparator:","`
	ClientID      string   `env:"KAFKA_CLIENT_ID" envDefault:"apm-master-ingest"`
	ConsumerGroup string   `env:"KAFKA_CONSUMER_GROUP" envDefault:"apm-master-ingest-writers"`

	TracesTopic     string `env:"KAFKA_TRACES_TOPIC" envDefault:"apm.telemetry.traces.v1"`
	LogsTopic       string `env:"KAFKA_LOGS_TOPIC" envDefault:"apm.telemetry.logs.v1"`
	MetricsTopic    string `env:"KAFKA_METRICS_TOPIC" envDefault:"apm.telemetry.metrics.v1"`
	TracesDLQTopic  string `env:"KAFKA_TRACES_DLQ_TOPIC" envDefault:"apm.telemetry.traces.dlq.v1"`
	LogsDLQTopic    string `env:"KAFKA_LOGS_DLQ_TOPIC" envDefault:"apm.telemetry.logs.dlq.v1"`
	MetricsDLQTopic string `env:"KAFKA_METRICS_DLQ_TOPIC" envDefault:"apm.telemetry.metrics.dlq.v1"`

	TopicPartitions        int           `env:"KAFKA_TOPIC_PARTITIONS" envDefault:"12"`
	TopicReplicationFactor int           `env:"KAFKA_TOPIC_REPLICATION_FACTOR" envDefault:"1"`
	TopicRetention         time.Duration `env:"KAFKA_TOPIC_RETENTION_MS" envDefault:"604800000"`
	Compression            string        `env:"KAFKA_COMPRESSION" envDefault:"lz4"`

	ProducerBatchSize    int           `env:"KAFKA_PRODUCER_BATCH_SIZE" envDefault:"1000"`
	ProducerBatchBytes   int           `env:"KAFKA_PRODUCER_BATCH_BYTES" envDefault:"1048576"`
	ProducerBatchTimeout time.Duration `env:"KAFKA_PRODUCER_BATCH_TIMEOUT_MS" envDefault:"10"`
	ProducerWriteTimeout time.Duration `env:"KAFKA_PRODUCER_WRITE_TIMEOUT_MS" envDefault:"10000"`
	ProducerRequiredAcks int           `env:"KAFKA_PRODUCER_REQUIRED_ACKS" envDefault:"-1"`
	ProducerMaxAttempts  int           `env:"KAFKA_PRODUCER_MAX_ATTEMPTS" envDefault:"10"`

	ConsumerWorkers       int           `env:"KAFKA_CONSUMER_WORKERS" envDefault:"3"`
	ConsumerBatchRecords  int           `env:"KAFKA_CONSUMER_BATCH_RECORDS" envDefault:"1000"`
	ConsumerBatchBytes    int           `env:"KAFKA_CONSUMER_BATCH_BYTES" envDefault:"4194304"`
	ConsumerFlushInterval time.Duration `env:"KAFKA_CONSUMER_FLUSH_INTERVAL_MS" envDefault:"1000"`
	ConsumerReadMaxBytes  int           `env:"KAFKA_CONSUMER_READ_MAX_BYTES" envDefault:"10485760"`
	ConsumerMaxAttempts   int           `env:"KAFKA_CONSUMER_MAX_ATTEMPTS" envDefault:"5"`
	ConsumerRetryInitial  time.Duration `env:"KAFKA_CONSUMER_RETRY_INITIAL_MS" envDefault:"250"`
	ConsumerRetryMax      time.Duration `env:"KAFKA_CONSUMER_RETRY_MAX_MS" envDefault:"5000"`
}

type AlertingConfig struct {
	EvaluationInterval time.Duration `env:"ALERT_EVALUATION_INTERVAL_MS" envDefault:"60000"`
}

// NotificationConfig protects queued emails that carry one-time links
// (invites, password resets): their payload is encrypted with these keys and
// never stored in plaintext.
//
// EncryptionKeys is "id:base64key,..." with 32-byte keys (generate one with
// `openssl rand -base64 32`); EncryptionKeyID selects the key that encrypts.
// Every process that sends email (API and scheduler) needs the same keys.
// Dead-lettered jobs keep their encrypted payload for DeadLetterRecoveryHours
// so an operator can fix the cause and re-queue them; then it is erased.
type NotificationConfig struct {
	EncryptionKeys          string `env:"NOTIFICATION_ENCRYPTION_KEYS"`
	EncryptionKeyID         string `env:"NOTIFICATION_ENCRYPTION_KEY_ID"`
	DeadLetterRecoveryHours int    `env:"NOTIFICATION_DEAD_LETTER_RECOVERY_HOURS" envDefault:"72"`
	// StaleProcessingMinutes is how long a job may stay 'processing' before
	// it is assumed abandoned (its worker died) and requeued.
	StaleProcessingMinutes int `env:"NOTIFICATION_STALE_PROCESSING_MINUTES" envDefault:"15"`
}

// StaleProcessing is the age after which a 'processing' job is requeued.
func (c NotificationConfig) StaleProcessing() time.Duration {
	return time.Duration(c.StaleProcessingMinutes) * time.Minute
}

// DeadLetterRecovery is how long a dead-lettered job keeps its ciphertext.
func (c NotificationConfig) DeadLetterRecovery() time.Duration {
	return time.Duration(c.DeadLetterRecoveryHours) * time.Hour
}

// RetentionConfig covers tables outside Timescale's own retention policies.
type RetentionConfig struct {
	NotificationDays int `env:"NOTIFICATION_RETENTION_DAYS" envDefault:"90"`
	RefreshTokenDays int `env:"REFRESH_TOKEN_RETENTION_DAYS" envDefault:"30"`
}

// AIConfig is the integration with apm-ai-service.
type AIConfig struct {
	InvestigationTopic string        `env:"KAFKA_AI_INVESTIGATION_TOPIC" envDefault:"ai.investigation.jobs"`
	RetryTopic         string        `env:"KAFKA_AI_RETRY_TOPIC" envDefault:"ai.investigation.retry"`
	DLQTopic           string        `env:"KAFKA_AI_DLQ_TOPIC" envDefault:"ai.investigation.dlq"`
	ServiceToken       string        `env:"AI_SERVICE_TOKEN" envDefault:"apm-ai-secret-token-change-in-production"`
	OutboxPollInterval time.Duration `env:"INVESTIGATION_OUTBOX_POLL_INTERVAL_MS" envDefault:"2000"`
	OutboxBatchSize    int           `env:"INVESTIGATION_OUTBOX_BATCH_SIZE" envDefault:"50"`
}

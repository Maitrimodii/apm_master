package kafka

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/platform/kafkax"
)

const (
	defaultClientID = "apm-master-ingest"
)

type Topics struct {
	Traces     string
	Logs       string
	Metrics    string
	TracesDLQ  string
	LogsDLQ    string
	MetricsDLQ string
}

func (t Topics) TopicFor(signal model.Signal) (string, error) {
	switch signal {
	case model.SignalTraces:
		return t.Traces, nil
	case model.SignalLogs:
		return t.Logs, nil
	case model.SignalMetrics:
		return t.Metrics, nil
	default:
		return "", fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (t Topics) DLQFor(signal model.Signal) (string, error) {
	switch signal {
	case model.SignalTraces:
		return t.TracesDLQ, nil
	case model.SignalLogs:
		return t.LogsDLQ, nil
	case model.SignalMetrics:
		return t.MetricsDLQ, nil
	default:
		return "", fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

type Config struct {
	Brokers                []string
	ClientID               string
	GroupID                string
	Topics                 Topics
	Compression            string
	TopicPartitions        int
	TopicReplicationFactor int
	TopicRetention         time.Duration

	ProducerBatchSize    int
	ProducerBatchBytes   int
	ProducerBatchTimeout time.Duration
	ProducerWriteTimeout time.Duration
	ProducerRequiredAcks int
	ProducerMaxAttempts  int

	ConsumerWorkers       int
	ConsumerBatchRecords  int
	ConsumerBatchBytes    int
	ConsumerFlushInterval time.Duration
	ConsumerReadMaxBytes  int
	ConsumerMaxAttempts   int
	ConsumerRetryInitial  time.Duration
	ConsumerRetryMax      time.Duration
}

// FromAppConfig builds the ingest Kafka settings from application config.
func FromAppConfig(cfg config.KafkaConfig) Config {
	return Config{
		Brokers:     cfg.Brokers,
		ClientID:    defaultString(cfg.ClientID, defaultClientID),
		GroupID:     cfg.ConsumerGroup,
		Compression: cfg.Compression,
		Topics: Topics{
			Traces:     cfg.TracesTopic,
			Logs:       cfg.LogsTopic,
			Metrics:    cfg.MetricsTopic,
			TracesDLQ:  cfg.TracesDLQTopic,
			LogsDLQ:    cfg.LogsDLQTopic,
			MetricsDLQ: cfg.MetricsDLQTopic,
		},
		TopicPartitions:        cfg.TopicPartitions,
		TopicReplicationFactor: cfg.TopicReplicationFactor,
		TopicRetention:         cfg.TopicRetention,
		ProducerBatchSize:      cfg.ProducerBatchSize,
		ProducerBatchBytes:     cfg.ProducerBatchBytes,
		ProducerBatchTimeout:   cfg.ProducerBatchTimeout,
		ProducerWriteTimeout:   cfg.ProducerWriteTimeout,
		ProducerRequiredAcks:   cfg.ProducerRequiredAcks,
		ProducerMaxAttempts:    cfg.ProducerMaxAttempts,
		ConsumerWorkers:        cfg.ConsumerWorkers,
		ConsumerBatchRecords:   cfg.ConsumerBatchRecords,
		ConsumerBatchBytes:     cfg.ConsumerBatchBytes,
		ConsumerFlushInterval:  cfg.ConsumerFlushInterval,
		ConsumerReadMaxBytes:   cfg.ConsumerReadMaxBytes,
		ConsumerMaxAttempts:    cfg.ConsumerMaxAttempts,
		ConsumerRetryInitial:   cfg.ConsumerRetryInitial,
		ConsumerRetryMax:       cfg.ConsumerRetryMax,
	}
}

// EnsureTopics creates the telemetry and dead-letter topics if missing.
func EnsureTopics(ctx context.Context, cfg Config) error {
	specs := make([]kafkax.TopicSpec, 0, 6)
	for _, name := range []string{cfg.Topics.Traces, cfg.Topics.Logs, cfg.Topics.Metrics, cfg.Topics.TracesDLQ, cfg.Topics.LogsDLQ, cfg.Topics.MetricsDLQ} {
		specs = append(specs, kafkax.TopicSpec{
			Name:              name,
			Partitions:        cfg.TopicPartitions,
			ReplicationFactor: cfg.TopicReplicationFactor,
			Retention:         cfg.TopicRetention,
			Compression:       cfg.Compression,
		})
	}
	if err := kafkax.EnsureTopics(ctx, cfg.Brokers, specs...); err != nil {
		return fmt.Errorf("ensure telemetry topics: %w", err)
	}
	return nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

package kafka

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	kafkago "github.com/segmentio/kafka-go"
)

const (
	defaultClientID = "apm-master-ingest"

	CompressionNone   = "none"
	CompressionGzip   = "gzip"
	CompressionSnappy = "snappy"
	CompressionLz4    = "lz4"
)

type Topics struct {
	Traces     string
	Logs       string
	Metrics    string
	TracesDLQ  string
	LogsDLQ    string
	MetricsDLQ string
}

func (testRunner Topics) TopicFor(signal model.Signal) (string, error) {
	switch signal {
	case model.SignalTraces:
		return testRunner.Traces, nil
	case model.SignalLogs:
		return testRunner.Logs, nil
	case model.SignalMetrics:
		return testRunner.Metrics, nil
	default:
		return "", fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (testRunner Topics) DLQFor(signal model.Signal) (string, error) {
	switch signal {
	case model.SignalTraces:
		return testRunner.TracesDLQ, nil
	case model.SignalLogs:
		return testRunner.LogsDLQ, nil
	case model.SignalMetrics:
		return testRunner.MetricsDLQ, nil
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

func FromAppConfig(cfg *config.Config) Config {
	return Config{
		Brokers:     splitCSV(cfg.KafkaBrokers),
		ClientID:    defaultString(cfg.KafkaClientID, defaultClientID),
		GroupID:     cfg.KafkaConsumerGroup,
		Compression: cfg.KafkaCompression,
		Topics: Topics{
			Traces:     cfg.KafkaTracesTopic,
			Logs:       cfg.KafkaLogsTopic,
			Metrics:    cfg.KafkaMetricsTopic,
			TracesDLQ:  cfg.KafkaTracesDLQTopic,
			LogsDLQ:    cfg.KafkaLogsDLQTopic,
			MetricsDLQ: cfg.KafkaMetricsDLQTopic,
		},
		TopicPartitions:        cfg.KafkaTopicPartitions,
		TopicReplicationFactor: cfg.KafkaTopicReplicationFactor,
		TopicRetention:         cfg.KafkaTopicRetention,
		ProducerBatchSize:      cfg.KafkaProducerBatchSize,
		ProducerBatchBytes:     cfg.KafkaProducerBatchBytes,
		ProducerBatchTimeout:   cfg.KafkaProducerBatchTimeout,
		ProducerWriteTimeout:   cfg.KafkaProducerWriteTimeout,
		ProducerRequiredAcks:   cfg.KafkaProducerRequiredAcks,
		ProducerMaxAttempts:    cfg.KafkaProducerMaxAttempts,
		ConsumerWorkers:        cfg.KafkaConsumerWorkers,
		ConsumerBatchRecords:   cfg.KafkaConsumerBatchRecords,
		ConsumerBatchBytes:     cfg.KafkaConsumerBatchBytes,
		ConsumerFlushInterval:  cfg.KafkaConsumerFlushInterval,
		ConsumerReadMaxBytes:   cfg.KafkaConsumerReadMaxBytes,
		ConsumerMaxAttempts:    cfg.KafkaConsumerMaxAttempts,
		ConsumerRetryInitial:   cfg.KafkaConsumerRetryInitial,
		ConsumerRetryMax:       cfg.KafkaConsumerRetryMax,
	}
}

func EnsureTopics(ctx context.Context, cfg Config) error {
	if len(cfg.Brokers) == 0 {
		return fmt.Errorf("kafka brokers are required")
	}

	client := &kafkago.Client{
		Addr:    kafkago.TCP(cfg.Brokers[0]),
		Timeout: 10 * time.Second,
	}

	retention := fmt.Sprintf("%d", cfg.TopicRetention.Milliseconds())
	resp, err := client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{
		Topics: []kafkago.TopicConfig{
			topicConfig(cfg.Topics.Traces, cfg, retention),
			topicConfig(cfg.Topics.Logs, cfg, retention),
			topicConfig(cfg.Topics.Metrics, cfg, retention),
			topicConfig(cfg.Topics.TracesDLQ, cfg, retention),
			topicConfig(cfg.Topics.LogsDLQ, cfg, retention),
			topicConfig(cfg.Topics.MetricsDLQ, cfg, retention),
		},
	})
	if err != nil {
		return fmt.Errorf("create kafka telemetry topics: %w", err)
	}

	for topic, topicErr := range resp.Errors {
		if topicErr == nil || errors.Is(topicErr, kafkago.TopicAlreadyExists) {
			continue
		}
		return fmt.Errorf("create kafka topic %s: %w", topic, topicErr)
	}

	return nil
}

func topicConfig(topic string, cfg Config, retention string) kafkago.TopicConfig {
	return kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     cfg.TopicPartitions,
		ReplicationFactor: cfg.TopicReplicationFactor,
		ConfigEntries: []kafkago.ConfigEntry{
			{ConfigName: "cleanup.policy", ConfigValue: "delete"},
			{ConfigName: "retention.ms", ConfigValue: retention},
			{ConfigName: "compression.type", ConfigValue: cfg.Compression},
		},
	}
}

func CompressionCodec(name string) (kafkago.CompressionCodec, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", CompressionNone:
		return nil, nil
	case CompressionGzip:
		return kafkago.Gzip.Codec(), nil
	case CompressionSnappy:
		return kafkago.Snappy.Codec(), nil
	case CompressionLz4:
		return kafkago.Lz4.Codec(), nil
	default:
		return nil, fmt.Errorf("unsupported kafka compression %q", name)
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

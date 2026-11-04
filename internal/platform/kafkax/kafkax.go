// Package kafkax holds the Kafka plumbing shared by telemetry ingest and the
// investigation outbox: topic creation, compression codecs and writers.
package kafkax

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Compression names accepted in configuration.
const (
	CompressionNone   = "none"
	CompressionGzip   = "gzip"
	CompressionSnappy = "snappy"
	CompressionLz4    = "lz4"
	CompressionZstd   = "zstd"
)

const adminTimeout = 10 * time.Second

// TopicSpec describes a topic to create if it does not exist.
type TopicSpec struct {
	Name              string
	Partitions        int
	ReplicationFactor int
	Retention         time.Duration
	// Compression is the broker-side compression.type; empty keeps the default.
	Compression string
}

// EnsureTopics creates any missing topics. Existing topics are left as they are.
func EnsureTopics(ctx context.Context, brokers []string, specs ...TopicSpec) error {
	if len(brokers) == 0 {
		return errors.New("kafka brokers are required")
	}
	configs := make([]kafkago.TopicConfig, 0, len(specs))
	for _, s := range specs {
		entries := []kafkago.ConfigEntry{
			{ConfigName: "cleanup.policy", ConfigValue: "delete"},
			{ConfigName: "retention.ms", ConfigValue: strconv.FormatInt(s.Retention.Milliseconds(), 10)},
		}
		if s.Compression != "" {
			entries = append(entries, kafkago.ConfigEntry{ConfigName: "compression.type", ConfigValue: s.Compression})
		}
		configs = append(configs, kafkago.TopicConfig{
			Topic:             s.Name,
			NumPartitions:     s.Partitions,
			ReplicationFactor: s.ReplicationFactor,
			ConfigEntries:     entries,
		})
	}

	client := &kafkago.Client{Addr: kafkago.TCP(brokers...), Timeout: adminTimeout}
	resp, err := client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{Topics: configs})
	if err != nil {
		return fmt.Errorf("create kafka topics: %w", err)
	}
	for topic, topicErr := range resp.Errors {
		if topicErr != nil && !errors.Is(topicErr, kafkago.TopicAlreadyExists) {
			return fmt.Errorf("create kafka topic %s: %w", topic, topicErr)
		}
	}
	return nil
}

// CompressionCodec maps a configured compression name to a codec.
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
	case CompressionZstd:
		return kafkago.Zstd.Codec(), nil
	default:
		return nil, fmt.Errorf("unsupported kafka compression %q", name)
	}
}

// SplitBrokers parses a comma-separated broker list.
func SplitBrokers(csv string) []string {
	var out []string
	for _, b := range strings.Split(csv, ",") {
		if b = strings.TrimSpace(b); b != "" {
			out = append(out, b)
		}
	}
	return out
}

// WriterConfig configures a producer.
type WriterConfig struct {
	Brokers      []string
	Compression  string
	BatchSize    int
	BatchBytes   int
	BatchTimeout time.Duration
	WriteTimeout time.Duration
	MaxAttempts  int
	RequiredAcks kafkago.RequiredAcks
	Logger       func(msg string)
}

// NewWriter returns a synchronous writer that routes each message by its own
// Topic field. The writer itself has no topic: kafka-go rejects messages
// that name a topic when the writer has one too.
func NewWriter(cfg WriterConfig) (*kafkago.Writer, error) {
	codec, err := CompressionCodec(cfg.Compression)
	if err != nil {
		return nil, err
	}
	w := &kafkago.Writer{
		Addr:         kafkago.TCP(cfg.Brokers...),
		Balancer:     &kafkago.Hash{},
		BatchSize:    cfg.BatchSize,
		BatchBytes:   int64(cfg.BatchBytes),
		BatchTimeout: cfg.BatchTimeout,
		WriteTimeout: cfg.WriteTimeout,
		ReadTimeout:  cfg.WriteTimeout,
		MaxAttempts:  cfg.MaxAttempts,
		RequiredAcks: cfg.RequiredAcks,
		Compression:  compressionFor(codec),
	}
	if cfg.Logger != nil {
		w.ErrorLogger = kafkago.LoggerFunc(func(format string, args ...any) { cfg.Logger(fmt.Sprintf(format, args...)) })
	}
	return w, nil
}

func compressionFor(codec kafkago.CompressionCodec) kafkago.Compression {
	if codec == nil {
		return 0
	}
	return kafkago.Compression(codec.Code())
}

package producer

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	ingestkafka "github.com/Maitrimodii/apm_master/internal/ingest/kafka"
	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	kafkago "github.com/segmentio/kafka-go"
)

type Producer interface {
	Publish(ctx context.Context, event model.Event) error
	Close() error
}

type KafkaProducer struct {
	writer     *kafkago.Writer
	topics     ingestkafka.Topics
	serializer serializer.JSON
	log        *slog.Logger
}

func NewKafkaProducer(cfg ingestkafka.Config, log *slog.Logger) (*KafkaProducer, error) {
	if log == nil {
		log = slog.Default()
	}

	codec, err := ingestkafka.CompressionCodec(cfg.Compression)
	if err != nil {
		return nil, err
	}

	writer := kafkago.NewWriter(kafkago.WriterConfig{
		Brokers:          cfg.Brokers,
		Balancer:         &kafkago.Hash{},
		MaxAttempts:      cfg.ProducerMaxAttempts,
		BatchSize:        cfg.ProducerBatchSize,
		BatchBytes:       cfg.ProducerBatchBytes,
		BatchTimeout:     cfg.ProducerBatchTimeout,
		WriteTimeout:     cfg.ProducerWriteTimeout,
		ReadTimeout:      cfg.ProducerWriteTimeout,
		RequiredAcks:     cfg.ProducerRequiredAcks,
		CompressionCodec: codec,
		Async:            true,
		ErrorLogger: kafkago.LoggerFunc(func(format string, args ...any) {
			log.Error("kafka producer error", "message", fmt.Sprintf(format, args...))
		}),
	})

	return &KafkaProducer{
		writer: writer,
		topics: cfg.Topics,
		log:    log,
	}, nil
}

func (producer *KafkaProducer) Publish(ctx context.Context, event model.Event) error {
	topic, err := producer.topics.TopicFor(event.Signal)
	if err != nil {
		return err
	}

	value, err := producer.serializer.Marshal(event)
	if err != nil {
		return err
	}

	var key []byte
	switch event.Signal {
	case model.SignalTraces:
		if event.TraceID != "" {
			key = []byte(event.TraceID)
		} else {
			key = []byte(event.OrgID.String())
		}
	case model.SignalLogs, model.SignalMetrics:
		if event.ServiceName != "" {
			key = []byte(event.OrgID.String() + ":" + event.ServiceName)
		} else {
			key = []byte(event.OrgID.String())
		}
	default:
		key = []byte(event.OrgID.String())
	}

	start := time.Now()
	if err := producer.writer.WriteMessages(ctx, kafkago.Message{
		Topic: topic,
		Key:   key,
		Value: value,
		Time:  event.ProducedAt,
		Headers: []kafkago.Header{
			{Key: "schema_version", Value: []byte(fmt.Sprintf("%d", event.SchemaVersion))},
			{Key: "signal", Value: []byte(string(event.Signal))},
			{Key: "event_id", Value: []byte(event.EventID.String())},
			{Key: "org_id", Value: []byte(event.OrgID.String())},
		},
	}); err != nil {
		return fmt.Errorf("publish %s telemetry to kafka topic %s: %w", event.Signal, topic, err)
	}

	producer.log.Info(
		"telemetry event queued asynchronously",
		"event_id", event.EventID,
		"signal", event.Signal,
		"topic", topic,
		"org_id", event.OrgID,
		"accepted", event.Metadata.AcceptedRecords,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return nil
}

func (producer *KafkaProducer) Close() error {
	if err := producer.writer.Close(); err != nil {
		return fmt.Errorf("close kafka producer: %w", err)
	}
	return nil
}

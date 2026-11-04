package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"

	ingestkafka "github.com/Maitrimodii/apm_master/internal/ingest/kafka"
	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/platform/kafkax"
	"github.com/Maitrimodii/apm_master/internal/platform/retry"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

type SpanWriter interface {
	BulkInsert(context.Context, []traces.Span) error
}

type LogWriter interface {
	BulkInsert(context.Context, []logs.LogRecord) error
}

type MetricWriter interface {
	BulkInsert(context.Context, []metrics.WriteBatch) error
}

type Persistence struct {
	Spans   SpanWriter
	Logs    LogWriter
	Metrics MetricWriter
}

type Runner struct {
	cfg         ingestkafka.Config
	persistence Persistence
	dlqWriter   *kafkago.Writer
	serializer  serializer.JSON
	log         *slog.Logger
	cancel      context.CancelFunc
	done        chan struct{}
}

func NewRunner(cfg ingestkafka.Config, persistence Persistence, log *slog.Logger) (*Runner, error) {
	if log == nil {
		log = slog.Default()
	}

	codec, err := kafkax.CompressionCodec(cfg.Compression)
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:         cfg,
		persistence: persistence,
		dlqWriter: kafkago.NewWriter(kafkago.WriterConfig{
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
			Async:            false,
			ErrorLogger: kafkago.LoggerFunc(func(format string, args ...any) {
				log.Error("kafka dlq writer error", "message", fmt.Sprintf(format, args...))
			}),
		}),
		log:  log,
		done: make(chan struct{}),
	}, nil
}

func (r *Runner) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.done = make(chan struct{})

	var wg sync.WaitGroup
	for _, spec := range r.signalSpecs() {
		for workerID := 0; workerID < r.cfg.ConsumerWorkers; workerID++ {
			wg.Add(1)
			go func(spec signalSpec, workerID int) {
				defer wg.Done()
				r.consumeSignal(ctx, spec, workerID)
			}(spec, workerID)
		}
	}

	go func() {
		wg.Wait()
		close(r.done)
	}()
}

func (r *Runner) Shutdown(ctx context.Context) error {
	if r.cancel != nil {
		r.cancel()
	}

	select {
	case <-r.done:
	case <-ctx.Done():
		return fmt.Errorf("shutdown kafka consumers: %w", ctx.Err())
	}

	if err := r.dlqWriter.Close(); err != nil {
		return fmt.Errorf("close kafka dlq writer: %w", err)
	}
	return nil
}

func (r *Runner) Done() <-chan struct{} {
	return r.done
}

type signalSpec struct {
	signal model.Signal
	topic  string
	dlq    string
}

func (r *Runner) signalSpecs() []signalSpec {
	return []signalSpec{
		{signal: model.SignalTraces, topic: r.cfg.Topics.Traces, dlq: r.cfg.Topics.TracesDLQ},
		{signal: model.SignalLogs, topic: r.cfg.Topics.Logs, dlq: r.cfg.Topics.LogsDLQ},
		{signal: model.SignalMetrics, topic: r.cfg.Topics.Metrics, dlq: r.cfg.Topics.MetricsDLQ},
	}
}

func (r *Runner) consumeSignal(ctx context.Context, spec signalSpec, workerID int) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:               r.cfg.Brokers,
		GroupID:               r.cfg.GroupID,
		Topic:                 spec.topic,
		MinBytes:              1,
		MaxBytes:              r.cfg.ConsumerReadMaxBytes,
		MaxWait:               r.cfg.ConsumerFlushInterval,
		CommitInterval:        0,
		WatchPartitionChanges: true,
		StartOffset:           kafkago.FirstOffset,
		MaxAttempts:           r.cfg.ConsumerMaxAttempts,
		ReadBackoffMin:        r.cfg.ConsumerRetryInitial,
		ReadBackoffMax:        r.cfg.ConsumerRetryMax,
		ErrorLogger: kafkago.LoggerFunc(func(format string, args ...any) {
			r.log.Error("kafka consumer error", "signal", spec.signal, "worker_id", workerID, "message", fmt.Sprintf(format, args...))
		}),
	})
	defer func() {
		if err := reader.Close(); err != nil {
			r.log.Warn("close kafka reader", "signal", spec.signal, "error", err)
		}
	}()

	r.log.Info("kafka telemetry consumer started", "signal", spec.signal, "topic", spec.topic, "worker_id", workerID)

	batch := newMessageBatch(r.cfg.ConsumerBatchRecords, r.cfg.ConsumerBatchBytes)
	ticker := time.NewTicker(r.cfg.ConsumerFlushInterval)
	defer ticker.Stop()

	type msgOrErr struct {
		msg kafkago.Message
		err error
	}

	msgChan := make(chan msgOrErr)
	go func() {
		for {
			msg, err := reader.FetchMessage(ctx)
			if err != nil && errors.Is(err, context.Canceled) {
				return
			}
			select {
			case <-ctx.Done():
				return
			case msgChan <- msgOrErr{msg: msg, err: err}:
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			_ = r.flushBatch(context.WithoutCancel(ctx), reader, spec, batch)
			r.log.Info("kafka telemetry consumer stopped", "signal", spec.signal, "worker_id", workerID)
			return
		case <-ticker.C:
			if err := r.flushBatch(ctx, reader, spec, batch); err != nil {
				r.log.Error("periodic flush batch failed", "signal", spec.signal, "worker_id", workerID, "error", err)
				time.Sleep(1 * time.Second)
			}
		case res, ok := <-msgChan:
			if !ok {
				return
			}
			if res.err != nil {
				r.log.Error("fetch kafka telemetry message failed", "signal", spec.signal, "worker_id", workerID, "error", res.err)
				continue
			}

			batch.add(res.msg)

			if batch.full() {
				if err := r.flushBatch(ctx, reader, spec, batch); err != nil {
					r.log.Error("full flush batch failed", "signal", spec.signal, "worker_id", workerID, "error", err)
					time.Sleep(1 * time.Second)
				}
			}
		}
	}
}

func (r *Runner) flushBatch(ctx context.Context, reader *kafkago.Reader, spec signalSpec, batch *messageBatch) error {
	if batch.len() == 0 {
		return nil
	}

	messages := batch.getMessages()
	start := time.Now()
	if err := r.persistBatch(ctx, spec, messages); err != nil {
		return fmt.Errorf("persist batch: %w", err)
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		return fmt.Errorf("commit messages: %w", err)
	}

	batch.reset()
	r.log.Info("telemetry batch persisted", "signal", spec.signal, "messages", len(messages), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (r *Runner) persistBatch(ctx context.Context, spec signalSpec, messages []kafkago.Message) error {
	validEvents := make([]model.Event, 0, len(messages))
	for _, msg := range messages {
		event, err := r.serializer.Unmarshal(msg.Value)
		if err != nil {
			if dlqErr := r.writeDLQ(ctx, spec, msg, err); dlqErr != nil {
				return fmt.Errorf("write serialization failure to dlq: %w", dlqErr)
			}
			continue
		}
		if event.Signal != spec.signal {
			if dlqErr := r.writeDLQ(ctx, spec, msg, fmt.Errorf("event signal %q does not match topic signal %q", event.Signal, spec.signal)); dlqErr != nil {
				return fmt.Errorf("write signal mismatch to dlq: %w", dlqErr)
			}
			continue
		}

		// Validate payload format before bulk insert to catch serialization errors
		if err := validatePayload(event.Signal, event.Payload); err != nil {
			if dlqErr := r.writeDLQ(ctx, spec, msg, fmt.Errorf("invalid payload format: %w", err)); dlqErr != nil {
				return fmt.Errorf("write invalid payload to dlq: %w", dlqErr)
			}
			continue
		}

		validEvents = append(validEvents, event)
	}

	if len(validEvents) == 0 {
		return nil
	}

	persist := func() error {
		return r.persistEvents(ctx, spec.signal, validEvents)
	}

	policy := retry.Policy{
		MaxAttempts: r.cfg.ConsumerMaxAttempts,
		Initial:     r.cfg.ConsumerRetryInitial,
		Max:         r.cfg.ConsumerRetryMax,
	}
	if err := retry.Do(ctx, policy, persist); err != nil {
		return fmt.Errorf("db bulk insert failed: %w", err)
	}

	return nil
}

func validatePayload(signal model.Signal, payload []byte) error {
	switch signal {
	case model.SignalTraces:
		_, err := serializer.UnmarshalPayload[[]model.SpanV1](payload)
		return err
	case model.SignalLogs:
		_, err := serializer.UnmarshalPayload[[]model.LogRecordV1](payload)
		return err
	case model.SignalMetrics:
		_, err := serializer.UnmarshalPayload[model.MetricBatchV1](payload)
		return err
	default:
		return fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (r *Runner) persistEvents(ctx context.Context, signal model.Signal, events []model.Event) error {
	switch signal {
	case model.SignalTraces:
		records := make([]traces.Span, 0, totalAccepted(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[[]model.SpanV1](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, model.SpansFromWire(payload)...)
		}
		return r.persistence.Spans.BulkInsert(ctx, records)
	case model.SignalLogs:
		records := make([]logs.LogRecord, 0, totalAccepted(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[[]model.LogRecordV1](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, model.LogsFromWire(payload)...)
		}
		return r.persistence.Logs.BulkInsert(ctx, records)
	case model.SignalMetrics:
		records := make([]metrics.WriteBatch, 0, len(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[model.MetricBatchV1](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, model.MetricsFromWire(payload))
		}
		return r.persistence.Metrics.BulkInsert(ctx, records)
	default:
		return fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (r *Runner) writeDLQ(ctx context.Context, spec signalSpec, msg kafkago.Message, cause error) error {
	payload, err := json.Marshal(model.DeadLetterEvent{
		SchemaVersion: model.SchemaVersion,
		EventID:       uuid.New(),
		Signal:        spec.signal,
		Topic:         msg.Topic,
		Partition:     msg.Partition,
		Offset:        msg.Offset,
		Reason:        cause.Error(),
		Payload:       msg.Value,
		FailedAt:      time.Now().UTC(),
	})
	if err != nil {
		return fmt.Errorf("marshal dead-letter event: %w", err)
	}

	if err := r.dlqWriter.WriteMessages(ctx, kafkago.Message{
		Topic: spec.dlq,
		Key:   msg.Key,
		Value: payload,
		Time:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("write telemetry message to dlq topic %s: %w", spec.dlq, err)
	}

	r.log.Warn("telemetry message moved to dlq", "signal", spec.signal, "dlq_topic", spec.dlq, "source_topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", cause)
	return nil
}

func totalAccepted(events []model.Event) int {
	total := 0
	for _, event := range events {
		total += event.Metadata.AcceptedRecords
	}
	return total
}

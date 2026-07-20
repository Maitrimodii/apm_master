package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	ingestkafka "github.com/Maitrimodii/apm_master/internal/ingest/kafka"
	"github.com/Maitrimodii/apm_master/internal/ingest/model"
	"github.com/Maitrimodii/apm_master/internal/ingest/retry"
	"github.com/Maitrimodii/apm_master/internal/ingest/serializer"
	"github.com/Maitrimodii/apm_master/internal/log"
	"github.com/Maitrimodii/apm_master/internal/metric"
	"github.com/Maitrimodii/apm_master/internal/trace"
	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
)

type SpanWriter interface {
	BulkInsert(context.Context, []trace.Span) error
}

type LogWriter interface {
	BulkInsert(context.Context, []log.LogRecord) error
}

type MetricWriter interface {
	BulkInsert(context.Context, []metric.WriteBatch) error
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

	codec, err := ingestkafka.CompressionCodec(cfg.Compression)
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

func (runner *Runner) Start(ctx context.Context) {
	ctx, runner.cancel = context.WithCancel(ctx)
	runner.done = make(chan struct{})

	var wg sync.WaitGroup
	for _, spec := range runner.signalSpecs() {
		for workerID := 0; workerID < runner.cfg.ConsumerWorkers; workerID++ {
			wg.Add(1)
			go func(spec signalSpec, workerID int) {
				defer wg.Done()
				runner.consumeSignal(ctx, spec, workerID)
			}(spec, workerID)
		}
	}

	go func() {
		wg.Wait()
		close(runner.done)
	}()
}

func (runner *Runner) Shutdown(ctx context.Context) error {
	if runner.cancel != nil {
		runner.cancel()
	}

	select {
	case <-runner.done:
	case <-ctx.Done():
		return fmt.Errorf("shutdown kafka consumers: %w", ctx.Err())
	}

	if err := runner.dlqWriter.Close(); err != nil {
		return fmt.Errorf("close kafka dlq writer: %w", err)
	}
	return nil
}

func (runner *Runner) Done() <-chan struct{} {
	return runner.done
}

type signalSpec struct {
	signal model.Signal
	topic  string
	dlq    string
}

func (runner *Runner) signalSpecs() []signalSpec {
	return []signalSpec{
		{signal: model.SignalTraces, topic: runner.cfg.Topics.Traces, dlq: runner.cfg.Topics.TracesDLQ},
		{signal: model.SignalLogs, topic: runner.cfg.Topics.Logs, dlq: runner.cfg.Topics.LogsDLQ},
		{signal: model.SignalMetrics, topic: runner.cfg.Topics.Metrics, dlq: runner.cfg.Topics.MetricsDLQ},
	}
}

func (runner *Runner) consumeSignal(ctx context.Context, spec signalSpec, workerID int) {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers:               runner.cfg.Brokers,
		GroupID:               runner.cfg.GroupID,
		Topic:                 spec.topic,
		MinBytes:              1,
		MaxBytes:              runner.cfg.ConsumerReadMaxBytes,
		MaxWait:               runner.cfg.ConsumerFlushInterval,
		CommitInterval:        0,
		WatchPartitionChanges: true,
		StartOffset:           kafkago.FirstOffset,
		MaxAttempts:           runner.cfg.ConsumerMaxAttempts,
		ReadBackoffMin:        runner.cfg.ConsumerRetryInitial,
		ReadBackoffMax:        runner.cfg.ConsumerRetryMax,
		ErrorLogger: kafkago.LoggerFunc(func(format string, args ...any) {
			runner.log.Error("kafka consumer error", "signal", spec.signal, "worker_id", workerID, "message", fmt.Sprintf(format, args...))
		}),
	})
	defer reader.Close()

	runner.log.Info("kafka telemetry consumer started", "signal", spec.signal, "topic", spec.topic, "worker_id", workerID)

	batch := newMessageBatch(runner.cfg.ConsumerBatchRecords, runner.cfg.ConsumerBatchBytes)
	ticker := time.NewTicker(runner.cfg.ConsumerFlushInterval)
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
			_ = runner.flushBatch(context.WithoutCancel(ctx), reader, spec, batch)
			runner.log.Info("kafka telemetry consumer stopped", "signal", spec.signal, "worker_id", workerID)
			return
		case <-ticker.C:
			if err := runner.flushBatch(ctx, reader, spec, batch); err != nil {
				runner.log.Error("periodic flush batch failed", "signal", spec.signal, "worker_id", workerID, "error", err)
				time.Sleep(1 * time.Second)
			}
		case res, ok := <-msgChan:
			if !ok {
				return
			}
			if res.err != nil {
				runner.log.Error("fetch kafka telemetry message failed", "signal", spec.signal, "worker_id", workerID, "error", res.err)
				continue
			}

			batch.add(res.msg)

			if batch.full() {
				if err := runner.flushBatch(ctx, reader, spec, batch); err != nil {
					runner.log.Error("full flush batch failed", "signal", spec.signal, "worker_id", workerID, "error", err)
					time.Sleep(1 * time.Second)
				}
			}
		}
	}
}

func (runner *Runner) flushBatch(ctx context.Context, reader *kafkago.Reader, spec signalSpec, batch *messageBatch) error {
	if batch.len() == 0 {
		return nil
	}

	messages := batch.getMessages()
	start := time.Now()
	if err := runner.persistBatch(ctx, spec, messages); err != nil {
		return fmt.Errorf("persist batch: %w", err)
	}

	if err := reader.CommitMessages(ctx, messages...); err != nil {
		return fmt.Errorf("commit messages: %w", err)
	}

	batch.reset()
	runner.log.Info("telemetry batch persisted", "signal", spec.signal, "messages", len(messages), "duration_ms", time.Since(start).Milliseconds())
	return nil
}

func (runner *Runner) persistBatch(ctx context.Context, spec signalSpec, messages []kafkago.Message) error {
	validEvents := make([]model.Event, 0, len(messages))
	for _, msg := range messages {
		event, err := runner.serializer.Unmarshal(msg.Value)
		if err != nil {
			if dlqErr := runner.writeDLQ(ctx, spec, msg, err); dlqErr != nil {
				return fmt.Errorf("write serialization failure to dlq: %w", dlqErr)
			}
			continue
		}
		if event.Signal != spec.signal {
			if dlqErr := runner.writeDLQ(ctx, spec, msg, fmt.Errorf("event signal %q does not match topic signal %q", event.Signal, spec.signal)); dlqErr != nil {
				return fmt.Errorf("write signal mismatch to dlq: %w", dlqErr)
			}
			continue
		}

		// Validate payload format before bulk insert to catch serialization errors
		if err := validatePayload(event.Signal, event.Payload); err != nil {
			if dlqErr := runner.writeDLQ(ctx, spec, msg, fmt.Errorf("invalid payload format: %w", err)); dlqErr != nil {
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
		return runner.persistEvents(ctx, spec.signal, validEvents)
	}

	policy := retry.Policy{
		MaxAttempts: runner.cfg.ConsumerMaxAttempts,
		Initial:     runner.cfg.ConsumerRetryInitial,
		Max:         runner.cfg.ConsumerRetryMax,
	}
	if err := retry.Do(ctx, policy, persist); err != nil {
		return fmt.Errorf("db bulk insert failed: %w", err)
	}

	return nil
}

func validatePayload(signal model.Signal, payload []byte) error {
	switch signal {
	case model.SignalTraces:
		_, err := serializer.UnmarshalPayload[[]trace.Span](payload)
		return err
	case model.SignalLogs:
		_, err := serializer.UnmarshalPayload[[]log.LogRecord](payload)
		return err
	case model.SignalMetrics:
		_, err := serializer.UnmarshalPayload[metric.WriteBatch](payload)
		return err
	default:
		return fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (runner *Runner) persistEvents(ctx context.Context, signal model.Signal, events []model.Event) error {
	switch signal {
	case model.SignalTraces:
		records := make([]trace.Span, 0, totalAccepted(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[[]trace.Span](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, payload...)
		}
		return runner.persistence.Spans.BulkInsert(ctx, records)
	case model.SignalLogs:
		records := make([]log.LogRecord, 0, totalAccepted(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[[]log.LogRecord](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, payload...)
		}
		return runner.persistence.Logs.BulkInsert(ctx, records)
	case model.SignalMetrics:
		records := make([]metric.WriteBatch, 0, len(events))
		for _, event := range events {
			payload, err := serializer.UnmarshalPayload[metric.WriteBatch](event.Payload)
			if err != nil {
				return err
			}
			records = append(records, payload)
		}
		return runner.persistence.Metrics.BulkInsert(ctx, records)
	default:
		return fmt.Errorf("unknown telemetry signal %q", signal)
	}
}

func (runner *Runner) writeDLQ(ctx context.Context, spec signalSpec, msg kafkago.Message, cause error) error {
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

	if err := runner.dlqWriter.WriteMessages(ctx, kafkago.Message{
		Topic: spec.dlq,
		Key:   msg.Key,
		Value: payload,
		Time:  time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("write telemetry message to dlq topic %s: %w", spec.dlq, err)
	}

	runner.log.Warn("telemetry message moved to dlq", "signal", spec.signal, "dlq_topic", spec.dlq, "source_topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset, "error", cause)
	return nil
}

func totalAccepted(events []model.Event) int {
	total := 0
	for _, event := range events {
		total += event.Metadata.AcceptedRecords
	}
	return total
}

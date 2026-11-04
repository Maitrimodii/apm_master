package investigation

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"
)

// Outbox polling defaults and back-off bounds.
const (
	defaultOutboxPollInterval = 2 * time.Second
	defaultOutboxBatchSize    = 50
	outboxInitialBackoff      = 250 * time.Millisecond
	outboxMaxBackoff          = 30 * time.Second
	outboxDrainTimeout        = 10 * time.Second
)

// KafkaWriter is the part of kafka.Writer the outbox publisher uses.
type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

// OutboxConfig configures the outbox publisher.
type OutboxConfig struct {
	Topic        string
	PollInterval time.Duration
	BatchSize    int
}

// OutboxPublisher moves investigation job events from the outbox table to
// Kafka. Rows are marked published only after Kafka acknowledges them, so a
// crash can re-send a job but never lose one; the AI worker deduplicates.
type OutboxPublisher struct {
	store   *Store
	writer  KafkaWriter
	cfg     OutboxConfig
	logger  *slog.Logger
	stopCh  chan struct{}
	doneCh  chan struct{}
	stopped bool
	mu      sync.Mutex
}

func NewOutboxPublisher(store *Store, writer KafkaWriter, cfg OutboxConfig, logger *slog.Logger) *OutboxPublisher {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultOutboxPollInterval
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = defaultOutboxBatchSize
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &OutboxPublisher{
		store:  store,
		writer: writer,
		cfg:    cfg,
		logger: logger,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// Run polls the outbox until ctx is cancelled or Stop is called, then drains
// what is left.
func (p *OutboxPublisher) Run(ctx context.Context) {
	defer close(p.doneCh)

	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	backoff := outboxInitialBackoff
	p.logger.Info("investigation outbox publisher started", "poll_interval", p.cfg.PollInterval, "topic", p.cfg.Topic)

	for {
		select {
		case <-ctx.Done():
			p.drain()
			return
		case <-p.stopCh:
			p.drain()
			return
		case <-ticker.C:
			published, err := p.publishBatch(ctx)
			if err == nil {
				backoff = outboxInitialBackoff
				if published > 0 {
					p.logger.Debug("published outbox batch", "count", published)
				}
				continue
			}
			p.logger.Error("outbox publish failed, backing off", "error", err, "backoff", backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return
			case <-p.stopCh:
				return
			}
			backoff = min(backoff*2, outboxMaxBackoff)
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) (int, error) {
	events, err := p.store.ListUnpublishedOutbox(ctx, p.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("list unpublished outbox: %w", err)
	}
	for i, event := range events {
		msg := kafkago.Message{
			Topic:   p.cfg.Topic,
			Key:     []byte(event.InvestigationID.String()),
			Value:   event.Payload,
			Headers: []kafkago.Header{{Key: KafkaHeaderCorrelationID, Value: []byte(event.CorrelationID.String())}},
			Time:    event.CreatedAt,
		}
		if err := p.writer.WriteMessages(ctx, msg); err != nil {
			return i, fmt.Errorf("publish outbox event %s: %w", event.ID, err)
		}
		if err := p.store.MarkOutboxPublished(ctx, event.ID); err != nil {
			return i, fmt.Errorf("mark outbox event %s published: %w", event.ID, err)
		}
	}
	return len(events), nil
}

func (p *OutboxPublisher) drain() {
	ctx, cancel := context.WithTimeout(context.Background(), outboxDrainTimeout)
	defer cancel()
	for {
		count, err := p.publishBatch(ctx)
		if err != nil || count == 0 {
			return
		}
	}
}

// Stop starts a graceful shutdown and waits for the drain to finish.
func (p *OutboxPublisher) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	close(p.stopCh)
	p.mu.Unlock()
	<-p.doneCh
}

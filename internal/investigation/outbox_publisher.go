package investigation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	kafkago "github.com/segmentio/kafka-go"
)

type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafkago.Message) error
	Close() error
}

type OutboxPublisher struct {
	pool    *pgxpool.Pool
	store   *Store
	writer  KafkaWriter
	cfg     *config.Config
	logger  *slog.Logger
	stopCh  chan struct{}
	doneCh  chan struct{}
	stopped bool
	mu      sync.Mutex
}

func NewOutboxPublisher(
	pool *pgxpool.Pool,
	writer KafkaWriter,
	cfg *config.Config,
	logger *slog.Logger,
) *OutboxPublisher {
	return &OutboxPublisher{
		pool:   pool,
		store:  NewStore(pool),
		writer: writer,
		cfg:    cfg,
		logger: logger,
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
}

// EnsureAITopics creates Kafka topics for AI investigations if they do not exist.
func EnsureAITopics(ctx context.Context, cfg *config.Config) error {
	brokers := strings.Split(cfg.KafkaBrokers, ",")
	if len(brokers) == 0 || brokers[0] == "" {
		return fmt.Errorf("no kafka brokers configured")
	}

	client := &kafkago.Client{
		Addr:    kafkago.TCP(brokers[0]),
		Timeout: 10 * time.Second,
	}

	retention := fmt.Sprintf("%d", cfg.KafkaTopicRetention.Milliseconds())
	topicConfigs := []kafkago.TopicConfig{
		{
			Topic:             cfg.KafkaAIInvestigationTopic,
			NumPartitions:     cfg.KafkaTopicPartitions,
			ReplicationFactor: cfg.KafkaTopicReplicationFactor,
			ConfigEntries: []kafkago.ConfigEntry{
				{ConfigName: "retention.ms", ConfigValue: retention},
			},
		},
		{
			Topic:             cfg.KafkaAIRetryTopic,
			NumPartitions:     cfg.KafkaTopicPartitions,
			ReplicationFactor: cfg.KafkaTopicReplicationFactor,
			ConfigEntries: []kafkago.ConfigEntry{
				{ConfigName: "retention.ms", ConfigValue: retention},
			},
		},
		{
			Topic:             cfg.KafkaAIDLQTopic,
			NumPartitions:     cfg.KafkaTopicPartitions,
			ReplicationFactor: cfg.KafkaTopicReplicationFactor,
			ConfigEntries: []kafkago.ConfigEntry{
				{ConfigName: "retention.ms", ConfigValue: retention},
			},
		},
	}

	resp, err := client.CreateTopics(ctx, &kafkago.CreateTopicsRequest{
		Topics: topicConfigs,
	})
	if err != nil {
		return fmt.Errorf("create ai kafka topics: %w", err)
	}

	for topic, topicErr := range resp.Errors {
		if topicErr == nil || errors.Is(topicErr, kafkago.TopicAlreadyExists) {
			continue
		}
		return fmt.Errorf("create ai kafka topic %s: %w", topic, topicErr)
	}

	return nil
}

// Run starts the background publisher loop.
func (p *OutboxPublisher) Run(ctx context.Context) {
	defer close(p.doneCh)

	// Ensure topics on startup
	if err := EnsureAITopics(ctx, p.cfg); err != nil {
		p.logger.Warn("failed to ensure AI kafka topics on startup (will continue running)", "error", err)
	} else {
		p.logger.Info("AI investigation kafka topics verified/created")
	}

	interval := p.cfg.InvestigationOutboxPollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	backoff := 250 * time.Millisecond
	const maxBackoff = 30 * time.Second

	p.logger.Info("investigation outbox publisher started", "poll_interval", interval)

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("investigation outbox publisher context cancelled, draining in-flight...")
			p.drain(context.Background())
			return
		case <-p.stopCh:
			p.logger.Info("investigation outbox publisher stopping, draining in-flight...")
			p.drain(context.Background())
			return
		case <-ticker.C:
			published, err := p.publishBatch(ctx)
			if err != nil {
				p.logger.Error("outbox publish batch error, backing off", "error", err, "backoff", backoff)
				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return
				case <-p.stopCh:
					return
				}
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			} else {
				backoff = 250 * time.Millisecond
				if published > 0 {
					p.logger.Debug("published outbox batch", "count", published)
				}
			}
		}
	}
}

func (p *OutboxPublisher) publishBatch(ctx context.Context) (int, error) {
	batchSize := p.cfg.InvestigationOutboxBatchSize
	if batchSize <= 0 {
		batchSize = 50
	}

	events, err := p.store.ListUnpublishedOutbox(ctx, batchSize)
	if err != nil {
		return 0, fmt.Errorf("list unpublished outbox: %w", err)
	}
	if len(events) == 0 {
		return 0, nil
	}

	count := 0
	for _, event := range events {
		msg := kafkago.Message{
			Topic: p.cfg.KafkaAIInvestigationTopic,
			Key:   []byte(event.InvestigationID.String()),
			Value: event.Payload,
			Headers: []kafkago.Header{
				{
					Key:   "correlation_id",
					Value: []byte(event.CorrelationID.String()),
				},
			},
			Time: event.CreatedAt,
		}

		if err := p.writer.WriteMessages(ctx, msg); err != nil {
			return count, fmt.Errorf("publish outbox event %s to kafka: %w", event.ID, err)
		}

		if err := p.store.MarkOutboxPublished(ctx, event.ID); err != nil {
			p.logger.Error("failed to mark outbox published", "event_id", event.ID, "error", err)
			return count, fmt.Errorf("mark outbox published %s: %w", event.ID, err)
		}
		count++
	}

	return count, nil
}

func (p *OutboxPublisher) drain(ctx context.Context) {
	drainCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for {
		count, err := p.publishBatch(drainCtx)
		if err != nil || count == 0 {
			break
		}
	}
}

// Stop initiates graceful shutdown and waits for drain.
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

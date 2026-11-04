package ingest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/ingest/consumer"
	ingestkafka "github.com/Maitrimodii/apm_master/internal/ingest/kafka"
	"github.com/Maitrimodii/apm_master/internal/ingest/producer"
	"github.com/Maitrimodii/apm_master/internal/ingest/services"
	"github.com/Maitrimodii/apm_master/internal/ingest/writer"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
)

type Module struct {
	handler  *Handler
	producer *producer.KafkaProducer
	consumer *consumer.Runner
}

func NewModule(
	ctx context.Context,
	database *pgxpool.Pool,
	cfg *config.Config,
	apiKeys middleware.APIKeyValidator,
	log *slog.Logger,
) (*Module, error) {

	traceWriter := writer.NewSpanWriter(database)
	logWriter := writer.NewLogWriter(database)
	metricWriter := writer.NewMetricWriter(database)
	kafkaCfg := ingestkafka.FromAppConfig(cfg.Kafka)

	if err := ingestkafka.EnsureTopics(ctx, kafkaCfg); err != nil {
		return nil, fmt.Errorf("ensure kafka ingest topics: %w", err)
	}

	kafkaProducer, err := producer.NewKafkaProducer(kafkaCfg, log)
	if err != nil {
		return nil, fmt.Errorf("create kafka ingest producer: %w", err)
	}

	kafkaConsumer, err := consumer.NewRunner(kafkaCfg, consumer.Persistence{
		Spans:   traceWriter,
		Logs:    logWriter,
		Metrics: metricWriter,
	}, log)
	if err != nil {
		return nil, fmt.Errorf("create kafka ingest consumer: %w", err)
	}
	kafkaConsumer.Start(ctx)

	service := services.NewService(
		kafkaProducer,
		log,
	)

	return &Module{
		handler: NewHandler(
			service,
			apiKeys,
		),
		producer: kafkaProducer,
		consumer: kafkaConsumer,
	}, nil
}

func (m *Module) RegisterRoutes(routerGroup *gin.RouterGroup) {
	m.handler.RegisterRoutes(routerGroup)
}

func (m *Module) Shutdown(ctx context.Context) error {

	if err := m.producer.Close(); err != nil {
		return err
	}

	if err := m.consumer.Shutdown(ctx); err != nil {
		return err
	}

	return nil
}

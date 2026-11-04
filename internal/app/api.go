package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	kafkago "github.com/segmentio/kafka-go"

	"github.com/Maitrimodii/apm_master/internal/alert"
	"github.com/Maitrimodii/apm_master/internal/auth"
	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/ingest"
	"github.com/Maitrimodii/apm_master/internal/investigation"
	"github.com/Maitrimodii/apm_master/internal/investigation/evidence"
	"github.com/Maitrimodii/apm_master/internal/logs"
	"github.com/Maitrimodii/apm_master/internal/metrics"
	"github.com/Maitrimodii/apm_master/internal/platform/kafkax"
	"github.com/Maitrimodii/apm_master/internal/platform/middleware"
	"github.com/Maitrimodii/apm_master/internal/platform/module"
	"github.com/Maitrimodii/apm_master/internal/platform/redisx"
	"github.com/Maitrimodii/apm_master/internal/scheduler"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/Maitrimodii/apm_master/internal/traces"
)

// shutdownTimeout is each component's budget to stop, so a slow HTTP drain
// doesn't eat into ingest's or the notification worker's time.
const shutdownTimeout = 10 * time.Second

// RunAPI runs the HTTP API, ingest consumers, the notification worker, the
// investigation outbox and, in APP_MODE=all, the scheduler. It returns when
// ctx is cancelled and everything has shut down.
func RunAPI(ctx context.Context, cfg *config.Config, log *slog.Logger) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
	if !cfg.App.IsDevelopment() {
		gin.SetMode(gin.ReleaseMode)
	}

	pool, err := connectPostgres(ctx, cfg, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	redisCtx, cancel := context.WithTimeout(ctx, cfg.Redis.Timeout)
	rdb, err := redisx.Connect(redisCtx, cfg.Redis.URL, cfg.Redis.Timeout)
	cancel()
	if err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = rdb.Close() }()
	log.Info("redis connected")

	// Background work outlives the request context and stops on appCancel.
	appCtx, appCancel := context.WithCancel(context.WithoutCancel(ctx))
	defer appCancel()

	notifications := newNotificationService(pool, cfg.SMTP, log)
	notifications.Start(appCtx)

	limiter := middleware.NewRateLimiter(rdb, cfg.HTTP.RateLimitBurst, cfg.HTTP.RateLimitWindow, log)
	authModule := auth.NewModule(pool, rdb, cfg, notifications, log).WithRateLimit(limiter.ByOrg())
	deps := module.Deps{
		DB:                pool,
		PermissionChecker: authModule.PermissionChecker,
		TokenParser:       authModule.TokenParser,
		Logger:            log,
		RateLimit:         limiter.ByOrg(),
	}

	ingestModule, err := ingest.NewModule(appCtx, pool, cfg, authModule.APIKeyValidator, log)
	if err != nil {
		return fmt.Errorf("create ingest module: %w", err)
	}

	incidents := alert.NewStore(pool)
	srv := server.NewServer(server.ServerParams{
		HTTP: cfg.HTTP,
		Middlewares: []gin.HandlerFunc{
			middleware.CORS(cfg.App.Origins()),
			middleware.RequestID(),
			middleware.Logger(log),
			middleware.Recovery(log),
			middleware.BodyLimit(cfg.HTTP.MaxBodyBytes),
			limiter.ByClientIP(),
		},
		APIHandler: server.NewAPIHandler(
			authModule,
			ingestModule,
			traces.NewModule(deps),
			logs.NewModule(deps),
			metrics.NewModule(deps),
			alert.NewModule(deps, notifications),
			investigation.NewModule(deps, incidents),
		),
		InternalHandler: server.NewInternalHandler(
			investigation.NewInternalModule(deps, incidents, evidence.Readers{
				Metrics: metrics.NewStore(pool),
				Traces:  traces.NewStore(pool),
				Logs:    logs.NewStore(pool),
			}, cfg.AI.ServiceToken),
		),
		HealthChecker: server.NewHealthChecker(pool, rdb),
	})

	outbox, outboxWriter, err := newOutbox(appCtx, cfg, investigation.NewStore(pool), log)
	if err != nil {
		return err
	}
	go outbox.Run(appCtx)

	var jobs *scheduler.Scheduler
	if cfg.App.Mode.RunsScheduler() {
		if jobs, err = newScheduler(pool, cfg, notifications, log); err != nil {
			return err
		}
		jobs.Start(appCtx)
		log.Info("scheduler started", "app_mode", cfg.App.Mode)
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("server started", "port", cfg.HTTP.Port)
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
		close(serveErr)
	}()

	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if err != nil {
			log.Error("server failed", slog.Any("error", err))
		}
	}
	log.Info("shutting down")

	var errs []error
	stop := func(name string, fn func(context.Context) error) {
		c, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := fn(c); err != nil {
			log.Error("shutdown failed", "component", name, slog.Any("error", err))
			errs = append(errs, fmt.Errorf("%s shutdown: %w", name, err))
		}
	}
	stop("http server", srv.Shutdown)
	stop("ingest", ingestModule.Shutdown)
	appCancel()
	if jobs != nil {
		jobs.Stop()
	}
	outbox.Stop()
	if err := outboxWriter.Close(); err != nil {
		log.Error("close investigation kafka writer failed", slog.Any("error", err))
	}
	stop("notification worker", func(c context.Context) error {
		select {
		case <-notifications.Done():
			return nil
		case <-c.Done():
			return c.Err()
		}
	})
	log.Info("server exited")
	return errors.Join(errs...)
}

// newOutbox creates the AI investigation topics if missing and the publisher
// that moves queued jobs to Kafka.
func newOutbox(ctx context.Context, cfg *config.Config, store *investigation.Store, log *slog.Logger) (*investigation.OutboxPublisher, *kafkago.Writer, error) {
	specs := make([]kafkax.TopicSpec, 0, 3)
	for _, topic := range []string{cfg.AI.InvestigationTopic, cfg.AI.RetryTopic, cfg.AI.DLQTopic} {
		specs = append(specs, kafkax.TopicSpec{
			Name:              topic,
			Partitions:        cfg.Kafka.TopicPartitions,
			ReplicationFactor: cfg.Kafka.TopicReplicationFactor,
			Retention:         cfg.Kafka.TopicRetention,
		})
	}
	if err := kafkax.EnsureTopics(ctx, cfg.Kafka.Brokers, specs...); err != nil {
		// Startup continues: the publisher retries until Kafka is reachable.
		log.Warn("could not ensure AI investigation topics", slog.Any("error", err))
	}

	writer, err := kafkax.NewWriter(kafkax.WriterConfig{
		Brokers:      cfg.Kafka.Brokers,
		BatchSize:    cfg.AI.OutboxBatchSize,
		BatchTimeout: 10 * time.Millisecond,
		WriteTimeout: cfg.Kafka.ProducerWriteTimeout,
		MaxAttempts:  cfg.Kafka.ProducerMaxAttempts,
		RequiredAcks: kafkago.RequireAll,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("create investigation kafka writer: %w", err)
	}
	publisher := investigation.NewOutboxPublisher(store, writer, investigation.OutboxConfig{
		Topic:        cfg.AI.InvestigationTopic,
		PollInterval: cfg.AI.OutboxPollInterval,
		BatchSize:    cfg.AI.OutboxBatchSize,
	}, log)
	return publisher, writer, nil
}

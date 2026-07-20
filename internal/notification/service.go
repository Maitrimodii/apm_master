package notification

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Maitrimodii/apm_master/internal/config"
	"github.com/Maitrimodii/apm_master/internal/server"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	store  *Store
	worker *NotificationWorker
	log    *slog.Logger
}

func NewService(pool *pgxpool.Pool, cfg *config.Config, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}

	store := NewStore(pool)
	providers := []NotificationProvider{
		NewSMTPProvider(cfg),
	}

	return &Service{
		store:  store,
		worker: NewNotificationWorker(store, providers, log, defaultWorkerConcurrency),
		log:    log,
	}
}

func (service *Service) Start(ctx context.Context) {
	service.worker.Start(ctx)
}

func (service *Service) Done() <-chan struct{} {
	return service.worker.Done()
}

func (service *Service) EnqueueEmail(ctx context.Context, task NotificationTask) error {
	task.Provider = "email"
	if task.EventType == "" || task.Recipient == "" || task.Subject == "" {
		return fmt.Errorf("notification task requires event_type, recipient, and subject")
	}

	if err := service.store.CreateJob(ctx, task, 5); err != nil {
		service.log.Error("failed to create notification job", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
		return err
	}

	return nil
}

func (service *Service) EnqueueHighPriorityEmail(ctx context.Context, task NotificationTask) error {
	task.Provider = "email"
	if task.EventType == "" || task.Recipient == "" || task.Subject == "" {
		return fmt.Errorf("notification task requires event_type, recipient, and subject")
	}

	if task.Priority == 0 {
		task.Priority = 10
	}

	if task.ReferenceID != "" {
		exists, err := service.store.ExistsByReference(ctx, task.ReferenceID, task.Recipient, task.EventType)
		if err != nil {
			return server.Internal(err)
		}
		if exists {
			service.log.Info("duplicate alert notification skipped", "event_type", task.EventType, "recipient", task.Recipient, "reference_id", task.ReferenceID)
			return nil
		}
	}

	if err := service.store.CreateJob(ctx, task, 5); err != nil {
		service.log.Error("failed to create high-priority notification job", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
		return err
	}

	return nil
}

func (service *Service) RecordEvent(ctx context.Context, event Event) (*Event, error) {
	if event.Status == "" {
		event.Status = StatusPending
	}
	if event.Channel == "" {
		event.Channel = ChannelInApp
	}

	created, err := service.store.CreateEvent(ctx, event)
	if err != nil {
		return nil, server.Internal(err)
	}

	service.log.Info("notification event recorded", "event_id", created.ID, "event_type", created.EventType, "channel", created.Channel)
	return created, nil
}

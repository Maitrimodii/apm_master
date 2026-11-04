package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var errTaskIncomplete = errors.New("notification task requires event_type, recipient, and subject")

// Service enqueues notifications and records in-app events. Delivery is done
// asynchronously by the Worker.
type Service struct {
	store  *Store
	worker *Worker
	log    *slog.Logger
}

func NewService(store *Store, worker *Worker, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{store: store, worker: worker, log: log}
}

func (s *Service) Start(ctx context.Context) {
	s.worker.Start(ctx)
}

func (s *Service) Done() <-chan struct{} {
	return s.worker.Done()
}

// EnqueueEmail queues a transactional email (invites, password resets).
func (s *Service) EnqueueEmail(ctx context.Context, task Task) error {
	return s.enqueueEmail(ctx, task, PriorityNormal)
}

// EnqueueHighPriorityEmail queues an alert email ahead of transactional mail.
// Tasks with a ReferenceID are enqueued at most once per recipient and event.
func (s *Service) EnqueueHighPriorityEmail(ctx context.Context, task Task) error {
	if task.ReferenceID != "" {
		exists, err := s.store.ExistsByReference(ctx, task.ReferenceID, task.Recipient, task.EventType)
		if err != nil {
			return fmt.Errorf("check duplicate notification: %w", err)
		}
		if exists {
			s.log.InfoContext(ctx, "duplicate alert notification skipped", "event_type", task.EventType, "reference_id", task.ReferenceID)
			return nil
		}
	}
	return s.enqueueEmail(ctx, task, PriorityHigh)
}

func (s *Service) enqueueEmail(ctx context.Context, task Task, defaultPriority int) error {
	if task.EventType == "" || task.Recipient == "" || task.Subject == "" {
		return errTaskIncomplete
	}
	task.Provider = ChannelEmail
	if task.Priority == 0 {
		task.Priority = defaultPriority
	}
	if err := s.store.CreateJob(ctx, task, DefaultMaxAttempts); err != nil {
		s.log.ErrorContext(ctx, "failed to create notification job", "event_type", task.EventType, "error", err)
		return fmt.Errorf("create notification job: %w", err)
	}
	return nil
}

// RecordEvent stores an in-app notification event.
func (s *Service) RecordEvent(ctx context.Context, event Event) (*Event, error) {
	if event.Status == "" {
		event.Status = StatusPending
	}
	if event.Channel == "" {
		event.Channel = ChannelInApp
	}
	created, err := s.store.CreateEvent(ctx, event)
	if err != nil {
		return nil, fmt.Errorf("record notification event: %w", err)
	}
	s.log.InfoContext(ctx, "notification event recorded", "event_id", created.ID, "event_type", created.EventType, "channel", created.Channel)
	return created, nil
}

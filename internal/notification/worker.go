package notification

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	defaultWorkerConcurrency = 5
	workerPollInterval       = 500 * time.Millisecond
	jobProcessingTimeout     = 30 * time.Second
	initialRetryBackoff      = 2 * time.Second
	maxRetryBackoff          = 5 * time.Minute
)

type NotificationWorker struct {
	store       *Store
	providers   map[string]NotificationProvider
	done        chan struct{}
	log         *slog.Logger
	concurrency int
	cancel      context.CancelFunc
}

func NewNotificationWorker(store *Store, providers []NotificationProvider, log *slog.Logger, concurrency int) *NotificationWorker {
	if log == nil {
		log = slog.Default()
	}
	if concurrency <= 0 {
		concurrency = defaultWorkerConcurrency
	}

	providerMap := make(map[string]NotificationProvider, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		providerMap[provider.Name()] = provider
	}

	return &NotificationWorker{
		store:       store,
		providers:   providerMap,
		done:        make(chan struct{}),
		log:         log,
		concurrency: concurrency,
	}
}

func (worker *NotificationWorker) Start(ctx context.Context) {
	ctx, worker.cancel = context.WithCancel(ctx)
	worker.done = make(chan struct{})

	var wg sync.WaitGroup
	for index := 0; index < worker.concurrency; index++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			worker.startWorker(ctx, workerID)
		}(index)
	}

	go func() {
		wg.Wait()
		close(worker.done)
	}()
}

func (worker *NotificationWorker) Done() <-chan struct{} {
	return worker.done
}

func (worker *NotificationWorker) startWorker(ctx context.Context, identifier int) {
	worker.log.Debug("notification worker started", "worker_id", identifier)
	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()

	// Poll immediately on startup
	processed, err := worker.processNextJob(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		worker.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
	}
	if processed {
		worker.processPendingJobs(ctx, identifier)
	}

	for {
		select {
		case <-ctx.Done():
			worker.log.Debug("notification worker stopping", "worker_id", identifier)
			return
		case <-ticker.C:
			processed, err := worker.processNextJob(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				worker.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
			}
			if processed {
				worker.processPendingJobs(ctx, identifier)
			}
		}
	}
}

func (worker *NotificationWorker) processPendingJobs(ctx context.Context, identifier int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		more, err := worker.processNextJob(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			worker.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
		}
		if !more {
			break
		}
	}
}

func (worker *NotificationWorker) processNextJob(ctx context.Context) (bool, error) {
	job, err := worker.store.FetchNextJob(ctx)
	if err != nil {
		return false, err
	}
	if job == nil {
		return false, nil
	}

	// Allow in-flight provider calls and state updates to finish during graceful
	// shutdown while still preserving request-scoped values from the worker context.
	processCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jobProcessingTimeout)
	defer cancel()

	worker.log.InfoContext(processCtx, "processing notification job", "job_id", job.ID, "recipient", job.Recipient, "attempt", job.Attempts)

	task := NotificationTask{
		OrgID:       job.OrgID,
		UserID:      job.UserID,
		EventType:   job.EventType,
		Provider:    job.Provider,
		Recipient:   job.Recipient,
		Subject:     job.Subject,
		Body:        job.Body,
		ReferenceID: job.ReferenceID,
		Metadata:    job.Metadata,
	}

	provider, exists := worker.providers[job.Provider]
	if !exists {
		err := fmt.Errorf("notification provider %q not registered", job.Provider)
		worker.log.ErrorContext(processCtx, "provider not registered", "provider", job.Provider)
		worker.handleFailure(processCtx, job, err)
		return true, nil
	}

	msg := Message{
		To:        job.Recipient,
		Subject:   job.Subject,
		Body:      job.Body,
		EventType: job.EventType,
		Meta:      job.Metadata,
	}

	if err := provider.Send(processCtx, msg); err != nil {
		worker.log.ErrorContext(processCtx, "notification send failed", "job_id", job.ID, "error", err)
		worker.handleFailure(processCtx, job, err)
		return true, nil
	}

	// Success! Update state to sent and record result
	if err := worker.store.UpdateJobState(processCtx, job.ID, "sent", "", job.Attempts, nil); err != nil {
		worker.log.ErrorContext(processCtx, "failed to update job state to sent", "job_id", job.ID, "error", err)
	}
	worker.recordResult(processCtx, task, StatusSent, nil)

	return true, nil
}

func (worker *NotificationWorker) handleFailure(ctx context.Context, job *NotificationJob, sendErr error) {
	if job.Attempts < job.MaxAttempts {
		// Calculate exponential backoff: 2s, 4s, 8s, 16s...
		backoff := initialRetryBackoff << max(job.Attempts-1, 0)
		if backoff > maxRetryBackoff {
			backoff = maxRetryBackoff
		}
		runAt := time.Now().UTC().Add(backoff)

		worker.log.InfoContext(ctx, "scheduling retry for job", "job_id", job.ID, "attempts", job.Attempts, "run_at", runAt, "backoff_duration", backoff.String(), "reason", sendErr.Error())
		if err := worker.store.UpdateJobState(ctx, job.ID, "pending", sendErr.Error(), job.Attempts, &runAt); err != nil {
			worker.log.ErrorContext(ctx, "failed to schedule retry", "job_id", job.ID, "error", err)
		}
	} else {
		worker.log.WarnContext(ctx, "job exceeded max attempts, moving to dead_letter", "job_id", job.ID, "attempts", job.Attempts, "reason", sendErr.Error())
		if err := worker.store.UpdateJobState(ctx, job.ID, "dead_letter", sendErr.Error(), job.Attempts, nil); err != nil {
			worker.log.ErrorContext(ctx, "failed to move job to dead_letter", "job_id", job.ID, "error", err)
		}
	}

	task := NotificationTask{
		OrgID:       job.OrgID,
		UserID:      job.UserID,
		EventType:   job.EventType,
		Provider:    job.Provider,
		Recipient:   job.Recipient,
		Subject:     job.Subject,
		Body:        job.Body,
		ReferenceID: job.ReferenceID,
		Metadata:    job.Metadata,
	}
	worker.recordResult(ctx, task, StatusFailed, sendErr)
}

func (worker *NotificationWorker) recordResult(ctx context.Context, task NotificationTask, status Status, sendErr error) {
	metadata := cloneMetadata(task.Metadata)
	if task.ReferenceID != "" {
		metadata["reference_id"] = task.ReferenceID
	}
	metadata["provider"] = task.Provider

	encoded, err := marshalMetadata(metadata)
	if err != nil {
		worker.log.Error("notification metadata marshal failed", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
		return
	}

	var errText *string
	if sendErr != nil {
		text := sendErr.Error()
		errText = &text
	}

	var sentAt *time.Time
	if status == StatusSent {
		now := time.Now().UTC()
		sentAt = &now
	}

	if _, err := worker.store.CreateEvent(ctx, Event{
		OrgID:     task.OrgID,
		UserID:    task.UserID,
		EventType: task.EventType,
		Channel:   providerChannel(task.Provider),
		Recipient: task.Recipient,
		Subject:   task.Subject,
		Body:      task.Body,
		Status:    status,
		Error:     errText,
		Metadata:  encoded,
		SentAt:    sentAt,
	}); err != nil {
		worker.log.Error("notification event write failed", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
	}
}

func providerChannel(provider string) Channel {
	if provider == "email" {
		return ChannelEmail
	}
	return ChannelInApp
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata)+2)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

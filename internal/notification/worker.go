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

type Worker struct {
	store       *Store
	providers   map[Channel]Provider
	done        chan struct{}
	log         *slog.Logger
	concurrency int
	cancel      context.CancelFunc
}

func NewWorker(store *Store, providers []Provider, log *slog.Logger, concurrency int) *Worker {
	if log == nil {
		log = slog.Default()
	}
	if concurrency <= 0 {
		concurrency = defaultWorkerConcurrency
	}

	providerMap := make(map[Channel]Provider, len(providers))
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		providerMap[provider.Channel()] = provider
	}

	return &Worker{
		store:       store,
		providers:   providerMap,
		done:        make(chan struct{}),
		log:         log,
		concurrency: concurrency,
	}
}

func (w *Worker) Start(ctx context.Context) {
	ctx, w.cancel = context.WithCancel(ctx)
	w.done = make(chan struct{})

	var wg sync.WaitGroup
	for index := 0; index < w.concurrency; index++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.startWorker(ctx, workerID)
		}(index)
	}

	go func() {
		wg.Wait()
		close(w.done)
	}()
}

func (w *Worker) Done() <-chan struct{} {
	return w.done
}

func (w *Worker) startWorker(ctx context.Context, identifier int) {
	w.log.Debug("notification worker started", "worker_id", identifier)
	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()

	// Poll immediately on startup
	processed, err := w.processNextJob(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
	}
	if processed {
		w.processPendingJobs(ctx, identifier)
	}

	for {
		select {
		case <-ctx.Done():
			w.log.Debug("notification worker stopping", "worker_id", identifier)
			return
		case <-ticker.C:
			processed, err := w.processNextJob(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				w.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
			}
			if processed {
				w.processPendingJobs(ctx, identifier)
			}
		}
	}
}

func (w *Worker) processPendingJobs(ctx context.Context, identifier int) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		more, err := w.processNextJob(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			w.log.Error("worker failed to process job", "worker_id", identifier, "error", err)
		}
		if !more {
			break
		}
	}
}

func (w *Worker) processNextJob(ctx context.Context) (bool, error) {
	job, err := w.store.FetchNextJob(ctx)
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

	w.log.InfoContext(processCtx, "processing notification job", "job_id", job.ID, "recipient", job.Recipient, "attempt", job.Attempts)

	task := Task{
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

	provider, exists := w.providers[job.Provider]
	if !exists {
		err := fmt.Errorf("notification provider %q not registered", job.Provider)
		w.log.ErrorContext(processCtx, "provider not registered", "provider", job.Provider)
		w.handleFailure(processCtx, job, err)
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
		w.log.ErrorContext(processCtx, "notification send failed", "job_id", job.ID, "error", err)
		w.handleFailure(processCtx, job, err)
		return true, nil
	}

	// Success! Update state to sent and record result
	if err := w.store.UpdateJobState(processCtx, job.ID, JobSent, "", job.Attempts, nil); err != nil {
		w.log.ErrorContext(processCtx, "failed to update job state to sent", "job_id", job.ID, "error", err)
	}
	w.recordResult(processCtx, task, StatusSent, nil)

	return true, nil
}

func (w *Worker) handleFailure(ctx context.Context, job *Job, sendErr error) {
	if job.Attempts < job.MaxAttempts {
		// Calculate exponential backoff: 2s, 4s, 8s, 16s...
		backoff := initialRetryBackoff << max(job.Attempts-1, 0)
		if backoff > maxRetryBackoff {
			backoff = maxRetryBackoff
		}
		runAt := time.Now().UTC().Add(backoff)

		w.log.InfoContext(ctx, "scheduling retry for job", "job_id", job.ID, "attempts", job.Attempts, "run_at", runAt, "backoff_duration", backoff.String(), "reason", sendErr.Error())
		if err := w.store.UpdateJobState(ctx, job.ID, JobPending, sendErr.Error(), job.Attempts, &runAt); err != nil {
			w.log.ErrorContext(ctx, "failed to schedule retry", "job_id", job.ID, "error", err)
		}
	} else {
		w.log.WarnContext(ctx, "job exceeded max attempts, moving to dead_letter", "job_id", job.ID, "attempts", job.Attempts, "reason", sendErr.Error())
		if err := w.store.UpdateJobState(ctx, job.ID, JobDeadLetter, sendErr.Error(), job.Attempts, nil); err != nil {
			w.log.ErrorContext(ctx, "failed to move job to dead_letter", "job_id", job.ID, "error", err)
		}
	}

	task := Task{
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
	w.recordResult(ctx, task, StatusFailed, sendErr)
}

func (w *Worker) recordResult(ctx context.Context, task Task, status Status, sendErr error) {
	metadata := cloneMetadata(task.Metadata)
	if task.ReferenceID != "" {
		metadata["reference_id"] = task.ReferenceID
	}
	metadata["provider"] = string(task.Provider)

	encoded, err := marshalMetadata(metadata)
	if err != nil {
		w.log.Error("notification metadata marshal failed", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
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

	if _, err := w.store.CreateEvent(ctx, Event{
		OrgID:     task.OrgID,
		UserID:    task.UserID,
		EventType: task.EventType,
		Channel:   task.Provider,
		Recipient: task.Recipient,
		Subject:   task.Subject,
		Body:      task.Body,
		Status:    status,
		Error:     errText,
		Metadata:  encoded,
		SentAt:    sentAt,
	}); err != nil {
		w.log.Error("notification event write failed", "event_type", task.EventType, "recipient", task.Recipient, "error", err)
	}
}

func cloneMetadata(metadata map[string]string) map[string]string {
	cloned := make(map[string]string, len(metadata)+2)
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

package retry

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"
)

type Policy struct {
	MaxAttempts int
	Initial     time.Duration
	Max         time.Duration
}

func Do(ctx context.Context, policy Policy, fn func() error) error {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.Initial <= 0 {
		policy.Initial = 100 * time.Millisecond
	}
	if policy.Max <= 0 {
		policy.Max = policy.Initial
	}

	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
		} else {
			return nil
		}

		if attempt == policy.MaxAttempts {
			break
		}

		delay := backoff(policy, attempt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("retry canceled after attempt %d: %w", attempt, ctx.Err())
		case <-timer.C:
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", policy.MaxAttempts, lastErr)
}

func backoff(policy Policy, attempt int) time.Duration {
	delay := policy.Initial << max(attempt-1, 0)
	if delay > policy.Max {
		delay = policy.Max
	}
	jitterLimit := max(delay/4, time.Millisecond)
	return delay + time.Duration(rand.Int64N(int64(jitterLimit)))
}

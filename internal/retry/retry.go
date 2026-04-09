// Package retry provides API call retry logic with exponential backoff
// and optional model fallback.
package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/noknov/mini-claude-code/internal/provider"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

// Config controls retry behavior.
type Config struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	FallbackModel string // model to try when primary fails repeatedly
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxRetries:   3,
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second,
	}
}

// ---------------------------------------------------------------------------
// Retry logic
// ---------------------------------------------------------------------------

// SendStreamWithRetry wraps provider.SendStream with retry + fallback logic.
func SendStreamWithRetry(
	ctx context.Context,
	prov provider.Provider,
	req provider.Request,
	cfg Config,
) (<-chan provider.StreamEvent, <-chan error) {
	eventCh := make(chan provider.StreamEvent, 64)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		var lastErr error
		for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
			if attempt > 0 {
				delay := backoffDelay(attempt, cfg.InitialDelay, cfg.MaxDelay)
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case <-time.After(delay):
				}
			}

			if attempt == cfg.MaxRetries && cfg.FallbackModel != "" {
				prov.SetModel(cfg.FallbackModel)
			}

			innerEventCh, innerErrCh := prov.SendStream(ctx, req)
			success := forwardEvents(innerEventCh, innerErrCh, eventCh, &lastErr)
			if success {
				return
			}

			if !isRetryable(lastErr) {
				break
			}
		}

		if lastErr != nil {
			errCh <- fmt.Errorf("after %d retries: %w", cfg.MaxRetries, lastErr)
		}
	}()

	return eventCh, errCh
}

// forwardEvents relays events from inner channels to outer. Returns true if
// the stream completed successfully.
func forwardEvents(
	innerEventCh <-chan provider.StreamEvent,
	innerErrCh <-chan error,
	outerEventCh chan<- provider.StreamEvent,
	lastErr *error,
) bool {
	for {
		select {
		case evt, ok := <-innerEventCh:
			if !ok {
				return *lastErr == nil
			}
			outerEventCh <- evt
		case err, ok := <-innerErrCh:
			if ok && err != nil {
				*lastErr = err
				return false
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func backoffDelay(attempt int, initial, maxDelay time.Duration) time.Duration {
	base := time.Duration(float64(initial) * math.Pow(2, float64(attempt-1)))
	if base > maxDelay {
		base = maxDelay
	}
	jitter := time.Duration(rand.Float64() * 0.25 * float64(base))
	return base + jitter
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()

	// Client errors (4xx except 429) are never retryable
	if strings.Contains(msg, "status 400") ||
		strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "status 403") ||
		strings.Contains(msg, "status 404") ||
		strings.Contains(msg, "status 422") {
		return false
	}

	return strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 500") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 529") ||
		strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "connection reset")
}

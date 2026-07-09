package llm

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// IsRateLimitError reports whether err is an HTTP 429 / rate-limit response.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "API error 429:") || strings.Contains(msg, "status code 429")
}

// rateLimitRetryProvider retries ChatCompletion after a fixed backoff on 429 errors.
type rateLimitRetryProvider struct {
	inner       Provider
	backoffSec  int
	maxRetries  int
}

// NewRateLimitRetryProvider wraps a provider with 429 backoff retries.
// backoffSec <= 0 disables wrapping. maxRetries is the number of retries after the first 429.
func NewRateLimitRetryProvider(inner Provider, backoffSec, maxRetries int) Provider {
	if inner == nil || backoffSec <= 0 {
		return inner
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	return &rateLimitRetryProvider{
		inner:      inner,
		backoffSec: backoffSec,
		maxRetries: maxRetries,
	}
}

func (p *rateLimitRetryProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	attempts := p.maxRetries + 1
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := p.inner.ChatCompletion(ctx, req)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !IsRateLimitError(err) || attempt >= attempts {
			return nil, err
		}

		log.Printf("[INFO] LLM rate limited (429), backing off %ds (retry %d/%d)",
			p.backoffSec, attempt, p.maxRetries)

		timer := time.NewTimer(time.Duration(p.backoffSec) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("rate limit backoff cancelled: %w", ctx.Err())
		case <-timer.C:
		}
	}

	return nil, lastErr
}

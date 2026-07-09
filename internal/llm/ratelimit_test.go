package llm

import (
	"context"
	"errors"
	"testing"
	"time"
)

type stubProvider struct {
	responses []*ChatResponse
	errors    []error
	calls     int
}

func (s *stubProvider) ChatCompletion(ctx context.Context, req *ChatRequest) (*ChatResponse, error) {
	idx := s.calls
	s.calls++
	if idx < len(s.errors) && s.errors[idx] != nil {
		return nil, s.errors[idx]
	}
	if idx < len(s.responses) {
		return s.responses[idx], nil
	}
	return &ChatResponse{Content: "ok"}, nil
}

func TestIsRateLimitError(t *testing.T) {
	if !IsRateLimitError(errors.New(`API error 429: {"error":"rpm exhausted"}`)) {
		t.Fatal("expected rate limit error")
	}
	if IsRateLimitError(errors.New(`API error 404: not found`)) {
		t.Fatal("expected non-rate-limit error")
	}
}

func TestRateLimitRetryProviderSuccessAfterBackoff(t *testing.T) {
	inner := &stubProvider{
		errors: []error{
			errors.New(`API error 429: {"error":"rpm exhausted"}`),
		},
		responses: []*ChatResponse{{Content: "pong"}, {Content: "pong"}},
	}

	p := NewRateLimitRetryProvider(inner, 1, 2).(*rateLimitRetryProvider)
	start := time.Now()
	resp, err := p.ChatCompletion(context.Background(), &ChatRequest{Model: "m"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "pong" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if inner.calls != 2 {
		t.Fatalf("expected 2 calls, got %d", inner.calls)
	}
	if elapsed < time.Second {
		t.Fatalf("expected backoff delay, got %v", elapsed)
	}
}

func TestRateLimitRetryProviderDisabledWhenZeroBackoff(t *testing.T) {
	inner := &stubProvider{
		errors: []error{errors.New(`API error 429: throttled`)},
	}
	p := NewRateLimitRetryProvider(inner, 0, 2)
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("expected single call without retry, got %d", inner.calls)
	}
}

func TestRateLimitRetryProviderDoesNotRetryOtherErrors(t *testing.T) {
	inner := &stubProvider{
		errors: []error{errors.New(`API error 500: boom`)},
	}
	p := NewRateLimitRetryProvider(inner, 1, 2)
	_, err := p.ChatCompletion(context.Background(), &ChatRequest{Model: "m"})
	if err == nil {
		t.Fatal("expected error")
	}
	if inner.calls != 1 {
		t.Fatalf("expected single call, got %d", inner.calls)
	}
}

func TestRegistryWrapsProvidersWithRateLimitBackoff(t *testing.T) {
	r := &Registry{providers: map[string]Provider{"test": &stubProvider{}}}
	r.SetRateLimitBackoff(5, 1)

	p, err := r.Get("test")
	if err != nil {
		t.Fatalf("get provider: %v", err)
	}
	if _, ok := p.(*rateLimitRetryProvider); !ok {
		t.Fatalf("expected wrapped provider, got %T", p)
	}
}

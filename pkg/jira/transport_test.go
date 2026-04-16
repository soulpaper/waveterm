// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// --- RateLimitedTransport tests ---

func TestRateLimitedTransport_Throttles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Rate limit at 5 req/s => 200ms between requests, burst=1.
	// 5 requests need 4 waits of ~200ms = ~800ms minimum.
	transport := NewRateLimitedTransport(http.DefaultTransport, 5)
	client := &http.Client{Transport: transport}

	start := time.Now()
	for i := 0; i < 5; i++ {
		req, err := http.NewRequest("GET", srv.URL, nil)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		resp.Body.Close()
	}
	elapsed := time.Since(start)

	// 4 waits at 200ms each = 800ms. Allow some slack (700ms floor).
	if elapsed < 700*time.Millisecond {
		t.Errorf("5 requests at 5 req/s should take >= 700ms, took %v", elapsed)
	}
}

func TestRateLimitedTransport_ZeroRPS_Passthrough(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// rps <= 0 should return inner directly (passthrough, no wrapping).
	inner := http.DefaultTransport
	result := NewRateLimitedTransport(inner, 0)

	// The returned transport should be the same as inner (no wrapper).
	if result != inner {
		t.Errorf("NewRateLimitedTransport(inner, 0) should return inner directly")
	}
}

// --- RetryTransport tests ---

func TestRetryTransport_429_WithRetryAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	start := time.Now()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&reqCount); got != 2 {
		t.Errorf("expected exactly 2 requests, got %d", got)
	}
	if elapsed < 1*time.Second {
		t.Errorf("should wait >= 1s (Retry-After: 1), took %v", elapsed)
	}
}

func TestRetryTransport_429_MissingRetryAfter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		if n == 1 {
			// No Retry-After header.
			w.WriteHeader(429)
			fmt.Fprint(w, `{"message":"rate limited"}`)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	start := time.Now()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&reqCount); got != 2 {
		t.Errorf("expected exactly 2 requests, got %d", got)
	}
	// D-RETRY-02: default backoff is 5s when Retry-After is missing.
	if elapsed < 5*time.Second {
		t.Errorf("should wait >= 5s (default backoff), took %v", elapsed)
	}
}

func TestRetryTransport_429_ExcessiveRetryAfter(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(429)
		fmt.Fprint(w, `{"message":"rate limited"}`)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// Retry-After > 60s is fatal — no retry, return as-is.
	if resp.StatusCode != 429 {
		t.Errorf("expected 429 returned as-is, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&reqCount); got != 1 {
		t.Errorf("expected exactly 1 request (no retry), got %d", got)
	}
}

func TestRetryTransport_5xx_ExponentialBackoff(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		if n <= 2 {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	start := time.Now()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	// 3 requests: initial (500) + retry 1 (500, 1s wait) + retry 2 (200, 2s wait) = 3 total, ~3s
	if got := atomic.LoadInt32(&reqCount); got != 3 {
		t.Errorf("expected 3 requests, got %d", got)
	}
	// Backoff: 1s (attempt 0) + 2s (attempt 1) = 3s minimum.
	if elapsed < 3*time.Second {
		t.Errorf("expected >= 3s for 2 retries (1s + 2s backoff), took %v", elapsed)
	}
}

func TestRetryTransport_5xx_Exhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(503)
		fmt.Fprint(w, `{"message":"service unavailable"}`)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// 1 initial + 3 retries = 4 total requests.
	if got := atomic.LoadInt32(&reqCount); got != 4 {
		t.Errorf("expected 4 requests (1 + 3 retries), got %d", got)
	}
	// Final response should be 503 (exhausted retries).
	if resp.StatusCode != 503 {
		t.Errorf("expected 503 after retry exhaustion, got %d", resp.StatusCode)
	}
}

func TestRetryTransport_NonRetryable4xx(t *testing.T) {
	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(400)
		fmt.Fprint(w, `{"message":"bad request"}`)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	// 4xx (non-429) should not be retried.
	if got := atomic.LoadInt32(&reqCount); got != 1 {
		t.Errorf("expected exactly 1 request (no retry for 400), got %d", got)
	}
	if resp.StatusCode != 400 {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRetryTransport_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.WriteHeader(500)
		fmt.Fprint(w, `{"message":"server error"}`)
	}))
	defer srv.Close()

	transport := NewRetryTransport(http.DefaultTransport, 3)
	client := &http.Client{Transport: transport}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 500ms — before the 1s backoff completes.
	go func() {
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL, nil)
	_, err := client.Do(req)

	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !isContextError(err) {
		t.Errorf("expected context.Canceled or DeadlineExceeded, got: %v", err)
	}
	// Should have made only 1 request before cancellation during backoff.
	if got := atomic.LoadInt32(&reqCount); got > 2 {
		t.Errorf("expected at most 2 requests before cancellation, got %d", got)
	}
}

// isContextError checks if err wraps context.Canceled or context.DeadlineExceeded.
func isContextError(err error) bool {
	if err == nil {
		return false
	}
	// errors.Is walks the Unwrap chain for us.
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func TestRetryTransport_Composability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		if n == 1 {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"message":"server error"}`)
			return
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()

	// Compose: RetryTransport(RateLimitedTransport(DefaultTransport))
	// Rate limit at 10 req/s; retry up to 3 times.
	rateLimited := NewRateLimitedTransport(http.DefaultTransport, 10)
	retrying := NewRetryTransport(rateLimited, 3)
	client := &http.Client{Transport: retrying}

	start := time.Now()
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := client.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&reqCount); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
	// 1s backoff + rate limiter wait (100ms at 10 req/s) = ~1.1s minimum.
	if elapsed < 1*time.Second {
		t.Errorf("expected >= 1s (backoff + rate limit), took %v", elapsed)
	}
}

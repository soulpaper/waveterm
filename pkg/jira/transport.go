// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

// defaultRetryAfter is the backoff duration when a 429 response has no
// Retry-After header (or the header is unparseable). Per D-RETRY-02.
const defaultRetryAfter = 5 * time.Second

// maxRetryAfter is the ceiling for Retry-After values. If the server asks
// for longer than this, we treat the 429 as fatal (no retry). Per D-RETRY-02.
const maxRetryAfter = 60 * time.Second

// --- RateLimitedTransport ---

// rateLimitedTransport is an http.RoundTripper that throttles outbound
// requests via a token-bucket rate limiter. Per D-RL-01 the burst is 1
// so requests are evenly spaced.
type rateLimitedTransport struct {
	inner   http.RoundTripper
	limiter *rate.Limiter
}

// NewRateLimitedTransport wraps inner with a token-bucket rate limiter.
// rps = requests per second. If rps <= 0, no rate limiting is applied
// (inner is returned directly). Per D-RL-03 this supports "0 = no limit".
func NewRateLimitedTransport(inner http.RoundTripper, rps float64) http.RoundTripper {
	if rps <= 0 {
		return inner
	}
	return &rateLimitedTransport{
		inner:   inner,
		limiter: rate.NewLimiter(rate.Limit(rps), 1),
	}
}

// RoundTrip implements http.RoundTripper. It waits for a rate-limiter token
// (respecting the request's context deadline) then delegates to the inner
// transport. Per T-05-02 the caller can cancel if the limiter blocks too long.
func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.inner.RoundTrip(req)
}

// --- RetryTransport ---

// retryTransport is an http.RoundTripper that retries 429 and 5xx responses.
// 429 uses the Retry-After header (or defaultRetryAfter). 5xx uses exponential
// backoff (1s, 2s, 4s). Other status codes are returned immediately.
type retryTransport struct {
	inner      http.RoundTripper
	maxRetries int
}

// NewRetryTransport wraps inner with retry logic for 429 and 5xx responses.
// maxRetries is the maximum number of retry attempts (not counting the initial
// request). For example, maxRetries=3 means up to 4 total attempts.
func NewRetryTransport(inner http.RoundTripper, maxRetries int) http.RoundTripper {
	return &retryTransport{
		inner:      inner,
		maxRetries: maxRetries,
	}
}

// RoundTrip implements http.RoundTripper with retry logic.
//
// Retry policy:
//   - 429: honour Retry-After header (capped at maxRetryAfter). Missing/zero
//     header uses defaultRetryAfter. Retry-After > maxRetryAfter is fatal.
//   - 5xx: exponential backoff 1s, 2s, 4s (1<<attempt seconds).
//   - All other status codes: no retry.
//
// Per T-05-03, response bodies are closed before retrying to prevent leaking
// partial response data.
//
// Request bodies are buffered so POST/PUT requests can be replayed on retry.
// Jira request payloads are small (search JQL, ~100 bytes), so buffering is
// safe and bounded by the caller's payload size.
func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Buffer the request body (if any) so we can replay it on retries.
	// Without this, POST bodies are consumed on the first attempt and
	// subsequent retries send an empty body (ContentLength mismatch).
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
	}

	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		// Reset the body for each attempt.
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		resp, err = t.inner.RoundTrip(req)
		if err != nil {
			return nil, err
		}

		switch {
		case resp.StatusCode == 429:
			delay := parseRetryAfter(resp.Header.Get("Retry-After"))

			// Retry-After > 60s is fatal — Atlassian is throttling hard.
			if delay > maxRetryAfter {
				return resp, nil
			}
			// Missing or zero → use default.
			if delay == 0 {
				delay = defaultRetryAfter
			}
			// If we've exhausted retries, return the last response.
			if attempt == t.maxRetries {
				return resp, nil
			}
			// Close old body before retrying (T-05-03).
			resp.Body.Close()
			// Wait for the delay, respecting the request's context.
			if err := sleepWithContext(req.Context(), delay); err != nil {
				return nil, err
			}

		case resp.StatusCode >= 500 && resp.StatusCode < 600:
			// If we've exhausted retries, return the last response.
			if attempt == t.maxRetries {
				return resp, nil
			}
			// Exponential backoff: 1s, 2s, 4s for attempts 0, 1, 2.
			backoff := time.Duration(1<<uint(attempt)) * time.Second
			// Close old body before retrying (T-05-03).
			resp.Body.Close()
			// Wait for the backoff, respecting the request's context.
			if err := sleepWithContext(req.Context(), backoff); err != nil {
				return nil, err
			}

		default:
			// 2xx, 3xx, 4xx (non-429): return immediately, no retry.
			return resp, nil
		}
	}

	return resp, nil
}

// sleepWithContext blocks for the given duration or until the context is
// cancelled, whichever comes first. Returns the context error if cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

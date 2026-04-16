// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

// integration_test.go — End-to-end tests verifying that RateLimitedTransport
// + RetryTransport work correctly when wired through the full Client.SearchIssues
// / Client.GetIssue path. These complement the unit-level transport_test.go by
// exercising the entire doJSON → APIError → retry pipeline.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// newIntegrationClient creates a Client with transport wrapping applied,
// pointed at the given test server. This mirrors what NewClient does in
// production: RetryTransport(RateLimitedTransport(inner)).
//
// rps=0 disables rate limiting (faster tests that only care about retry).
func newIntegrationClient(t *testing.T, srv *httptest.Server, rps float64, maxRetries int) *Client {
	t.Helper()
	cfg := Config{
		BaseUrl:  srv.URL,
		CloudId:  "test-cloud-id",
		Email:    "user@example.com",
		ApiToken: "test-token",
		Jql:      "assignee = currentUser()",
		PageSize: 50,
	}
	// Start from the test server's transport (handles TLS for httptest).
	inner := srv.Client().Transport
	if inner == nil {
		inner = http.DefaultTransport
	}
	transport := NewRateLimitedTransport(inner, rps)
	transport = NewRetryTransport(transport, maxRetries)
	hc := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	return NewClientWithHTTP(cfg, hc)
}

// --------------------------------------------------------------------------
// Test: SearchIssues retries a 429 and succeeds on second attempt
// --------------------------------------------------------------------------

func TestIntegration_SearchIssues_Retries429(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive integration test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			fmt.Fprint(w, `{"errorMessages":["rate limited"]}`)
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"issues":[{"id":"1","key":"PROJ-1"}],"isLast":true,"nextPageToken":""}`)
	}))
	defer srv.Close()

	c := newIntegrationClient(t, srv, 0, 3) // no rate limit, 3 retries

	start := time.Now()
	result, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=PROJ"})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}
	if result == nil || len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %v", result)
	}
	if result.Issues[0].Key != "PROJ-1" {
		t.Errorf("issue key: got %q, want PROJ-1", result.Issues[0].Key)
	}
	if got := atomic.LoadInt32(&reqCount); got != 2 {
		t.Errorf("expected 2 requests (1 retry), got %d", got)
	}
	if elapsed < 1*time.Second {
		t.Errorf("should wait >= 1s for Retry-After: 1, took %v", elapsed)
	}
}

// --------------------------------------------------------------------------
// Test: GetIssue retries 5xx with exponential backoff and succeeds
// --------------------------------------------------------------------------

func TestIntegration_GetIssue_Retries5xx(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive integration test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n <= 2 {
			w.WriteHeader(502)
			fmt.Fprint(w, `{"errorMessages":["bad gateway"]}`)
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"10001","key":"PROJ-42","fields":{"summary":"Fixed"}}`)
	}))
	defer srv.Close()

	c := newIntegrationClient(t, srv, 0, 3) // no rate limit, 3 retries

	start := time.Now()
	issue, err := c.GetIssue(context.Background(), "PROJ-42", GetIssueOpts{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success after 5xx retries, got error: %v", err)
	}
	if issue.Key != "PROJ-42" {
		t.Errorf("issue key: got %q, want PROJ-42", issue.Key)
	}
	if issue.Fields.Summary != "Fixed" {
		t.Errorf("summary: got %q, want Fixed", issue.Fields.Summary)
	}
	if got := atomic.LoadInt32(&reqCount); got != 3 {
		t.Errorf("expected 3 requests (2 retries), got %d", got)
	}
	// Backoff: 1s (attempt 0) + 2s (attempt 1) = 3s minimum.
	if elapsed < 3*time.Second {
		t.Errorf("expected >= 3s for exponential backoff (1s + 2s), took %v", elapsed)
	}
}

// --------------------------------------------------------------------------
// Test: SearchIssues with 5xx exhaustion returns APIError (not infinite loop)
// --------------------------------------------------------------------------

func TestIntegration_SearchIssues_5xxExhaustion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive integration test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(503)
		fmt.Fprint(w, `{"errorMessages":["service unavailable"]}`)
	}))
	defer srv.Close()

	c := newIntegrationClient(t, srv, 0, 3) // 3 retries = 4 total attempts

	_, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=PROJ"})
	if err == nil {
		t.Fatal("expected error after retry exhaustion, got nil")
	}

	// The error should be an *APIError with status 503.
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 503 {
		t.Errorf("expected status 503, got %d", apiErr.StatusCode)
	}

	// 1 initial + 3 retries = 4 total.
	if got := atomic.LoadInt32(&reqCount); got != 4 {
		t.Errorf("expected 4 requests (1 + 3 retries), got %d", got)
	}
}

// --------------------------------------------------------------------------
// Test: GetIssue with rate limiting + 429 retry (full composition)
// --------------------------------------------------------------------------

func TestIntegration_GetIssue_RateLimitAndRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive integration test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			fmt.Fprint(w, `{"errorMessages":["rate limited"]}`)
			return
		}
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"10001","key":"PROJ-99","fields":{"summary":"Rate limited then OK"}}`)
	}))
	defer srv.Close()

	// Apply both rate limiting (10 req/s) and retry (3 max).
	c := newIntegrationClient(t, srv, 10, 3)

	start := time.Now()
	issue, err := c.GetIssue(context.Background(), "PROJ-99", GetIssueOpts{})
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if issue.Key != "PROJ-99" {
		t.Errorf("issue key: got %q, want PROJ-99", issue.Key)
	}
	if got := atomic.LoadInt32(&reqCount); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
	// Must wait >= 1s for Retry-After. Rate limiter adds ~100ms on top.
	if elapsed < 1*time.Second {
		t.Errorf("expected >= 1s (retry-after + rate limit), took %v", elapsed)
	}
}

// --------------------------------------------------------------------------
// Test: Context cancellation during retry backoff
// --------------------------------------------------------------------------

func TestIntegration_SearchIssues_ContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timing-sensitive integration test in short mode")
	}

	var reqCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&reqCount, 1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(500)
		fmt.Fprint(w, `{"errorMessages":["server error"]}`)
	}))
	defer srv.Close()

	c := newIntegrationClient(t, srv, 0, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := c.SearchIssues(ctx, SearchOpts{JQL: "project=PROJ"})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}

	// Should have made only 1 request before the context timeout during 1s backoff.
	if got := atomic.LoadInt32(&reqCount); got > 2 {
		t.Errorf("expected at most 2 requests before context timeout, got %d", got)
	}
}

// --------------------------------------------------------------------------
// Test: NewClient applies default transport wrapping
// --------------------------------------------------------------------------

func TestNewClient_AppliesDefaultTransportWrapping(t *testing.T) {
	// Verify that NewClient produces a client with a non-nil, wrapped transport.
	cfg := Config{
		BaseUrl:  "https://example.atlassian.net",
		CloudId:  "test-cloud",
		Email:    "user@example.com",
		ApiToken: "test-token",
	}
	c := NewClient(cfg)
	if c.hc == nil {
		t.Fatal("NewClient produced nil http.Client")
	}
	if c.hc.Transport == nil {
		t.Fatal("NewClient should set a wrapped Transport, got nil")
	}
	if c.hc.Timeout != defaultTimeout {
		t.Errorf("Timeout: got %v, want %v", c.hc.Timeout, defaultTimeout)
	}

	// The transport should be a *retryTransport wrapping a *rateLimitedTransport.
	rt, ok := c.hc.Transport.(*retryTransport)
	if !ok {
		t.Fatalf("expected *retryTransport, got %T", c.hc.Transport)
	}
	if rt.maxRetries != defaultMaxRetries {
		t.Errorf("maxRetries: got %d, want %d", rt.maxRetries, defaultMaxRetries)
	}
	rlt, ok := rt.inner.(*rateLimitedTransport)
	if !ok {
		t.Fatalf("expected *rateLimitedTransport as inner, got %T", rt.inner)
	}
	if rlt.inner != http.DefaultTransport {
		t.Errorf("innermost transport should be http.DefaultTransport")
	}
}

// --------------------------------------------------------------------------
// Test: NewClientWithHTTP does NOT wrap transport (no double-wrapping)
// --------------------------------------------------------------------------

func TestNewClientWithHTTP_NoDoubleWrapping(t *testing.T) {
	cfg := Config{
		BaseUrl:  "https://example.atlassian.net",
		CloudId:  "test-cloud",
		Email:    "user@example.com",
		ApiToken: "test-token",
	}
	customTransport := &http.Transport{}
	hc := &http.Client{Transport: customTransport}

	c := NewClientWithHTTP(cfg, hc)

	// The transport must be exactly what the caller provided — no wrapping.
	if c.hc.Transport != customTransport {
		t.Errorf("NewClientWithHTTP should preserve caller's transport, got %T", c.hc.Transport)
	}
}

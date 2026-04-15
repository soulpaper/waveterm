// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors — callers use errors.Is(err, ErrX) for class checks.
// Per D-18, these are plain errors.New sentinels (no wrapping, no codes).
var (
	// HTTP status bucket sentinels — returned via APIError.Unwrap().
	ErrUnauthorized = errors.New("jira: unauthorized") // 401
	ErrForbidden    = errors.New("jira: forbidden")    // 403
	ErrNotFound     = errors.New("jira: not found")    // 404
	ErrRateLimited  = errors.New("jira: rate limited") // 429
	ErrServerError  = errors.New("jira: server error") // 5xx

	// Config loader sentinels.
	ErrConfigNotFound   = errors.New("jira: config not found")
	ErrConfigInvalid    = errors.New("jira: config invalid")
	ErrConfigIncomplete = errors.New("jira: config incomplete")
)

// APIError is returned for every non-2xx response from the Jira API. Callers
// use errors.Is(err, ErrUnauthorized) for class checks and errors.As(err, &apiErr)
// when they need the status code, endpoint, or response body.
//
// Per D-20, RetryAfter is populated only when StatusCode == 429 AND the server
// sent a parseable Retry-After header.
type APIError struct {
	StatusCode int
	Endpoint   string
	Method     string
	Body       string        // truncated to 1 KB by the client
	RetryAfter time.Duration // populated iff StatusCode == 429 and header parseable
}

// Error implements the error interface. Format is intentionally short and
// debug-oriented — callers that need to display a user-facing message should
// use errors.Is to dispatch on the sentinel.
//
// Threat T-01-02-01: deliberately omits Body and any auth headers so that
// downstream loggers do not accidentally leak response payloads.
func (e *APIError) Error() string {
	return fmt.Sprintf("jira: HTTP %d %s %s", e.StatusCode, e.Method, e.Endpoint)
}

// Unwrap returns the sentinel that matches the HTTP status bucket. This lets
// errors.Is(err, ErrUnauthorized) succeed without the caller needing to know
// about APIError at all.
//
// Unrecognized status codes (non-401/403/404/429/5xx, e.g. 418) return nil,
// meaning errors.Is(err, anySentinel) is false for them — callers must fall
// back to errors.As to inspect the struct.
func (e *APIError) Unwrap() error {
	switch {
	case e.StatusCode == 401:
		return ErrUnauthorized
	case e.StatusCode == 403:
		return ErrForbidden
	case e.StatusCode == 404:
		return ErrNotFound
	case e.StatusCode == 429:
		return ErrRateLimited
	case e.StatusCode >= 500 && e.StatusCode < 600:
		return ErrServerError
	default:
		return nil
	}
}

// parseRetryAfter parses an Atlassian Retry-After header value. Per
// https://developer.atlassian.com/cloud/jira/platform/rate-limiting/ the value
// is always an integer number of seconds. Returns 0 if the header is empty,
// unparseable, or negative — Phase 5's rate limiter is responsible for picking
// a default backoff when this returns 0.
func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	secs, err := strconv.Atoi(header)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}

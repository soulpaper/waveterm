---
phase: 05-on-demand-downloads-hardening
plan: 03
subsystem: jira-transport
tags: [transport-wiring, integration-test, retry, rate-limiting]
dependency_graph:
  requires: [NewRateLimitedTransport, NewRetryTransport, pkg/jira/client.go#NewClient]
  provides: [default-transport-wrapping, integration-test-coverage, POST-body-retry-fix]
  affects: [pkg/jira/client.go, pkg/jira/transport.go]
tech_stack:
  added: []
  patterns: [http.RoundTripper composition, request body buffering for retry]
key_files:
  created:
    - pkg/jira/integration_test.go
  modified:
    - pkg/jira/client.go
    - pkg/jira/transport.go
decisions:
  - "NewClient wraps with RetryTransport(RateLimitedTransport(http.DefaultTransport)) per D-RETRY-04"
  - "NewClientWithHTTP does NOT wrap — caller owns transport (test seam, no double-wrapping)"
  - "RetryTransport buffers request body for POST retry safety"
metrics:
  duration_seconds: 292
  completed: "2026-04-16T01:39:54Z"
  tasks_completed: 3
  tasks_total: 3
  test_count: 7
  test_pass: 7
---

# Phase 5 Plan 3: Wire Transport + Integration Test Summary

RetryTransport(RateLimitedTransport(http.DefaultTransport)) wired into NewClient default path with 10 rps / 3 max retries; POST body buffering fix for retry safety; 7 integration tests covering 429 retry, 5xx backoff, exhaustion, context cancellation, rate limiting composition, and no-double-wrapping guarantee.

## Task Results

| Task | Name | Commit | Status |
|------|------|--------|--------|
| 1 | Wire transport wrapping into NewClient | `3a3e64f5` | PASS |
| 2 | Fix POST body replay in RetryTransport | `f0ab2e50` | PASS (Rule 1 bug fix) |
| 3 | Add integration tests for transport wiring | `7993cf67` | PASS |

## Implementation Details

### NewClient Transport Wrapping (`client.go`)
- Added `defaultRPS = 10.0` and `defaultMaxRetries = 3` constants
- `NewClient` now constructs: `RetryTransport(RateLimitedTransport(http.DefaultTransport))`
- Each retry re-acquires a rate-limiter token (D-RETRY-04)
- `NewClientWithHTTP` remains unchanged -- caller's transport is used as-is

### POST Body Buffering Fix (`transport.go`)
- `RetryTransport.RoundTrip` now buffers `req.Body` via `io.ReadAll` before the retry loop
- Each attempt resets `req.Body = io.NopCloser(bytes.NewReader(bodyBytes))`
- Fixes `ContentLength=N with Body length 0` error on POST retries (SearchIssues uses POST)
- Jira payloads are small (~100 bytes for search JQL), so buffering is safe

### Integration Tests (`integration_test.go`)

| Test | What it verifies | Timing |
|------|-----------------|--------|
| TestIntegration_SearchIssues_Retries429 | 429 + Retry-After through POST SearchIssues path | 1.0s |
| TestIntegration_GetIssue_Retries5xx | 502x2 then 200, exponential backoff through GetIssue | 3.0s |
| TestIntegration_SearchIssues_5xxExhaustion | 503x4, returns APIError after exhaustion | 7.0s |
| TestIntegration_GetIssue_RateLimitAndRetry | Rate limit (10 rps) + 429 retry composition | 1.0s |
| TestIntegration_SearchIssues_ContextCancellation | Context timeout during 5xx backoff | 0.5s |
| TestNewClient_AppliesDefaultTransportWrapping | Verifies retryTransport > rateLimitedTransport > DefaultTransport chain | instant |
| TestNewClientWithHTTP_NoDoubleWrapping | Caller's custom transport preserved unchanged | instant |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] POST body consumed on retry, causing ContentLength mismatch**
- **Found during:** Task 2 (integration test execution)
- **Issue:** `RetryTransport.RoundTrip` re-sent the same `*http.Request` on retry, but `req.Body` was already consumed by the first attempt. POST requests (SearchIssues) failed with `http: ContentLength=38 with Body length 0`.
- **Fix:** Buffer `req.Body` into `[]byte` before the retry loop, reset `req.Body` via `io.NopCloser(bytes.NewReader(bodyBytes))` on each attempt.
- **Files modified:** `pkg/jira/transport.go`
- **Commit:** `f0ab2e50`

## Verification

```
go build ./pkg/jira/...     -> clean
go vet ./pkg/jira/...       -> clean
go test ./pkg/jira/ -count=1 -> ok (31.2s, 35 tests, all pass, no regressions)
```

## Self-Check: PASSED

All files found: `pkg/jira/integration_test.go`, `pkg/jira/client.go`, `pkg/jira/transport.go`, `05-03-wire-transport-integration-test-SUMMARY.md`.
All commits found: `3a3e64f5`, `f0ab2e50`, `7993cf67`.

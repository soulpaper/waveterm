---
phase: 05-on-demand-downloads-hardening
plan: 01
subsystem: jira-transport
tags: [rate-limiting, retry, http-transport, tdd]
dependency_graph:
  requires: [golang.org/x/time/rate, pkg/jira/errors.go#parseRetryAfter]
  provides: [NewRateLimitedTransport, NewRetryTransport, sleepWithContext]
  affects: []
tech_stack:
  added: []
  patterns: [http.RoundTripper middleware, token-bucket rate limiting, exponential backoff]
key_files:
  created:
    - pkg/jira/transport.go
    - pkg/jira/transport_test.go
  modified: []
decisions:
  - "Burst=1 for rate limiter (D-RL-01) — evenly spaced requests"
  - "No jitter on backoff — deterministic for testability per plan guidance"
  - "sleepWithContext uses time.NewTimer + select for cancellation safety"
metrics:
  duration_seconds: 342
  completed: "2026-04-16T01:31:00Z"
  tasks_completed: 2
  tasks_total: 2
  test_count: 10
  test_pass: 10
---

# Phase 5 Plan 1: Rate Limiter + Retry Transports Summary

Token-bucket rate limiter (golang.org/x/time/rate, burst=1) and retry transport with 429 Retry-After honoring (5s default, 60s cap) plus 5xx exponential backoff (1s/2s/4s, max 3 retries), composable as http.RoundTripper middleware.

## Task Results

| Task | Name | Commit | Status |
|------|------|--------|--------|
| 1 | Write failing tests for RateLimitedTransport and RetryTransport | `f3b3c1ee` | PASS (RED) |
| 2 | Implement RateLimitedTransport and RetryTransport | `a93be8cd` | PASS (GREEN) |

## Implementation Details

### RateLimitedTransport (`NewRateLimitedTransport`)
- Wraps any `http.RoundTripper` with `rate.Limiter.Wait` call before delegation
- `rps <= 0` returns inner directly (passthrough, no allocation)
- Burst fixed at 1 per D-RL-01 for even request spacing
- Context-aware: cancellation during Wait returns immediately

### RetryTransport (`NewRetryTransport`)
- **429 handling:** Parses `Retry-After` header via existing `parseRetryAfter`. Missing/zero defaults to 5s (D-RETRY-02). Values > 60s treated as fatal (no retry)
- **5xx handling:** Exponential backoff `1<<attempt` seconds (1s, 2s, 4s). Max 3 retries = 4 total attempts
- **Non-retryable:** All other 4xx returned immediately
- **Context cancellation:** `sleepWithContext` uses `time.NewTimer` + `select` to respect deadline/cancel
- **Response body safety:** Old `resp.Body` closed before every retry (T-05-03)

### Composability
Designed for `RetryTransport(RateLimitedTransport(http.DefaultTransport))` — each retry re-acquires a rate limiter token (D-RETRY-04).

## Test Coverage

| Test | What it verifies | Timing |
|------|-----------------|--------|
| TestRateLimitedTransport_Throttles | 5 requests at 5 req/s take >= 700ms | 0.8s |
| TestRateLimitedTransport_ZeroRPS_Passthrough | rps=0 returns inner directly | instant |
| TestRetryTransport_429_WithRetryAfter | Retry-After: 1 honoured, 2 requests, >= 1s | 1.0s |
| TestRetryTransport_429_MissingRetryAfter | No header, default 5s backoff, 2 requests | 5.0s |
| TestRetryTransport_429_ExcessiveRetryAfter | Retry-After: 120 is fatal, 1 request | instant |
| TestRetryTransport_5xx_ExponentialBackoff | 500x2 then 200, 3 requests, >= 3s | 3.0s |
| TestRetryTransport_5xx_Exhaustion | 503x4, 4 requests, returns final 503 | 7.0s |
| TestRetryTransport_NonRetryable4xx | 400 returned immediately, 1 request | instant |
| TestRetryTransport_ContextCancellation | Cancel during backoff, context error | 0.5s |
| TestRetryTransport_Composability | RetryTransport + RateLimitedTransport together | 1.0s |

## Deviations from Plan

None - plan executed exactly as written.

## Threat Mitigations Verified

| Threat ID | Mitigation | Verified By |
|-----------|-----------|-------------|
| T-05-01 (DoS via Retry-After) | Cap at 60s, max 3 retries, bounded backoff (7s max) | TestRetryTransport_429_ExcessiveRetryAfter, TestRetryTransport_5xx_Exhaustion |
| T-05-02 (DoS via rate limiter block) | rate.Limiter.Wait respects context; caller can cancel | TestRetryTransport_ContextCancellation |
| T-05-03 (Info disclosure via response leak) | resp.Body.Close() before every retry | Code inspection in RoundTrip loop |

## Verification

```
go build ./pkg/jira/...     -> clean
go vet ./pkg/jira/...       -> clean
go test ./pkg/jira/ -count=1 -> ok (18.7s, all pass, no regressions)
```

## Self-Check: PASSED

All files found: `pkg/jira/transport.go`, `pkg/jira/transport_test.go`, `05-01-SUMMARY.md`.
All commits found: `f3b3c1ee`, `a93be8cd`.

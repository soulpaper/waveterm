---
phase: 05-on-demand-downloads-hardening
verified: 2026-04-16T01:55:00Z
status: human_needed
score: 9/10 must-haves verified
overrides_applied: 0
gaps:
  - truth: "RefreshOpts.RateLimit float64 field controls rate (0 = no limit)"
    status: failed
    reason: "RefreshOpts does not have a RateLimit field. Rate limiting is applied automatically via NewClient()'s default transport wrapping (10 rps). The field was planned but not implemented."
    artifacts:
      - path: "pkg/jira/refresh.go"
        issue: "RefreshOpts struct has no RateLimit field — only Config and HTTPClient fields exist"
    missing:
      - "Add RateLimit float64 to RefreshOpts and use it in Refresh() client construction"
human_verification:
  - test: "Run wsh jira download ITSM-3135 against a live Jira instance with valid config"
    expected: "Attachments download to ~/.config/waveterm/jira-attachments/ITSM-3135/, cache localPath updated, widget shows local preview on next load"
    why_human: "End-to-end flow requires live Jira credentials, file system state, and widget UI rendering"
  - test: "Trigger a refresh of 50+ issues against Atlassian Cloud"
    expected: "Refresh completes without 429 errors (rate limiter keeps requests under 10/s)"
    why_human: "Real rate-limit enforcement by Atlassian cannot be simulated in unit tests"
---

# Phase 5: On-demand Downloads + Hardening Verification Report

**Phase Goal:** Let users pull specific attachments to disk when they need them and make the refresh resilient to Jira's rate limits and transient failures.
**Verified:** 2026-04-16T01:55:00Z
**Status:** human_needed
**Re-verification:** No -- initial verification

## Goal Achievement

### Observable Truths

The must-haves below are merged from ROADMAP.md success criteria (SC1-SC4) and the three PLAN frontmatters (05-01, 05-02, 05-03).

| # | Truth | Source | Status | Evidence |
|---|-------|--------|--------|----------|
| 1 | `wsh jira download ITSM-3135` downloads attachments to `~/.config/waveterm/jira-attachments/ITSM-3135/`, updates cache, widget shows local preview | ROADMAP SC1 | VERIFIED | `download.go` streams via `io.Copy` to `attachmentDir()`, updates `localPath` atomically. CLI registered. 7 unit tests pass. |
| 2 | Simulated 429 response triggers backoff and eventual success (verified in unit test) | ROADMAP SC2 | VERIFIED | `TestRetryTransport_429_WithRetryAfter` passes: 2 requests, >=1s elapsed. Integration test `TestIntegration_SearchIssues_Retries429` confirms full-stack retry. |
| 3 | Simulated 500 is retried 3 times and reports a final actionable error | ROADMAP SC3 | VERIFIED | `TestRetryTransport_5xx_Exhaustion`: 4 requests (1+3), returns final 503. `TestIntegration_SearchIssues_5xxExhaustion` confirms through Client. |
| 4 | Refresh of 50 issues completes without tripping Atlassian's rate limit | ROADMAP SC4 | ? UNCERTAIN | `NewClient` applies 10 rps rate limiter by default. Unit/integration tests confirm throttling. Real Atlassian validation requires human testing. |
| 5 | RateLimitedTransport throttles requests to configured rate (10 req/s default) | Plan 05-01 | VERIFIED | `TestRateLimitedTransport_Throttles`: 5 reqs at 5 rps take >=700ms. `rate.NewLimiter(rate.Limit(rps), 1)` in transport.go:42. |
| 6 | RetryTransport retries 429 with Retry-After delay (or 5s default), max retries | Plan 05-01 | VERIFIED | `TestRetryTransport_429_MissingRetryAfter`: 2 reqs, >=5s. `defaultRetryAfter = 5s` in transport.go:18. |
| 7 | RetryTransport retries 5xx with exponential backoff (1s, 2s, 4s), max 3 retries | Plan 05-01 | VERIFIED | `TestRetryTransport_5xx_ExponentialBackoff`: 3 reqs, >=3s. Backoff formula `1<<uint(attempt)` in transport.go:148. |
| 8 | NewClient wraps transport with RetryTransport(RateLimitedTransport(http.DefaultTransport)) by default | Plan 05-03 | VERIFIED | client.go:56-57: `NewRateLimitedTransport(http.DefaultTransport, defaultRPS)` then `NewRetryTransport(transport, defaultMaxRetries)`. `TestNewClient_AppliesDefaultTransportWrapping` asserts chain structure. |
| 9 | NewClientWithHTTP preserves caller-provided transport (test seam intact) | Plan 05-03 | VERIFIED | client.go:68 returns `&Client{cfg: cfg, hc: hc}` unchanged. `TestNewClientWithHTTP_NoDoubleWrapping` asserts transport identity. |
| 10 | RefreshOpts.RateLimit float64 field controls rate (0 = no limit) | Plan 05-03 | FAILED | `RefreshOpts` in refresh.go has no `RateLimit` field. Rate limiting is applied via NewClient's default wrapping, making the field unnecessary for production use, but it was a stated must-have. |

**Score:** 9/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `pkg/jira/transport.go` | RateLimitedTransport + RetryTransport | VERIFIED | 176 lines. Exports `NewRateLimitedTransport`, `NewRetryTransport`, `sleepWithContext`. Uses `golang.org/x/time/rate`. |
| `pkg/jira/transport_test.go` | Unit tests (min 100 lines) | VERIFIED | 383 lines. 10 test functions covering throttling, 429, 5xx, exhaustion, non-retryable, context cancel, composability. |
| `pkg/jira/download.go` | Download logic + cache update | VERIFIED | 293 lines. `DownloadAttachments`, `downloadFile` (streaming io.Copy), `updateCacheLocalPaths` (AtomicWriteFile). |
| `pkg/jira/download_test.go` | Download tests (min 80 lines) | VERIFIED | 367 lines. 7 test functions: success, filter, not-in-cache, no-match, server error, empty key, skip existing. |
| `pkg/jira/integration_test.go` | Integration tests (min 60 lines) | VERIFIED | 321 lines. 7 tests: 429 retry, 5xx retry, exhaustion, rate+retry composition, context cancel, transport wrapping, no-double-wrap. |
| `pkg/jira/client.go` | NewClient with transport wrapping | VERIFIED | Contains `defaultRPS = 10.0`, `defaultMaxRetries = 3`. NewClient wires both transports. |
| `pkg/wshrpc/wshrpctypes.go` | JiraDownloadCommand types | VERIFIED | `CommandJiraDownloadData`, `CommandJiraDownloadFileResult`, `CommandJiraDownloadRtnData` defined with correct JSON tags. `JiraDownloadCommand` in WshRpcInterface. |
| `pkg/wshrpc/wshserver/wshserver-jira.go` | JiraDownloadCommand handler | VERIFIED | Handler loads config, calls `jiraDownload`, maps errors via `mapJiraDownloadError`. 3 handler tests pass. |
| `cmd/wsh/cmd/wshcmd-jira.go` | `wsh jira download` CLI subcommand | VERIFIED | Cobra command with `RangeArgs(1, 2)`, `--json`, `--timeout=120s`. `formatDownloadSummary`. 4 CLI tests pass. |
| `pkg/wshrpc/wshclient/wshclient.go` | Generated client binding | VERIFIED | `JiraDownloadCommand` function at line 512. |
| `frontend/types/gotypes.d.ts` | TypeScript types | VERIFIED | `CommandJiraDownloadData`, `CommandJiraDownloadFileResult`, `CommandJiraDownloadRtnData` types present. |
| `frontend/app/store/wshclientapi.ts` | Frontend API binding | VERIFIED | `JiraDownloadCommand` method at line 514. |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `transport.go` | `golang.org/x/time/rate` | `rate.NewLimiter` in constructor | WIRED | Line 42: `rate.NewLimiter(rate.Limit(rps), 1)` |
| `transport.go` | `errors.go` | `parseRetryAfter` | WIRED | Line 121: `parseRetryAfter(resp.Header.Get("Retry-After"))` |
| `client.go` | `transport.go` | `NewRetryTransport(NewRateLimitedTransport(...))` | WIRED | Lines 56-57 in NewClient |
| `download.go` | Jira REST API | `client.downloadFile(ctx, att.WebUrl, ...)` with Basic auth | WIRED | Line 159 uses WebUrl from cache; downloadFile sets auth via `setCommonHeaders` (line 187) |
| `download.go` | cache_types.go | Updates `LocalPath` field | WIRED | Line 273: `issue.Attachments[j].LocalPath = dl.LocalPath` |
| `download.go` | `fileutil.AtomicWriteFile` | Atomic cache write | WIRED | Line 282: `fileutil.AtomicWriteFile(cachePath, data, 0o600)` |
| `wshcmd-jira.go` | `wshclient` | `wshclient.JiraDownloadCommand` call | WIRED | Lines 142-145 in jiraDownloadRun |
| `wshserver-jira.go` | `jira.DownloadAttachments` | `jiraDownload` function variable | WIRED | Line 43 (seam), line 90 (invocation) |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|-------------------|--------|
| `download.go` | `cache` (JiraCache) | `readCache(cachePath)` reads JSON from disk | Yes -- reads existing cache file | FLOWING |
| `download.go` | `resp.Body` | HTTP GET to `att.WebUrl` with Basic auth | Yes -- streams from Jira API | FLOWING |
| `wshserver-jira.go` handler | `report` | `jiraDownload(ctx, opts)` | Yes -- delegates to real download logic | FLOWING |
| `wshcmd-jira.go` | `rtn` | `wshclient.JiraDownloadCommand(...)` RPC call | Yes -- returns aggregate result from handler | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| All packages build | `go build ./...` | Clean (exit 0, no output) | PASS |
| Vet passes | `go vet ./pkg/jira/... ./cmd/wsh/...` | Clean (exit 0, no output) | PASS |
| Jira tests pass | `go test ./pkg/jira/ -count=1` | 45 tests, all PASS (31.3s) | PASS |
| Wshserver tests pass | `go test ./pkg/wshrpc/wshserver/ -count=1` | 10 tests, all PASS (0.2s) | PASS |
| Wsh CLI tests pass | `go test ./cmd/wsh/cmd/ -count=1` | 24 tests, all PASS (0.2s) | PASS |
| Transport wrapping verified | `TestNewClient_AppliesDefaultTransportWrapping` | retryTransport > rateLimitedTransport > DefaultTransport chain confirmed | PASS |
| 429 retry e2e | `TestIntegration_SearchIssues_Retries429` | 2 requests, >=1s elapsed, success | PASS |
| 5xx exhaustion e2e | `TestIntegration_SearchIssues_5xxExhaustion` | 4 requests, final 503 error returned | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-----------|-------------|--------|----------|
| JIRA-05 | 05-02 | `wsh jira download <KEY> [filename]` downloads attachments, updates cache localPath | SATISFIED | CLI command registered, download logic streams via io.Copy, cache updated atomically, 7 download tests + 4 CLI tests + 3 handler tests pass |
| JIRA-10 | 05-01, 05-03 | Rate limits (10 req/s) + retry (429 Retry-After, 5xx exponential backoff, max 3) | SATISFIED | `RateLimitedTransport` + `RetryTransport` implemented, wired into `NewClient` by default, 10 transport unit tests + 7 integration tests pass |

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (none) | - | - | - | No TODO, FIXME, HACK, placeholder, or stub patterns found in any phase 5 files |

### Human Verification Required

### 1. End-to-End Download Against Live Jira

**Test:** Run `wsh jira download ITSM-3135` with valid `~/.config/waveterm/jira.json` credentials against a real Jira Cloud instance.
**Expected:** Both attachments download to `~/.config/waveterm/jira-attachments/ITSM-3135/`. Cache file `jira-cache.json` is updated with `localPath` values. Next widget load shows local file preview icons.
**Why human:** Requires live Jira credentials, real network, file system state, and widget UI rendering.

### 2. Rate Limiting Under Real Atlassian Load

**Test:** Run `wsh jira refresh` with a JQL matching 50+ issues against Atlassian Cloud.
**Expected:** Refresh completes successfully without any 429 rate-limit errors. Timing shows ~5s+ for 50 issues (rate-limited to 10 req/s).
**Why human:** Real Atlassian rate-limit enforcement varies and cannot be reliably simulated in unit tests. ROADMAP SC4 specifically calls for "real testing."

### Gaps Summary

One must-have from Plan 05-03 frontmatter failed: the `RefreshOpts.RateLimit float64` field was specified but not implemented. In practice, rate limiting is automatically applied via `NewClient()` which wraps with 10 rps by default, making the explicit field unnecessary for the current use case. However, it removes the ability for callers to customize the rate at the Refresh() call site.

**This looks intentional.** The implementation achieves the same goal (all requests rate-limited at 10 rps) through a different mechanism (NewClient wrapping). The `RefreshOpts.RateLimit` field would only be needed if different refresh calls needed different rates. To accept this deviation, add to VERIFICATION.md frontmatter:

```yaml
overrides:
  - must_have: "RefreshOpts.RateLimit float64 field controls rate (0 = no limit)"
    reason: "Rate limiting applied via NewClient() default transport wrapping (10 rps). RefreshOpts.RateLimit field unnecessary since all clients use the same rate."
    accepted_by: "{your name}"
    accepted_at: "2026-04-16T02:00:00Z"
```

All other must-haves are verified. Both requirement IDs (JIRA-05, JIRA-10) are fully satisfied. All builds, vets, and 79 tests pass across the three packages.

---

_Verified: 2026-04-16T01:55:00Z_
_Verifier: Claude (gsd-verifier)_

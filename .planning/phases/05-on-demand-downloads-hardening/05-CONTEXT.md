# Phase 5: On-demand Downloads + Hardening - Context

**Gathered:** 2026-04-16
**Status:** Ready for planning
**Mode:** Auto-generated — decisions derived from REQUIREMENTS (JIRA-05, JIRA-10) and existing Phase 1-4 code

<domain>
## Phase Boundary

Two independent features:

1. **Attachment downloads** — `wsh jira download <KEY> [filename]` fetches attachment file(s) to `~/.config/waveterm/jira-attachments/<KEY>/<filename>`, updates the cache's `localPath`, widget picks up local files on next load.
2. **Rate limiting + retry** — token-bucket rate limiter (10 req/s default) + retry with Retry-After on 429 + exponential backoff on 5xx (max 3 retries). Applied to all outbound Jira HTTP calls.

**Out of scope:** Encryption of apiToken (JIRA-F-01), streaming progress UI in widget (deferred from Phase 3).

</domain>

<decisions>
## Implementation Decisions

### Attachment Download (D-DL-01 .. D-DL-06)

- **D-DL-01**: New CLI command — `wsh jira download <ISSUE-KEY> [filename]`. `<KEY>` required, `[filename]` optional (defaults to all attachments for that issue).
- **D-DL-02**: Download directory — `~/.config/waveterm/jira-attachments/<KEY>/` per existing cache contract. Use `os.MkdirAll` before write.
- **D-DL-03**: Download URL — use the attachment's `webUrl` from cache (site pattern, NOT api.atlassian.com). But for downloading programmatically, we need the API token auth. Use `GET /rest/api/3/attachment/content/<id>` with Basic auth (same as Client does). Construct URL from `cfg.BaseUrl + /rest/api/3/attachment/content/ + attachment.id`.
- **D-DL-04**: After download, update cache entry's `localPath` to the absolute path. Re-write cache atomically via `fileutil.AtomicWriteFile`. This is a targeted cache update — read existing cache, find the issue+attachment, set `localPath`, re-write.
- **D-DL-05**: Streaming write — use `io.Copy` from `resp.Body` to file. No in-memory buffer for large files.
- **D-DL-06**: RPC method — `JiraDownloadCommand(ctx, data CommandJiraDownloadData) (CommandJiraDownloadRtnData, error)`. `CommandJiraDownloadData { IssueKey string; Filename string }`. Return includes list of downloaded files with sizes. Register in `WshRpcInterface` + run `task generate`.

### Rate Limiting (D-RL-01 .. D-RL-03)

- **D-RL-01**: Token-bucket rate limiter — `golang.org/x/time/rate` (stdlib-adjacent, well-maintained). `rate.NewLimiter(10, 1)` = 10 requests/sec, burst 1. Applied via a `RateLimitedTransport` wrapping `http.DefaultTransport`. No new third-party deps needed (x/time is Go extended stdlib).
- **D-RL-02**: Injection point — `Client` struct already accepts `*http.Client` via `NewClientWithHTTP`. Wrap the transport: `cfg.HTTPClient.Transport = NewRateLimitedTransport(cfg.HTTPClient.Transport, limiter)`. Done at `NewClient` time if no custom HTTPClient provided.
- **D-RL-03**: Rate limiter default — 10 req/s per `Client` instance. Configurable via `RefreshOpts.RateLimit float64` (0 = no limit). Download commands share the same client and thus the same limiter.

### Retry Logic (D-RETRY-01 .. D-RETRY-04)

- **D-RETRY-01**: Retry scope — 429 and 5xx (500, 502, 503, 504) responses only. 4xx other than 429 are NOT retried.
- **D-RETRY-02**: 429 handling — honor `Retry-After` header (already parsed by Phase 1's `APIError.RetryAfter`). If `RetryAfter` > 60s, don't retry (treat as fatal — Atlassian is throttling hard). If `RetryAfter` = 0 or missing, use 5s default.
- **D-RETRY-03**: 5xx handling — exponential backoff: 1s, 2s, 4s (3 attempts max). Use `context.WithTimeout` per retry to respect caller's context deadline.
- **D-RETRY-04**: Implementation — `RetryTransport` wrapping the `RateLimitedTransport`. Order: `http.DefaultTransport` → `RateLimitedTransport` → `RetryTransport`. Actually, retry should be OUTSIDE rate limit: `http.DefaultTransport` → `RateLimitedTransport` (inner) → `RetryTransport` (outer). No — retry should re-acquire rate limit token. So: `RetryTransport(RateLimitedTransport(http.DefaultTransport))`. Each retry attempt goes through the rate limiter.

### Testing (D-TEST-01 .. D-TEST-04)

- **D-TEST-01**: Rate limiter — test that N requests in rapid succession are throttled (measure elapsed time vs expected floor from rate).
- **D-TEST-02**: Retry 429 — httptest returns 429 with `Retry-After: 1`, then 200. Assert only 2 requests made, total time ≥ 1s.
- **D-TEST-03**: Retry 5xx — httptest returns 500 three times, then 200. Assert 4 requests made (1 initial + 3 retries), total backoff ≈ 1+2+4 = 7s. Also test 3 failures → final error (no infinite loop).
- **D-TEST-04**: Download — httptest serves attachment content at `/rest/api/3/attachment/content/{id}`. After download, assert file exists at expected path, cache updated with localPath.

### Claude's Discretion

- Whether `RateLimitedTransport` and `RetryTransport` live in `pkg/jira/transport.go` or inline in `client.go`
- Exact backoff jitter (optional small random component to prevent thundering herd)
- Whether `wsh jira download` shows a progress bar (nice-to-have) or just prints on completion
- `golang.org/x/time/rate` vs hand-rolled token bucket (x/time preferred if module is already a dep or easily added)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pkg/jira/client.go` — `NewClientWithHTTP(cfg, httpClient)` injection point for transport wrapping
- `pkg/jira/errors.go` — `APIError` with `RetryAfter int` already parsed from 429 responses
- `pkg/jira/refresh.go` — `loadExistingLocalPaths` pattern for reading/writing cache
- `pkg/util/fileutil/fileutil.go` — `AtomicWriteFile`
- `cmd/wsh/cmd/wshcmd-jira.go` — existing `jira` parent command, add `download` subcommand
- `pkg/wshrpc/wshserver/wshserver-jira.go` — handler pattern, add `JiraDownloadCommand`

### Established Patterns
- Transport wrapping is Go idiomatic (middleware pattern)
- `golang.org/x/time` may already be in go.mod — check before adding

### Integration Points
- `WshRpcInterface` — add `JiraDownloadCommand`
- `task generate` — regenerate TS bindings after adding new RPC
- Widget may need minor update to show local preview icon when `localPath` is set (check if already implemented)

</code_context>

<specifics>
## Specific Ideas

- Attachment `webUrl` in cache uses the site pattern. For downloading with Basic auth, reconstruct URL as `baseUrl + /rest/api/3/attachment/content/ + id`. This is the same base endpoint the site pattern uses.
- Widget already handles `localPath` non-empty: shows local file icon instead of cloud-download icon (existing Phase 1 memory, line 677 of jiratasks.tsx).
- For the `wsh jira download` command output, format like: `Downloaded 2 files (1.2 MB total) → ~/.config/waveterm/jira-attachments/ITSM-3135/`.

</specifics>

<deferred>
## Deferred Ideas

- **Per-file progress bar** for large downloads — nice-to-have, not in JIRA-05 scope.
- **Concurrent downloads** for multiple attachments — sequential is fine for v1.
- **Resume interrupted downloads** — deferred.
- **Auto-download on refresh** based on file size threshold — deferred.

</deferred>

# Phase 2: Cache Orchestration - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped — ROADMAP goal + existing cache contract memory provide complete spec)

<domain>
## Phase Boundary

Combine Phase 1 primitives (`LoadConfig`, `NewClient`, `SearchIssues`, `GetIssue`, `ADFToMarkdown`) into a single refresh operation:

- Input: `jira.Config` (loaded by caller)
- Output: `~/.config/waveterm/jira-cache.json` — the exact schema the existing widget (`frontend/app/view/jiratasks/jiratasks.tsx`) already reads
- Entry point: `jira.Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error)` in `pkg/jira/refresh.go`

**Out of scope this phase:** wsh RPC plumbing, widget changes, on-demand attachment downloads, setup UX.

</domain>

<decisions>
## Implementation Decisions

### Cache Schema (D-CACHE-01 .. D-CACHE-08 — locked by existing widget consumer)

- **D-CACHE-01**: Cache path = `~/.config/waveterm/jira-cache.json` (literal, via `os.UserHomeDir() + filepath.Join`).
- **D-CACHE-02**: Top-level fields: `cloudId`, `baseUrl`, `accountId`, `fetchedAt` (ISO8601 UTC), `issues[]`.
- **D-CACHE-03**: `JiraIssue` fields: `key`, `id`, `summary`, `description`, `status`, `statusCategory` (`new`|`indeterminate`|`done`), `issueType`, `priority`, `projectKey`, `projectName`, `updated`, `created`, `webUrl`, `attachments[]`, `comments[]`, `commentCount`, `lastCommentAt`.
- **D-CACHE-04**: Description = ADF-flattened markdown via `ADFToMarkdown`. Empty if description is null.
- **D-CACHE-05**: Attachments — metadata only (id, filename, mimeType, size, `webUrl` in **site pattern** `https://<site>.atlassian.net/rest/api/3/attachment/content/<id>`, NOT `api.atlassian.com`). `localPath` defaults to empty string.
- **D-CACHE-06**: Comments kept = latest **10** (oldest dropped). Body truncated at **2000 chars**; `truncated: true` set when body exceeds. `body` = ADF-flattened markdown.
- **D-CACHE-07**: `commentCount` = total count from API (may exceed 10). `lastCommentAt` = `max(updated, created)` across **kept** comments (ISO8601, empty string if no comments kept).
- **D-CACHE-08**: Status categories map Jira's `statusCategory.key` → our 3-way: `new`, `indeterminate`, `done`. Unknown category → `new`.

### Refresh Flow (D-FLOW-01 .. D-FLOW-05)

- **D-FLOW-01**: Step 1 — call `SearchIssues` with JQL from config (`cfg.Jql`) and page size = `cfg.PageSize`. Paginate until all pages consumed. Collect issue keys only from search response (search response has limited fields).
- **D-FLOW-02**: Step 2 — for each issue key, call `GetIssue(key, fields=[…])` with the explicit field set: `summary,description,status,issuetype,priority,project,created,updated,attachment,comment`. ADF conversion happens here.
- **D-FLOW-03**: Step 3 — resolve `accountId` once per refresh: `GET /rest/api/3/myself` → capture `accountId`. (Adds one `Client` method this phase.)
- **D-FLOW-04**: Step 4 — preserve-localPath: read existing cache (if present), build `map[issueKey+attachmentId] → localPath`. When writing new cache, carry forward non-empty `localPath` values. Missing or malformed existing cache = treat as empty map (no error).
- **D-FLOW-05**: Step 5 — atomic write: marshal with `json.MarshalIndent(..., "", "  ")`, write to `jira-cache.json.tmp` in same directory, `os.Rename` to final path. `os.Rename` on Windows is atomic within same volume (acceptable since both paths are under `~/.config/waveterm/`).

### Progress Reporting (D-PROG-01 .. D-PROG-02)

- **D-PROG-01**: `RefreshOpts` includes `OnProgress func(stage string, current, total int)` callback (nillable). Stages: `"search"`, `"fetch"`, `"write"`. For `"fetch"`, current/total = issue index / total issues.
- **D-PROG-02**: `RefreshReport` returned on success: `IssueCount int`, `AttachmentCount int`, `CommentCount int` (total kept across all issues), `Elapsed time.Duration`, `CachePath string`. On error, report may be partial (nil OK if fatal before first write). This phase does NOT implement any UI — the callback is an extension point for Phase 3's RPC layer.

### Error Handling (D-ERR-01 .. D-ERR-03)

- **D-ERR-01**: Per-issue `GetIssue` failures — log via `log.Printf` (no body, no auth), continue processing remaining issues. Failed issue is **omitted** from the cache. `RefreshReport.IssueCount` reflects successfully-fetched count.
- **D-ERR-02**: Fatal errors (cannot auth, cannot write cache, search fails entirely): return wrapped error. Do NOT write partial cache file.
- **D-ERR-03**: `/myself` failure is **fatal** (need accountId for schema). No fallback.

### Concurrency (D-CONC-01)

- **D-CONC-01**: Sequential issue fetching (no goroutines). Respects JIRA rate limits; simpler error handling. Future optimization → defer to a separate milestone.

### Testing (D-TEST-01 .. D-TEST-03)

- **D-TEST-01**: Use `httptest.NewServer` fake Jira server. Provide fixture JSON for `/search/jql`, `/issue/{key}`, `/myself`. Byte-identical cache JSON comparison against golden file is the top-level assertion (ROADMAP Success #1).
- **D-TEST-02**: Preserve-localPath test — seed cache with a non-empty `localPath`, run refresh, assert it survives.
- **D-TEST-03**: Comment truncation test — comment with 2500-char body → kept body = 2000 chars, `truncated: true`. Comment cap test — 15 comments in fixture → 10 kept, `commentCount: 15`.

### Claude's Discretion
- Exact `RefreshOpts` struct field order
- Internal helper function names and organization
- Whether to split into multiple files (`refresh.go`, `cache.go`, `mapping.go`) or keep as one — planner decides based on file size

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets (from Phase 1)
- `jira.Config` (`pkg/jira/config.go`) — reads `~/.config/waveterm/jira.json`. Use `LoadConfig()` OR accept a `Config` parameter.
- `jira.Client` (`pkg/jira/client.go`) — `SearchIssues(ctx, opts)`, `GetIssue(ctx, key, opts)` already return JSON-decoded structs.
- `jira.ADFToMarkdown(raw json.RawMessage) (string, error)` — drop-in for description and comment body conversion.
- Error sentinels — `ErrUnauthorized`, `ErrNotFound`, `ErrRateLimit`, `APIError` with `RetryAfter`.

### Widget Consumer (READ-ONLY reference)
- `frontend/app/view/jiratasks/jiratasks.tsx` — already consumes the target schema. Do NOT change the widget this phase.
- Existing cache file on dev machine may have been written by the legacy Atlassian-MCP-based flow. Phase 2 must produce byte-compatible output.

### Established Patterns (Phase 1)
- Stdlib-only. No new third-party deps.
- Windows-safe: `os.UserHomeDir()` + `filepath.Join`, no POSIX hardcoded paths.
- No logging of auth headers, config values, or response bodies (T-01-02, T-01-04 threat model).
- `package jira` white-box tests (not `jira_test`).

### Integration Points
- `Client` may need a small addition: `GetMyself(ctx) (Myself, error)` hitting `/rest/api/3/myself`. Add to `client.go` as a Phase 2 extension (not a Phase 1 retrofit).

</code_context>

<specifics>
## Specific Ideas

- The existing widget at `frontend/app/view/jiratasks/jiratasks.tsx` is the authoritative schema consumer. A reader test that decodes our cache and iterates over `issues[].attachments[]` / `.comments[]` proves Success Criteria #5.
- Description ADF null vs empty-doc: Jira returns `null` description for issues without one. `ADFToMarkdown(nil)` should return `"", nil` (Phase 1 already handles this — verify in Phase 2 test).

</specifics>

<deferred>
## Deferred Ideas

- Incremental refresh (only fetch issues with `updated` > last fetch) — deferred to a future milestone. This phase does full refresh every call.
- Parallel issue fetching — deferred (D-CONC-01).
- Per-user notification counts — schema extension noted in memory, not implemented.
- Watcher flag — same.

</deferred>

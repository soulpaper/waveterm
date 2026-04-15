# Phase 1: Jira HTTP Client + Config — Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

Deliver a new Go package `pkg/jira/` that any Waveterm subsystem can call to talk to Jira Cloud:

- Config struct + loader that reads `~/.config/waveterm/jira.json`
- HTTP client (PAT via HTTP Basic) with `SearchIssues` / `GetIssue`
- Minimal ADF → markdown converter
- `httptest`-backed unit tests covering auth, happy paths, and error paths (200/401/404/429/5xx)

**Explicitly out of scope this phase:**
- Cache file writing / atomic rename (Phase 2)
- Field mapping to the `JiraIssue` schema consumed by the widget (Phase 2)
- `wsh jira refresh` / RPC / widget wire-up (Phase 3)
- Setup UX, empty state, README (Phase 4)
- Rate limiter, retry with backoff, `wsh jira download` (Phase 5)

</domain>

<decisions>
## Implementation Decisions

### API surface (Jira endpoints)

- **D-01:** Use the **enhanced search endpoint** `POST /rest/api/3/search/jql` (cursor-based, `nextPageToken`). Do NOT implement the legacy `/rest/api/3/search` (`startAt/maxResults`) — Atlassian has announced deprecation and the internal team is on Jira Cloud where enhanced search is available.
- **D-02:** `GetIssue` uses `GET /rest/api/3/issue/{key}?fields=...` and accepts a caller-provided `[]string` of field names so Phase 2 can request exactly what the cache schema needs (`summary,description,status,issuetype,priority,project,created,updated,attachment,comment`).
- **D-03:** Default page size for search = **50**. Default JQL (if config omits it) = `assignee = currentUser() ORDER BY updated DESC` — matches the memory-captured refresh contract.

### Client API shape (Go)

- **D-04:** **Options struct style**, not variadic functional options. Shape:
  ```go
  type Client struct { /* ... */ }
  func NewClient(cfg Config) *Client

  type SearchOpts struct {
      JQL           string   // required
      NextPageToken string   // "" for first page
      Fields        []string // nil/empty => server default
      MaxResults    int      // 0 => 50
  }
  func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error)

  type GetIssueOpts struct {
      Fields []string
  }
  func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error)
  ```
- **D-05:** Client embeds a private `http.Client` with a sane default timeout (**30s**). Expose a test/injection seam: `NewClientWithHTTP(cfg Config, hc *http.Client) *Client` (or an unexported field settable in-package from tests). No global singleton.
- **D-06:** Auth = `Authorization: Basic base64(email:apiToken)` on every request. Also set `Accept: application/json` and a `User-Agent: waveterm-jira/<version>`.

### Config loader

- **D-07:** Config file path = `~/.config/waveterm/jira.json` — resolve via `os.UserHomeDir()` + `.config/waveterm/jira.json` directly. **Do not** route through `wavebase.GetWaveConfigDir()` because that helper returns a platform-specific path (e.g. `%LOCALAPPDATA%/waveterm` on Windows) which is NOT what the existing widget and memory-locked schema use. The literal `~/.config/waveterm/` path is the contract.
- **D-08:** Config struct fields (all JSON-tagged):
  - `BaseUrl` (e.g. `https://kakaovx.atlassian.net`) — required
  - `CloudId` — required for now (kakaovx = `280eeb13-4c6a-4dc3-aec5-c5f9385c7a7d`); used in cache file output, not necessarily in REST URL
  - `Email` — required
  - `ApiToken` — required
  - `Jql` — optional; default = `assignee = currentUser() ORDER BY updated DESC`
  - `PageSize` — optional; default 50
- **D-09:** Missing / unreadable config file ⇒ return a **typed sentinel error** `ErrConfigNotFound` so Phase 4's empty-state UX can distinguish "no config yet" from "config exists but bad token".
- **D-10:** Malformed JSON ⇒ `ErrConfigInvalid` (wraps the underlying `json.Unmarshal` error).
- **D-11:** Validation after load: missing `BaseUrl` / `Email` / `ApiToken` ⇒ `ErrConfigIncomplete` (names the missing fields in its message). Defaults fill `Jql` / `PageSize` silently.
- **D-12:** Loader re-reads on every call (no in-process caching). Milestone requirement: config edits take effect without restart.

### ADF → markdown converter

- **D-13:** Output format = **markdown** (the widget already renders markdown in expanded cards).
- **D-14:** Supported ADF nodes in this phase:
  - `doc`, `paragraph`, `hardBreak`
  - `heading` (levels 1–6) ⇒ `#`..`######`
  - `bulletList` / `listItem` ⇒ `- `
  - `orderedList` / `listItem` ⇒ `1. `, `2. `, ...
  - `codeBlock` ⇒ fenced ` ``` ` (language attr preserved when present)
  - `text` with marks: `strong` ⇒ `**x**`, `em` ⇒ `*x*`, `code` ⇒ `` `x` ``, `link` ⇒ `[x](href)`
  - `mention` ⇒ `@displayName` (fallback to `@accountId` if no `text` attr)
  - `rule` ⇒ `---`
  - `blockquote` ⇒ `> `
  - `table` / `tableRow` / `tableHeader` / `tableCell` ⇒ GFM pipe table (header row detected from `tableHeader` presence)
- **D-15:** Unknown / unsupported node types (media, panel, inlineCard, emoji, etc.) ⇒ **silently skip** the node (descend into children if present, render any `text` found; otherwise drop). Emit a `log.Printf` at debug level listing the unknown type — not an error return. Rationale: we'd rather show partial content than refuse.
- **D-16:** Entry point: `func ADFToMarkdown(raw json.RawMessage) (string, error)`. Error only on structural JSON parse failure, never on unknown node types.
- **D-17:** The converter handles both **issue description** ADF and **comment body** ADF — same function.

### Error typing

- **D-18:** Expose **both** sentinel errors AND a typed struct — callers can pick the granularity they need:
  ```go
  var (
      ErrUnauthorized = errors.New("jira: unauthorized")      // 401
      ErrForbidden    = errors.New("jira: forbidden")         // 403
      ErrNotFound     = errors.New("jira: not found")         // 404
      ErrRateLimited  = errors.New("jira: rate limited")      // 429
      ErrServerError  = errors.New("jira: server error")      // 5xx

      ErrConfigNotFound   = errors.New("jira: config not found")
      ErrConfigInvalid    = errors.New("jira: config invalid")
      ErrConfigIncomplete = errors.New("jira: config incomplete")
  )

  type APIError struct {
      StatusCode int
      Endpoint   string
      Method     string
      Body       string // truncated, up to 1KB
  }
  func (e *APIError) Error() string
  func (e *APIError) Unwrap() error // returns matching sentinel by status bucket
  ```
- **D-19:** Every non-2xx response from Jira is wrapped into an `*APIError` whose `Unwrap()` returns the matching sentinel. Callers use `errors.Is(err, jira.ErrUnauthorized)` for class checks and `errors.As(err, &apiErr)` when they need the status code or response body (e.g. for logging).
- **D-20:** `Retry-After` header value is preserved on the `APIError` struct (field `RetryAfter time.Duration`) when `StatusCode == 429` — Phase 5's rate limiter will consume it; Phase 1 itself does not retry.

### Tests

- **D-21:** Use `net/http/httptest.NewServer` to stub Jira. First use of this pattern in `pkg/`; follow standard Go idioms (no new test framework).
- **D-22:** Test coverage required for Success Criteria sign-off:
  - SearchIssues 200 + pagination (first page, second page via `nextPageToken`)
  - GetIssue 200 with full fields present
  - Authorization header correctness (Basic `email:token` exactly)
  - 401 path ⇒ `errors.Is(err, ErrUnauthorized)`
  - 404 path ⇒ `errors.Is(err, ErrNotFound)`
  - 429 path ⇒ `errors.Is(err, ErrRateLimited)` AND `APIError.RetryAfter` populated from header
  - 5xx path ⇒ `errors.Is(err, ErrServerError)`
  - Config loader: happy, missing file, malformed JSON, partial (defaults fill), incomplete (required missing)
  - ADF: table-driven test per supported node type + one "unknown node in mixed tree" case (should not fail, should render surrounding text)
- **D-23:** All tests must run green on **Windows** (the primary platform). No unix-only syscalls, path separators, or shell invocations.

### Claude's Discretion

These are fine for me to decide during planning/execution without further input:

- Internal file split within `pkg/jira/` (e.g., whether `APIError` lives in `errors.go` or inline in `client.go`)
- Exact field names on `SearchResult` / `Issue` structs (just reflect Jira's JSON shape — Phase 2 will own the mapping to `JiraIssue`)
- Whether ADF converter uses a struct-based visitor or a recursive `map[string]any` walk (whichever is simpler given the small node set)
- Go module version / package-level `log` vs `slog` — match whatever the rest of `pkg/` uses

### Folded Todos

None — no outstanding todos matched Phase 1 scope.

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents (researcher, planner, executor) MUST read these before acting.**

### Project-level specs
- `.planning/PROJECT.md` — Milestone vision, non-goals, Key Decisions table (PAT over OAuth, plain JSON config, Windows-primary)
- `.planning/REQUIREMENTS.md` — `JIRA-01` (HTTP request with PAT) and `JIRA-02` (config file semantics) are the requirements this phase closes
- `.planning/ROADMAP.md` §"Phase 1" — success criteria 1–4

### Existing contract (do not break)
- `frontend/app/view/jiratasks/jiratasks.tsx` — current cache consumer; defines the `JiraIssue` shape Phase 2 will populate. Phase 1's HTTP responses must be parseable into something that can later be mapped to this shape.
- User auto-memory `project_jira_cache_contract.md` (in `~/.claude/projects/F--waveterm/memory/`) — schema of `~/.config/waveterm/jira-cache.json`, refresh contract, attachment `webUrl` site-pattern requirement. Treat this as authoritative for the data contract even though it's outside the repo.

### Reference code patterns (read before writing similar code)
- `pkg/waveai/anthropicbackend.go` — existing pattern for an HTTP-calling Go service in this codebase (`http.Client` usage, JSON decode, error handling)
- `pkg/wconfig/settingsconfig.go` — existing pattern for loading a JSON config file with defaults
- `pkg/wavebase/wavebase.go` — home dir helpers. **Note:** `GetWaveConfigDir()` is NOT the path we want here (it's platform-specific); this phase uses `os.UserHomeDir() + /.config/waveterm/` literally per D-07.

### External (Atlassian)
- Jira Cloud REST API v3 index — https://developer.atlassian.com/cloud/jira/platform/rest/v3/
- Enhanced search (cursor pagination) — `POST /rest/api/3/search/jql` — https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-post
- Get issue — `GET /rest/api/3/issue/{issueIdOrKey}` — https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-get
- Basic auth with API tokens — https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/
- ADF node spec — https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/
- Rate limits (`Retry-After`) — https://developer.atlassian.com/cloud/jira/platform/rate-limiting/

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pkg/waveai/anthropicbackend.go` — real-world example of `http.Client`, `http.NewRequestWithContext`, JSON decoding, and error categorization in this codebase. Mirror its style for consistency.
- `pkg/wconfig/settingsconfig.go` — shows the project's convention for JSON config structs and decode-with-defaults.
- `pkg/panichandler` — used by other long-running goroutines; Phase 1 is synchronous and probably doesn't need it, but Phase 2 will.

### Established Patterns
- Go stdlib-first. No heavyweight HTTP client libraries currently in `pkg/` (no resty/retryablehttp). Keep it that way.
- Tests live alongside the package as `*_test.go`. Table-driven tests are the common style (see `pkg/gogen/gogen_test.go`, `pkg/ijson/ijson_test.go`).
- `httptest.NewServer` is not yet used anywhere in `pkg/` — Phase 1 introduces it. Follow the standard library example.

### Integration Points
- Phase 2 (`pkg/jira/refresh.go`) will import this package and call `Client.SearchIssues` + `Client.GetIssue` repeatedly.
- Phase 3 (wsh RPC) will import the same client via Phase 2's refresh entry point.
- `pkg/jira/` must be importable with no side effects beyond struct construction — no `init()` doing network I/O.

</code_context>

<specifics>
## Specific Ideas

- User explicitly approved "내 추천대로 다 가자" — all four gray areas resolved via the recommended defaults. Decisions above reflect those recommendations expanded into implementation-level detail.
- The cache file path in the memory (`~/.config/waveterm/jira.json` and `~/.config/waveterm/jira-cache.json`) is the contract. Do not "normalize" it to `GetWaveConfigDir()` — it is deliberately different from the Waveterm config dir.

</specifics>

<deferred>
## Deferred Ideas

- **Retry / backoff on 429 and 5xx** — belongs in Phase 5 (JIRA-10). Phase 1 only surfaces the `Retry-After` value on `APIError`; it does not retry.
- **Rate limiter (token bucket, 10 req/s)** — Phase 5.
- **Attachment download (`wsh jira download <KEY>`)** — Phase 5 (JIRA-05). Phase 1 does not expose any attachment fetch method; Phase 2 will carry attachment metadata only.
- **Comment fetching strategy (keep latest 10, truncate 2000)** — that transformation belongs to Phase 2's refresh orchestration. Phase 1's `GetIssue` returns whatever the Jira API returns for the comment field the caller requests.
- **Jira Server / Data Center** — explicitly out of milestone scope.
- **OAuth / PKCE, Electron safeStorage, React settings modal** — milestone non-goals, tracked as `JIRA-F-01..03`.

### Reviewed Todos (not folded)
None — no todos matched Phase 1.

</deferred>

---

*Phase: 01-jira-http-client-config*
*Context gathered: 2026-04-15*

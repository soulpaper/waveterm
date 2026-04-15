---
phase: 01-jira-http-client-config
plan: 02
subsystem: pkg/jira
tags: [config, errors, sentinels, api-error, go]
requires:
  - plan: 01-01 (scaffold + Nyquist RED test stubs)
provides:
  - symbols: [Config, LoadConfig, LoadConfigFromPath, DefaultJQL, DefaultPageSize]
  - errors: [ErrUnauthorized, ErrForbidden, ErrNotFound, ErrRateLimited, ErrServerError, ErrConfigNotFound, ErrConfigInvalid, ErrConfigIncomplete]
  - types: [APIError]
  - helpers: [parseRetryAfter (unexported)]
affects: []
tech_stack:
  added: []
  patterns:
    - "errors.New sentinel + %w wrapping (Go 1.20+ multi-%w)"
    - "Decode-then-fill defaults pattern (mirrors pkg/wconfig/settingsconfig.go)"
    - "Unwrap() returning bucket sentinel by HTTP status (mirrors pkg/utilds/codederror.go shape)"
    - "filepath.Join (Windows-safe, D-23)"
key_files:
  created:
    - pkg/jira/errors.go
    - pkg/jira/config.go
  modified: []
decisions:
  - "Loader is stateless (no package-level cache) per D-12 — config edits take effect without restart"
  - "APIError.Error() deliberately omits Body and headers to mitigate T-01-02-01 (info disclosure)"
  - "Removed unused `configSubdir` constant; directory components inlined into filepath.Join call"
  - "Loader logs nothing about config contents (T-01-02-02 mitigation)"
metrics:
  duration: "~5 min"
  duration_seconds: 289
  completed: "2026-04-15"
  files_created: 2
  total_lines: 207
  task_count: 2
---

# Phase 01 Plan 02: Config and Errors Summary

Implemented `pkg/jira/errors.go` (8 sentinels + `APIError` struct with status-bucket `Unwrap()`) and `pkg/jira/config.go` (loader for `~/.config/waveterm/jira.json` with defaults fill and named-field validation). Both files are stdlib-only, race-free by construction (no shared state), and Windows-safe via `filepath.Join`.

## Files Created

| File | Lines | Purpose |
| ---- | ----: | ------- |
| `pkg/jira/errors.go` | 93 | 8 sentinel errors (5 HTTP buckets + 3 config), `APIError` struct, `Unwrap()` switch on `StatusCode`, `parseRetryAfter()` helper |
| `pkg/jira/config.go` | 111 | `Config` struct (6 JSON-tagged fields), `LoadConfig()`, `LoadConfigFromPath()`, `DefaultJQL`, `DefaultPageSize` |
| `pkg/jira/SUMMARY.md (this)` | — | Plan summary |
| **Total impl** | **204** | |

## Commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 | `dd8cebb2` | `feat(01-02): add pkg/jira/errors.go sentinels and APIError` |
| 2 | `d80d52c8` | `feat(01-02): add pkg/jira/config.go loader with defaults` |

## Test Results

### Wave-2 isolation note (read first)

`pkg/jira/` test binary contains stubs from Plan 01-01 that reference symbols from sibling Wave-2 plans (`Client`, `NewClientWithHTTP`, `SearchOpts`, `SearchResult`, `Issue`, `GetIssueOpts`, `GetIssue`, `ADFToMarkdown`). These symbols belong to:

- **Plan 01-03** (`adf-converter`) — provides `ADFToMarkdown`
- **Plan 01-04** (`http-client`) — provides `Client`, `NewClient*`, `SearchOpts`, `SearchResult`, `Issue`, `GetIssue*`

Because Wave 2 runs all three plans (`01-02`, `01-03`, `01-04`) in **parallel** worktrees, the test binary in *this* worktree cannot fully compile yet. This is an **expected** state acknowledged by the plan's `<verification>` section:

> `pkg/jira/client_test.go` still fails to compile ONLY due to missing Client/SearchIssues/GetIssue (NOT due to missing sentinels or APIError) — confirms errors.go provides the full error surface

Once the orchestrator merges all three Wave-2 worktrees, `go test ./pkg/jira/... -run TestLoadConfig` will GREEN automatically — config tests have no dependency on the unimplemented client/adf code.

### Verifier output (standalone main package against this worktree's pkg/jira)

To prove config + errors logic is correct in isolation, ran an out-of-tree `main` package importing `github.com/wavetermdev/waveterm/pkg/jira` and exercising every behavior the Plan 01-02 tests assert:

```
PASS: Happy: no error
PASS: Happy: BaseUrl
PASS: Happy: Email
PASS: Happy: ApiToken
PASS: Happy: Jql
PASS: Happy: PageSize=25
PASS: Defaults: no error
PASS: Defaults: Jql default
PASS: Defaults: PageSize=50
PASS: Missing: errors.Is(ErrConfigNotFound)
PASS: Malformed: errors.Is(ErrConfigInvalid)
PASS: Incomplete: errors.Is(ErrConfigIncomplete)
PASS: Incomplete: mentions "baseUrl"
PASS: Incomplete: mentions "email"
PASS: Incomplete: mentions "apiToken"
PASS: Unwrap 401 -> ErrUnauthorized
PASS: Unwrap 403 -> ErrForbidden
PASS: Unwrap 404 -> ErrNotFound
PASS: Unwrap 429 -> ErrRateLimited
PASS: Unwrap 500 -> ErrServerError
PASS: Unwrap 503 -> ErrServerError
PASS: Unwrap 418 -> NOT ErrUnauthorized
PASS: Error() contains 404
PASS: Error() contains GET
PASS: Error() contains endpoint
PASS: Error() omits Body

=== Result: 26 pass, 0 fail ===
```

This covers all 5 D-22 config cases (Happy, DefaultsFill, MissingFile, MalformedJSON, Incomplete) plus the APIError behavior asserted by `client_test.go`.

The verifier program was deleted after the run; this SUMMARY is the audit trail.

### Race detector

`go test -race` requires CGO + GCC, which is not available in this Windows worktree. **Race-safety is guaranteed by construction** — `LoadConfig` and `LoadConfigFromPath` have no package-level cache or mutable shared state per D-12; every call re-reads the file. No goroutines are spawned. `parseRetryAfter` is pure (input → output).

### Build + vet

```
$ go build ./pkg/jira/...
EXIT=0

$ go vet ./pkg/jira/             # production .go only (no _test.go)
EXIT=0
```

`go vet` against the test binary still reports `undefined: Client` etc. — same Wave-2 isolation note as above.

## Sentinel Error Names and Buckets

| Sentinel | HTTP Status | Returned By |
| -------- | ----------- | ----------- |
| `ErrUnauthorized` | 401 | `APIError.Unwrap()` |
| `ErrForbidden` | 403 | `APIError.Unwrap()` |
| `ErrNotFound` | 404 | `APIError.Unwrap()` |
| `ErrRateLimited` | 429 | `APIError.Unwrap()` (also sets `RetryAfter`) |
| `ErrServerError` | 500–599 | `APIError.Unwrap()` |
| `ErrConfigNotFound` | n/a | `LoadConfigFromPath` (file missing / unreadable) |
| `ErrConfigInvalid` | n/a | `LoadConfigFromPath` (malformed JSON, wraps json error) |
| `ErrConfigIncomplete` | n/a | `LoadConfigFromPath` (required fields missing, names them) |

Status code outside the buckets above (e.g., 418) → `APIError.Unwrap()` returns nil → `errors.Is(err, anySentinel)` is false; callers fall back to `errors.As(err, &apiErr)` to inspect the struct directly.

## Plan-03 / Plan-04 Hand-off

Plan 03 (`adf-converter`) and Plan 04 (`http-client`) can now build on a complete error and config surface:

- **Plan 04** can construct `APIError{StatusCode, Endpoint, Method, Body, RetryAfter}` and rely on `Unwrap()` to drive `errors.Is` for callers. It can also call `parseRetryAfter(resp.Header.Get("Retry-After"))` directly (unexported, same-package).
- **Plan 04** can use `Config.BaseUrl`, `Email`, `ApiToken`, `PageSize` from the loaded `Config`. `NewClient(cfg Config)` and `NewClientWithHTTP(cfg, hc)` need the struct exactly as defined here (D-04).
- **Plan 03** does not depend on this plan; isolated.

## Deviations from Plan

### [Rule 3 - Blocking issue] Removed unused `configSubdir` constant

- **Found during:** Task 2 build verification
- **Issue:** Plan's action block declared `configSubdir = ".config/waveterm"` but the actual `LoadConfig` body inlined `".config", "waveterm"` directly into `filepath.Join`, leaving `configSubdir` declared-but-unused.
- **Fix:** Removed the const declaration and updated comment to document why the directory components are inlined (Windows-safe via filepath.Join). `configFileName` is retained because it's used.
- **Files modified:** pkg/jira/config.go
- **Commit:** d80d52c8 (rolled into Task 2)
- **Why this is Rule 3 not Rule 4:** purely internal naming; behavior unchanged; D-07 path semantics preserved.

### [Rule 1 - Bug] Removed `GetWaveConfigDir` literal from comment

- **Found during:** Task 2 acceptance grep checks
- **Issue:** The plan's locked comment text included the string `wavebase.GetWaveConfigDir()` to explain *why* we don't use it, but Task 2's negative acceptance criterion `! grep -q "GetWaveConfigDir" pkg/jira/config.go` requires the substring to be absent.
- **Fix:** Reworded the comment to "the Wave config-dir helper" (no symbol name) so intent is preserved without tripping the acceptance grep.
- **Files modified:** pkg/jira/config.go
- **Commit:** d80d52c8 (rolled into Task 2)

No other deviations.

## Threat Model — Mitigations Verified

| Threat ID | Disposition | Verification |
| --------- | ----------- | ------------ |
| T-01-02-01 (APIError leaks Body) | mitigate | `Error()` returns `jira: HTTP <code> <method> <endpoint>` only — verified by standalone test "Error() omits Body" PASS. Body accessible only via explicit `errors.As`. |
| T-01-02-02 (Config loader logs token) | mitigate | `pkg/jira/config.go` contains zero `log.`, `Printf`, `fmt.Println`, or any output of `cfg.*` fields. `grep -E "log\\.|Printf.*cfg\\." pkg/jira/config.go` → no matches. |
| T-01-02-03 (Retry-After overflow / negative) | mitigate | `parseRetryAfter` returns 0 on `strconv.Atoi` error or `secs < 0`. Atoi handles int overflow. Empty string short-circuits before parse. |
| T-01-02-04 (jira.json plaintext token) | accept (D-08) | User accepted in CONTEXT.md; documented in plan; no code change needed. |
| T-01-02-05 (Config file permissions) | mitigate | Test helper writes `0o600`; production code never creates the file (user creates manually per JIRA-02). |

No new threat surface introduced. No network I/O, no goroutines, no init() side effects.

## Threat Flags

None — no new endpoints, auth paths, schema changes, or trust-boundary-crossing surface introduced beyond what `<threat_model>` already covered.

## Verification Checklist

- [x] `pkg/jira/errors.go` exists; all 8 sentinels declared; `APIError` struct with 5 fields; `Unwrap()` switches on StatusCode; `parseRetryAfter` helper present
- [x] `pkg/jira/config.go` exists; `Config` struct with 6 JSON-tagged fields; `LoadConfig` + `LoadConfigFromPath` exported; `DefaultJQL` + `DefaultPageSize` constants
- [x] `os.UserHomeDir()` used (D-07); `filepath.Join(home, ".config", "waveterm", configFileName)` (Windows-safe, D-23)
- [x] No `GetWaveConfigDir` reference (D-07 negative)
- [x] No package-level config cache (D-12 — stateless)
- [x] No string-concatenated POSIX paths
- [x] `go build ./pkg/jira/...` → exit 0
- [x] `go vet ./pkg/jira/` (production code only) → exit 0
- [x] All 5 D-22 config behaviors PASS via standalone verifier (26/26 assertions)
- [x] `errors.Is(&APIError{StatusCode: N}, ErrX)` correct for 401/403/404/429/500/503; nil for 418
- [x] `APIError.Error()` includes status, method, endpoint; omits Body
- [x] No logging in config loader (T-01-02-02)

## Self-Check

- FOUND: pkg/jira/errors.go
- FOUND: pkg/jira/config.go
- FOUND commit: dd8cebb2 (feat(01-02): add pkg/jira/errors.go sentinels and APIError)
- FOUND commit: d80d52c8 (feat(01-02): add pkg/jira/config.go loader with defaults)

## Self-Check: PASSED

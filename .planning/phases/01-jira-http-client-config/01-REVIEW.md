---
phase: 01-jira-http-client-config
reviewed: 2026-04-15T00:00:00Z
depth: standard
files_reviewed: 8
files_reviewed_list:
  - pkg/jira/doc.go
  - pkg/jira/config.go
  - pkg/jira/errors.go
  - pkg/jira/adf.go
  - pkg/jira/client.go
  - pkg/jira/config_test.go
  - pkg/jira/adf_test.go
  - pkg/jira/client_test.go
findings:
  critical: 0
  warning: 4
  info: 5
  total: 9
status: issues_found
---

# Phase 01: Code Review Report

**Reviewed:** 2026-04-15
**Depth:** standard
**Files Reviewed:** 8
**Status:** issues_found

## Summary

Overall the Phase 01 implementation is well-structured, cleanly documented, and faithful to the phase plan decisions (D-01 through D-23). Security-sensitive decisions are handled correctly: the Authorization header is never logged, APIError.Error() deliberately omits the response body, error body reads are capped at 1 KB via `io.LimitReader`, and LoadConfigFromPath does not log file contents.

No Critical security or correctness issues were found. The findings below are primarily robustness improvements: a real-but-low-probability bug in the test request-body read, an edge case around trailing slashes in `BaseUrl`, a code-fence-collision hazard in the ADF code-block renderer, and a few code-quality cleanups.

All test files correctly use `httptest.NewServer` and `t.TempDir()` — no hardcoded credentials or leaked state.

## Warnings

### WR-01: Test body read is truncated by single `Read` call

**File:** `pkg/jira/client_test.go:117-119`
**Issue:** The handler uses `r.Body.Read(buf)` which, per `io.Reader` semantics, is not guaranteed to return the full body in a single call — especially for chunked transfers or when the runtime decides to deliver the body in multiple network reads. Today the JSON body is small and this usually succeeds, but it can cause flaky test failures and the assertion loop will report "missing field X" rather than "partial read." The test is already checking pagination token + fields + maxResults, so a partial read would be a misleading failure.
**Fix:**
```go
// Replace:
buf := make([]byte, 4096)
n, _ := r.Body.Read(buf)
bodyStr = string(buf[:n])

// With:
b, err := io.ReadAll(r.Body)
if err != nil {
    t.Fatalf("read request body: %v", err)
}
bodyStr = string(b)
```
(Requires adding `"io"` to the test imports.)

### WR-02: `BaseUrl` with trailing slash produces double-slash endpoint path

**File:** `pkg/jira/client.go:192, 216`
**Issue:** Both `SearchIssues` and `GetIssue` build the endpoint with `c.cfg.BaseUrl + "/rest/api/3/..."`. If a user writes `"baseUrl": "https://kakaovx.atlassian.net/"` in jira.json (a very natural input), the resulting URL becomes `https://kakaovx.atlassian.net//rest/api/3/...`. Atlassian's gateway may 404 or redirect on the doubled slash, and the failure mode is confusing because the config validates successfully. LoadConfigFromPath should normalize or the client should strip.
**Fix:** Trim the trailing slash once, either in `LoadConfigFromPath` right after defaults are filled:
```go
cfg.BaseUrl = strings.TrimRight(cfg.BaseUrl, "/")
```
…or, if you prefer to keep the config value untouched, normalize in `NewClient` / `NewClientWithHTTP` before storing the snapshot. Either location is fine; document the choice.

### WR-03: Code-block rendering can emit malformed markdown when source text contains triple backticks

**File:** `pkg/jira/adf.go:112-125`
**Issue:** The ADF `codeBlock` handler always fences with three backticks. If the embedded text contains ``` ``` ``` (e.g. a markdown snippet someone pasted into Jira), the output will terminate the fence prematurely and render the remainder as body text. For an internal-tool Jira cache this is low-severity but non-trivial to debug when it hits.
**Fix:** Detect the longest run of backticks in the text and use one more:
```go
fence := "```"
if max := maxBacktickRun(textContent); max >= 3 {
    fence = strings.Repeat("`", max+1)
}
sb.WriteString(fence)
sb.WriteString(lang)
sb.WriteString("\n")
// ... write text ...
sb.WriteString("\n")
sb.WriteString(fence)
sb.WriteString("\n\n")
```
Where `maxBacktickRun` scans the string once for the longest `` ` `` run. Alternatively, if fidelity to markdown is low priority, at least document the limitation in the package comment.

### WR-04: Permission-denied on jira.json is reported as `ErrConfigNotFound`

**File:** `pkg/jira/config.go:69-77`
**Issue:** The loader collapses every `os.ReadFile` failure that is not a "does not exist" into `fmt.Errorf("%w: %v", ErrConfigNotFound, err)`. That means a real permission issue (e.g. the file exists but is mode 0o000, or SELinux/AppArmor is blocking) is surfaced to the user as "config not found," which will mis-direct them to "create the file" when the file already exists. The code comment acknowledges this is intentional for Phase 4 empty-state UX, but the fallback case is inverted from the docstring: the docstring says `ErrConfigNotFound` is for "file does not exist," and then the code maps permission-denied to the same sentinel.
**Fix:** Either (a) introduce a distinct `ErrConfigUnreadable` sentinel for non-existent vs unreadable — Phase 4 can still treat them the same for empty-state UX but the error chain carries the truth, or (b) update the docstring on `ErrConfigNotFound` to say "file does not exist OR is unreadable" so the contract matches behavior. Option (a) is preferred because the wrapped `%v` loses the original `err` from `errors.Is/As` introspection (it's wrapped with `%v`, not `%w`).

## Info

### IN-01: `log.Printf` in ADF converter uses package default logger and can spam

**File:** `pkg/jira/adf.go:315-317`
**Issue:** `logUnknownADFType` emits one line per unknown node per conversion. A single complex issue with 50 panels/media/emoji nodes produces 50 log lines per refresh. The comment explicitly accepts this trade-off, but for an app that refreshes on a schedule this grows quickly in the Waveterm log.
**Fix:** Consider one of:
- Aggregate unknown types into a single summary line per `ADFToMarkdown` call: `"jira: ADF converter skipped N unknown nodes: [panel×12, emoji×3]"`.
- Log only the first N unique types per process (package-level `sync.Map` with small overhead — you noted mutex cost; `sync.Map` is cheap for write-once-read-many).
- Leave as-is and document the log volume expectation in the package doc.

### IN-02: `config_test.go` duplicates `strings.Contains` as `stringsContains`

**File:** `pkg/jira/config_test.go:107-117`
**Issue:** The comment says "Replace with strings.Contains if preferred during implementation." The package is already test-only; just use `strings.Contains` to reduce surface area. The hand-rolled version is O(n*m) vs the stdlib's more optimized search, though for test use sizes this is immaterial. Correctness nit: the hand-rolled version handles empty `sub` correctly (loop condition `i+0 <= len(s)` always true on iter 0, returns true), matching `strings.Contains` semantics, so the swap is safe.
**Fix:**
```go
import "strings"
// ...
if !strings.Contains(msg, field) { ... }
```
Delete `stringsContains`.

### IN-03: `searchRequest.MaxResults` is always sent, even when defaulted

**File:** `pkg/jira/client.go:166-171, 177-186`
**Issue:** The JSON tag is `"maxResults"` with no `omitempty`, and the zero-to-default substitution happens inside `SearchIssues`, so the server always receives `"maxResults": 50` even when the caller passed `0`. This is fine and arguably desirable (explicit over implicit), but it conflicts with the comment on `SearchOpts.MaxResults` that says `0 → server default`. The code actually sends 50 always — the "server default" is moot because we never omit the field.
**Fix:** Either document the actual behavior on `SearchOpts` ("0 → DefaultPageSize (50), always sent"), or add `omitempty` and let the server apply its own default when `MaxResults == 0`. The first option is less risky.

### IN-04: APIError body truncation silently drops read errors

**File:** `pkg/jira/client.go:256`
**Issue:** `body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))` discards the error. If the connection drops mid-body read, `APIError.Body` ends up as a partial / empty string and the caller has no indication the truncation was due to a read failure vs an empty server response. Not exploitable, but hurts debugging on flaky networks.
**Fix:** Capture the error and, if non-nil, prepend a marker to Body:
```go
body, rerr := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
if rerr != nil {
    body = append(body, []byte(fmt.Sprintf("\n[body read error: %v]", rerr))...)
}
```
The marker is safe to include in Body because Body is never returned by `APIError.Error()` (T-01-02-01 is preserved).

### IN-05: Empty blockquote produces a stray `> ` line

**File:** `pkg/jira/adf.go:128-141`
**Issue:** When a `blockquote` has empty `content` (or all whitespace), `inner.String()` is empty, `strings.TrimRight(..., "\n")` returns `""`, and `strings.Split("", "\n")` returns `[""]` — producing a single `> \n` line in the output. Cosmetic, not wrong, but worth a one-line guard.
**Fix:**
```go
trimmed := strings.TrimRight(inner.String(), "\n")
if trimmed == "" {
    sb.WriteString("\n")
    return
}
for _, line := range strings.Split(trimmed, "\n") { ... }
```

---

_Reviewed: 2026-04-15_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_

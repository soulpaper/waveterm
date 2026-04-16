---
phase: 02-cache-orchestration
reviewed: 2026-04-15T00:00:00Z
depth: standard
files_reviewed: 10
files_reviewed_list:
  - pkg/jira/refresh.go
  - pkg/jira/cache_types.go
  - pkg/jira/client.go
  - pkg/jira/refresh_test.go
  - pkg/jira/testdata/cache.golden.json
  - pkg/jira/testdata/cache-with-localpath.seed.json
  - pkg/jira/testdata/issue-itsm1.json
  - pkg/jira/testdata/issue-comment-cap.json
  - pkg/jira/testdata/issue-comment-truncation.json
  - pkg/jira/testdata/issue-null-description.json
findings:
  critical: 0
  warning: 2
  info: 4
  total: 6
status: fixed
fix_report: 02-REVIEW-FIX.md
---

# Phase 2: Code Review Report

**Reviewed:** 2026-04-15T00:00:00Z
**Depth:** standard
**Files Reviewed:** 10
**Status:** issues_found

## Summary

Phase 2 delivers a clean orchestration layer. The file layout, error semantics,
and schema shape match the spec in `02-CONTEXT.md` + `02-RESEARCH.md` precisely.
The six explicit concerns raised in the brief all check out:

- **Secrets safety (T-01-02/04):** `APIError.Error()` omits Body by design
  (`pkg/jira/errors.go:50`), and every `fmt.Errorf` in `refresh.go` uses `%v`
  (never `%w`), so credentials/response bodies cannot escape via `errors.Unwrap`
  chains. The per-issue failure log at `refresh.go:125` uses `%v` on the
  APIError, logging only `HTTP <code> <METHOD> <path>`. Clean.
- **Comment ordering (Pitfall 2):** `refresh.go:224-226` keeps `raw[len(raw)-10:]`
  — i.e., the LAST 10 of an oldest-first wire slice. `TestRefresh_CommentCapAndTruncation`
  asserts c06..c15 are kept (c01..c05 dropped). Correct.
- **Author flatten (Pitfall 1):** `refresh.go:241-244` collapses `Author.DisplayName`
  (with `AccountID` fallback) into a single string, matching the widget's
  `c.author: string` expectation in `cache_types.go:75-82`. Correct.
- **webUrl passthrough (D-CACHE-05 / A3):** `refresh.go:217` passes the wire
  `attachment.content` through unchanged; `TestRefresh_AttachmentWebUrlPassthrough`
  locks the exact bytes `https://example.atlassian.net/rest/api/3/attachment/content/att1`.
  Correct.
- **MkdirAll before AtomicWriteFile (Pitfall 5):** `refresh.go:147-152` performs
  `os.MkdirAll(filepath.Dir(cachePath), 0o755)` immediately before the atomic
  write. Fresh-install scenario is covered by `setFakeHome()` tests that
  deliberately do NOT pre-create `~/.config/waveterm/`.
- **Atomic write on Windows:** `fileutil.AtomicWriteFile` uses `os.WriteFile`
  + `os.Rename` to the same directory. On Windows, `os.Rename` maps to
  `MoveFileEx` which is atomic for same-volume renames. Since both source and
  target sit in `~/.config/waveterm/`, the rename is safe. Partial writes
  leave `.tmp` orphans (not the live cache) — acceptable, callers never read
  `.tmp`.

Findings below are a mix of one real correctness concern (UTF-8 truncation)
and polish items.

## Warnings

### WR-01: Comment body truncation can produce invalid UTF-8 (mid-rune slice)

**File:** `pkg/jira/refresh.go:236-239`
**Issue:** `body = body[:2000]` truncates on byte boundaries, not rune
boundaries. When a comment body contains multi-byte UTF-8 (Korean/CJK/emoji —
common given this is a Korean user's codebase, and the Jira content is typical
corporate text), the final 1-3 bytes may fall inside a rune, producing invalid
UTF-8. Consequences:

1. Go's `encoding/json` encoder converts invalid UTF-8 bytes to the
   replacement character U+FFFD (or escapes them), silently corrupting the
   tail of the body.
2. The widget (React/Electron) then renders a garbage glyph at the cut.
3. Byte-identical golden-file tests written with ASCII fixtures will pass
   while this bug lives on for real users.

**Fix:** Truncate on a valid rune boundary:

```go
if len(body) > 2000 {
    // Walk back from byte 2000 to the start of the rune that overruns the cap.
    cut := 2000
    for cut > 0 && !utf8.RuneStart(body[cut]) {
        cut--
    }
    body = body[:cut]
    truncated = true
}
```

Add `"unicode/utf8"` to the import block. Consider also adding a test fixture
with a 2001-byte body built from 3-byte runes (e.g., Korean "한" × N) to
pin the invariant.

### WR-02: Cache file written with world-readable mode 0o644

**File:** `pkg/jira/refresh.go:150`
**Issue:** `fileutil.AtomicWriteFile(cachePath, data, 0o644)` makes the cache
group/other-readable on POSIX. The cache contains:

- `accountId` (identifier, not a secret, but PII-adjacent)
- Every issue summary, description, and comment body the user can see in Jira
- Comment author display names

On a shared workstation this is readable by any local user. The paired
`config.json` (which holds the API token) is written `0o600` by the Phase 1
config loader (`config_test.go:19` verifies). The cache should match.

**Fix:**

```go
if err := fileutil.AtomicWriteFile(cachePath, data, 0o600); err != nil {
    return nil, fmt.Errorf("jira refresh: write cache: %v", err)
}
```

No test will break — the test server runs as the same user that reads the
file. Update `02-CONTEXT.md` D-CACHE-01 note if the mode is explicit there.

## Info

### IN-01: `GetMyself` happens before `cacheFilePath()` — ordering is slightly wasteful

**File:** `pkg/jira/refresh.go:92-103`
**Issue:** `client.GetMyself(ctx)` runs (round-trip + auth) before
`cacheFilePath()` resolves `~/.config/waveterm/jira-cache.json`. If
`os.UserHomeDir()` returns an error (extremely rare but possible on a
misconfigured CI container with no HOME), Refresh has already burned a
network call and sent credentials to Jira. Reordering lets the local
check fail fast without touching the network.

**Fix:** Move the `cacheFilePath()` + `loadExistingLocalPaths` block above the
`GetMyself` call. Purely a hygiene cleanup; no observable behavior change in
normal usage.

### IN-02: `paginateSearch` requests `fields: ["summary"]` but never uses it

**File:** `pkg/jira/refresh.go:179`
**Issue:** The search loop collects only `ref.Key`; `ref.Fields.Summary` is
discarded. Requesting `summary` costs a small amount of server work and
bandwidth per search page for no downstream benefit (Step 2 re-fetches every
issue with the full field list).

**Fix:** Pass `Fields: []string{}` (Atlassian's enhanced search returns
key+id by default when `fields` is empty/omitted) or `Fields: []string{"*none"}`.
If the "summary" request was a future-proofing choice for a skip-re-fetch
optimization, add a `// TODO` so the intent is visible.

### IN-03: No lockfile / no concurrency guard on `Refresh`

**File:** `pkg/jira/refresh.go:82` (whole function)
**Issue:** D-CONC-01 states refresh is single-caller, but nothing enforces it.
Two concurrent `Refresh` calls (e.g., wsh subcommand + widget RPC firing
simultaneously) would race on the `.tmp` tempfile in `AtomicWriteFile` —
both writers target the same path, last-rename wins, and the loser may
see an `os.Rename` failure because the tmp file was already renamed away.

**Fix:** Either (a) document in a comment at the top of `Refresh` that
callers must serialize themselves, or (b) use `sync.Mutex` package-scoped
to the jira package:

```go
var refreshMu sync.Mutex

func Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error) {
    refreshMu.Lock()
    defer refreshMu.Unlock()
    // ...existing body
}
```

Phase 3 wsh + Phase 4 widget integration is where this matters; flagging
here so the decision is made before the call sites multiply.

### IN-04: `runRefreshOneIssue` helper loses the test server reference

**File:** `pkg/jira/refresh_test.go:115-126`
**Issue:** The helper registers `t.Cleanup(srv.Close)` but returns only
`(*RefreshReport, error)`. Callers that need to inspect the server (e.g.,
to assert the request path or headers) can't. Also, `TestRefresh_NullDescription`
and `TestRefresh_LastCommentAt` re-read the cache with `os.ReadFile(report.CachePath)`
while ignoring errors (`data, _ := ...`), which will panic unhelpfully on
`json.Unmarshal(nil, ...)` if the file wasn't written. Not a bug today
because Refresh returns an error when the file is missing, but a
defensive `if err != nil { t.Fatalf(...) }` makes diagnostics clearer.

**Fix:** Either return `*httptest.Server` alongside the report, or check
the `os.ReadFile` error explicitly in each test:

```go
data, err := os.ReadFile(report.CachePath)
if err != nil {
    t.Fatalf("read cache: %v", err)
}
```

Low priority; cosmetic.

---

_Reviewed: 2026-04-15T00:00:00Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_

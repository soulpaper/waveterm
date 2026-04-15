---
phase: 02-cache-orchestration
plan: 02
type: execute
wave: 2
depends_on: [02-01]
files_modified:
  - pkg/jira/refresh.go
autonomous: true
requirements: [JIRA-06, JIRA-07]

must_haves:
  truths:
    - "Refresh(ctx, opts) (*RefreshReport, error) orchestrates: GetMyself → paginate Search → GetIssue per key → buildCacheIssue → atomic write (D-FLOW-01..05)"
    - "GetMyself is called BEFORE pagination so auth failure aborts with no wasted work (D-ERR-03, RESEARCH §Pattern 1)"
    - "When RefreshOpts.HTTPClient is non-nil, Refresh uses NewClientWithHTTP(cfg, hc); else NewClient(cfg) (RESEARCH Open Question 1 / Recommendation 1)"
    - "Cache path resolves via os.UserHomeDir() + filepath.Join(home, \".config\", \"waveterm\", \"jira-cache.json\") — literal, Windows-safe (D-CACHE-01)"
    - "os.MkdirAll(filepath.Dir(cachePath), 0o755) is called before the atomic write (RESEARCH Pitfall 5)"
    - "Write uses pkg/util/fileutil.AtomicWriteFile — not a hand-rolled temp+rename (RESEARCH §Don't Hand-Roll)"
    - "json.MarshalIndent with 2-space indent — required for byte-identical golden-file compare (D-FLOW-05)"
    - "fetchedAt = time.Now().UTC().Format(time.RFC3339) — ISO8601 UTC with 'Z' suffix (D-CACHE-02)"
    - "Comments ordering: raw[len(raw)-10:] when len > 10 — keep LAST 10, drop oldest (RESEARCH Pitfall 2 / D-CACHE-06)"
    - "Comment.Author flattened: c.Author.DisplayName, fall back to c.Author.AccountID when displayName is empty (RESEARCH Pitfall 1)"
    - "Comment body truncated at 2000 chars; body=body[:2000] + Truncated=true (D-CACHE-06)"
    - "statusCategoryFromKey maps 'new'|'indeterminate'|'done' identically; everything else → 'new' (D-CACHE-08)"
    - "Attachment.WebUrl = wire-format a.Content unchanged (RESEARCH A3, D-CACHE-05)"
    - "Attachments/Comments slices always non-nil — initialized via make(..., 0, cap) even when empty (RESEARCH Pitfall 3)"
    - "loadExistingLocalPaths tolerates missing/malformed existing cache — returns empty map, never errors (D-FLOW-04)"
    - "Per-issue GetIssue failure: log.Printf with %v (never %+v), continue, omit from cache (D-ERR-01, T-01-02)"
    - "Fatal /myself failure or /search failure returns wrapped error; no cache file written (D-ERR-02, D-ERR-03)"
    - "OnProgress nil is safe; when non-nil, called with stages 'search', 'fetch', 'write' per D-PROG-01"
    - "RefreshReport.IssueCount = len(successfully-fetched issues); CachePath is populated even on partial success"
    - "All 9 TestRefresh_* tests pass (Nyquist GREEN transition complete)"
    - "All Phase 1 tests still pass"
  artifacts:
    - path: "pkg/jira/refresh.go"
      provides: "Refresh, RefreshOpts, RefreshReport — the phase entry point. Plus unexported helpers: buildCacheIssue, statusCategoryFromKey, loadExistingLocalPaths, cacheFilePath, progress, countAttachments, countComments."
      contains: "func Refresh(ctx context.Context"
  key_links:
    - from: "Refresh"
      to: "Client.GetMyself → Client.SearchIssues (loop) → Client.GetIssue (loop) → fileutil.AtomicWriteFile"
      via: "sequential orchestration per D-FLOW-01..05"
      pattern: "client\\.(GetMyself|SearchIssues|GetIssue)"
    - from: "buildCacheIssue"
      to: "ADFToMarkdown (description + every kept comment body)"
      via: "Phase 1 adf.go primitive"
      pattern: "ADFToMarkdown"
    - from: "Refresh cache write"
      to: "pkg/util/fileutil.AtomicWriteFile"
      via: "import github.com/wavetermdev/waveterm/pkg/util/fileutil"
      pattern: "fileutil\\.AtomicWriteFile"
    - from: "loadExistingLocalPaths"
      to: "JiraCacheIssue.Attachments[].LocalPath in prior cache"
      via: "map[key+\"::\"+attachmentID]string flat index"
      pattern: "\"::\""
---

<objective>
Implement `pkg/jira/refresh.go` — the single file that turns every RED test from Plan 01 GREEN.

Purpose: Close JIRA-06 and JIRA-07. Any Waveterm subsystem (Phase 3's wsh command, Phase 3's widget RPC) calls `jira.Refresh(ctx, opts)`, gets a `*RefreshReport` and a cache file at `~/.config/waveterm/jira-cache.json` in the exact schema the widget already reads.

Output: One new file, `pkg/jira/refresh.go`, ~250-300 LOC. No changes to any other file. No new imports outside stdlib + Phase 1 package + one in-repo helper (`pkg/util/fileutil`).

Non-goals (explicitly deferred):
- RPC / widget integration → Phase 3
- Rate limiting, retries, attachment downloads → Phase 5
- Parallel GetIssue calls → post-milestone (D-CONC-01 locked sequential)
- Incremental refresh (skip unchanged issues) → post-milestone
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/02-cache-orchestration/02-CONTEXT.md
@.planning/phases/02-cache-orchestration/02-RESEARCH.md
@.planning/phases/02-cache-orchestration/02-01-SUMMARY.md
@pkg/jira/client.go
@pkg/jira/cache_types.go
@pkg/jira/adf.go
@pkg/jira/errors.go
@pkg/jira/config.go
@pkg/jira/refresh_test.go
@pkg/util/fileutil/fileutil.go

<interfaces>
<!-- Everything Refresh needs is already defined. This plan only adds one file. -->

From pkg/jira/client.go (Phase 1 + Plan 01):
```go
func NewClient(cfg Config) *Client
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client
func (c *Client) SearchIssues(ctx, SearchOpts) (*SearchResult, error)
func (c *Client) GetIssue(ctx, key, GetIssueOpts) (*Issue, error)
func (c *Client) GetMyself(ctx) (*Myself, error)            // Plan 01

type SearchOpts struct { JQL, NextPageToken string; Fields []string; MaxResults int }
type GetIssueOpts struct { Fields []string }
type SearchResult struct { Issues []IssueRef; NextPageToken string; IsLast bool }
type IssueRef struct { ID, Key, Self string; Fields IssueFields }
type Issue struct { ID, Key, Self string; Fields IssueFields }
type IssueFields struct {
    Summary     string
    Description json.RawMessage
    Status      struct {
        Name, ID string
        StatusCategory struct { Key string } // Plan 01 addition
    }
    IssueType   struct { Name, ID string; Subtask bool }
    Priority    struct { Name, ID string }
    Project     struct { Key, Name, ID string }
    Created, Updated string
    Attachment  []Attachment
    Comment     CommentPage
}
type Attachment struct { ID, Filename, MimeType, Created, Content, Thumbnail string; Size int64; Author struct{ AccountID, DisplayName string } }
type CommentPage struct { Total int; Comments []Comment }
type Comment struct { ID string; Author struct{ AccountID, DisplayName string }; Body json.RawMessage; Created, Updated string }
type Myself struct { AccountID, DisplayName, EmailAddress string }
```

From pkg/jira/cache_types.go (Plan 01):
```go
type JiraCache struct { CloudId, BaseUrl, AccountId, FetchedAt string; Issues []JiraCacheIssue }
type JiraCacheIssue struct { /* 17 fields, see cache_types.go */ }
type JiraCacheAttachment struct { ID, Filename, MimeType string; Size int64; LocalPath, WebUrl string }
type JiraCacheComment struct { ID, Author, Created, Updated, Body string; Truncated bool `json:",omitempty"` }
```

From pkg/jira/adf.go: `func ADFToMarkdown(raw json.RawMessage) (string, error)` — returns ("", nil) for null/empty input.
From pkg/jira/config.go: `type Config struct { BaseUrl, CloudId, Email, ApiToken, Jql string; PageSize int }`, `const DefaultPageSize = 50`.
From pkg/util/fileutil/fileutil.go (line 179): `func AtomicWriteFile(fileName string, data []byte, perm os.FileMode) error` — temp+rename with cleanup on failure.
</interfaces>

<test_contract>
<!-- The failing tests in pkg/jira/refresh_test.go (Plan 01) are the contract. Read them. -->

The nine TestRefresh_* functions specify:
1. TestRefresh_GoldenFile     — byte-identical cache for single-issue ITSM-1 fixture
2. TestRefresh_PreserveLocalPath — carry forward localPath from seed cache
3. TestRefresh_CommentCapAndTruncation — keep LAST 10, truncate at 2000, Truncated=true
4. TestRefresh_LastCommentAt  — max(updated, created) across kept
5. TestRefresh_NullDescription — null ADF → ""
6. TestRefresh_StatusCategoryMapping — new/indeterminate/done passthrough, else "new"
7. TestRefresh_ErrorClassification — 401/500 fatal (no cache write); per-issue 404 omitted
8. TestRefresh_ProgressCallback — ("search",*,0), ("fetch",n,N), ("write",1,1)
9. TestRefresh_AttachmentWebUrlPassthrough — wire a.Content == cache a.WebUrl

The test helpers (setFakeHome, newRefreshTestServer, baseConfig, normalizeFetchedAt) are already in place — do NOT modify refresh_test.go. Only add new tests if you find a missing corner case; even then, prefer a separate follow-up plan.
</test_contract>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement pkg/jira/refresh.go — the orchestration entry point and helpers</name>
  <files>pkg/jira/refresh.go</files>
  <behavior>
After this task, `go test ./pkg/jira -count=1` runs every test in the package (including the nine TestRefresh_* tests from Plan 01) and they all pass. No assertion failures. No compile errors.

Test outcomes that prove behavior:
- Test 1 GoldenFile: produced cache bytes match testdata/cache.golden.json after fetchedAt normalization.
- Test 2 PreserveLocalPath: `out.Issues[0].Attachments[0].LocalPath == "C:/Users/dev/downloaded/att1-screenshot.png"`.
- Test 3 CommentCapAndTruncation: exactly 10 kept comments, first kept id "c06", c06 body len 2000 + Truncated=true, c07 Truncated=false.
- Test 4 LastCommentAt: `out.Issues[0].LastCommentAt == "2026-04-03T10:30:00.000+0000"` (c2's updated).
- Test 5 NullDescription: `out.Issues[0].Description == ""` and no error.
- Test 6 StatusCategoryMapping: table passes all 5 cases including unknown → "new".
- Test 7 ErrorClassification: 401 and 500 sub-tests → Refresh returns error AND cache file does not exist at report path; per-issue 404 sub-test → Refresh returns nil error, IssueCount reflects only successes.
- Test 8 ProgressCallback: recorded sequence contains ("search",*,0), ("fetch",1,1), ("write",1,1) in order.
- Test 9 AttachmentWebUrlPassthrough: exact byte equality of content URL.
  </behavior>
  <action>
Create `pkg/jira/refresh.go` as a single file with the following structure. Do NOT split into `cache.go`, `mapping.go` etc.; the file stays well under 400 LOC (RESEARCH §Recommended File Layout, alternative option approved).

### File header

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package jira — refresh orchestration (Phase 2).
//
// Refresh is the single entry point that Phase 3's wsh command and widget
// RPC will call. It combines Phase 1 primitives (Config, Client.SearchIssues,
// Client.GetIssue, Client.GetMyself, ADFToMarkdown) into one sequential
// operation that writes ~/.config/waveterm/jira-cache.json in the exact
// schema the widget at frontend/app/view/jiratasks/jiratasks.tsx consumes.
//
// Design per:
//   - CONTEXT.md:  D-CACHE-01..08, D-FLOW-01..05, D-PROG-01..02,
//                  D-ERR-01..03, D-CONC-01, D-TEST-01..03
//   - RESEARCH.md: §Architecture Patterns (Patterns 1-4), §Common Pitfalls 1-8

package jira
```

### Imports (exact set — no more, no fewer)

```go
import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "time"

    "github.com/wavetermdev/waveterm/pkg/util/fileutil"
)
```

### Public types

```go
// RefreshOpts is the input to Refresh. Config is required; the other fields
// are optional.
type RefreshOpts struct {
    // Config is the loaded Jira configuration. Typically obtained via
    // LoadConfig(); callers with a pre-built Config (tests, future wsh
    // subcommands) may pass it directly.
    Config Config

    // HTTPClient, if non-nil, overrides the default 30s-timeout http.Client
    // that NewClient would use. Tests pass httptest.NewServer().Client() here
    // so the server's self-signed cert is accepted. Phase 5 will pass a
    // rate-limiting Transport via this hook. Nil → NewClient(cfg) is used.
    HTTPClient *http.Client

    // OnProgress, if non-nil, is invoked at stage transitions (D-PROG-01).
    // Stages:
    //   "search" — paginating search. current = issues discovered so far;
    //              total = 0 (enhanced search has no total per RESEARCH
    //              Pitfall 6). First call is ("search", 0, 0).
    //   "fetch"  — per-issue GetIssue. current = 1-based index of the issue
    //              whose fetch just completed (success or skip);
    //              total = len(allKeys).
    //   "write"  — cache write. First call ("write", 0, 1) before marshal;
    //              final call ("write", 1, 1) after successful rename.
    //
    // The callback runs on Refresh's goroutine; it must not block.
    OnProgress func(stage string, current, total int)
}

// RefreshReport summarizes a successful Refresh. Nil when Refresh returns a
// fatal error before the first write (D-PROG-02).
type RefreshReport struct {
    IssueCount      int           // number of issues successfully written (omits per-issue failures)
    AttachmentCount int           // total attachments across kept issues
    CommentCount    int           // total kept comments across kept issues (NOT sum of issue.CommentCount)
    Elapsed         time.Duration
    CachePath       string        // absolute path of the written cache file
}
```

### Public entry point

```go
// Refresh fetches the configured JQL, converts each issue to the cache
// schema, and atomically writes ~/.config/waveterm/jira-cache.json.
//
// Error semantics:
//   - /myself failure:       fatal, no cache file written (D-ERR-03)
//   - /search failure:       fatal, no cache file written (D-ERR-02)
//   - per-issue GetIssue:    logged, issue skipped, Refresh continues (D-ERR-01)
//   - cache marshal/write:   fatal, returns wrapped error
//
// Security (T-01-02 extension): never logs Config fields or response Body.
// Error formatting uses %v only — see RESEARCH Pitfall 8.
func Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error) {
    start := time.Now()

    var client *Client
    if opts.HTTPClient != nil {
        client = NewClientWithHTTP(opts.Config, opts.HTTPClient)
    } else {
        client = NewClient(opts.Config)
    }

    // Step 3 first — fail fast on auth before doing any search work (D-ERR-03).
    me, err := client.GetMyself(ctx)
    if err != nil {
        return nil, fmt.Errorf("jira refresh: GetMyself failed: %v", err)
    }

    // Resolve cache path + preserve-localPath snapshot BEFORE fetching so a
    // fatal search failure leaves the existing cache untouched (D-ERR-02).
    cachePath, err := cacheFilePath()
    if err != nil {
        return nil, fmt.Errorf("jira refresh: %v", err)
    }
    localPaths := loadExistingLocalPaths(cachePath) // best-effort, never errors (D-FLOW-04)

    // Step 1 — paginate search until isLast or empty nextPageToken (D-FLOW-01).
    progress(opts.OnProgress, "search", 0, 0)
    allKeys, err := paginateSearch(ctx, client, opts.Config)
    if err != nil {
        return nil, fmt.Errorf("jira refresh: search failed: %v", err)
    }
    progress(opts.OnProgress, "search", len(allKeys), 0)

    // Step 2 — fetch each issue sequentially (D-CONC-01), build cache entries.
    const issueFieldsParam = "summary,description,status,issuetype,priority,project,created,updated,attachment,comment"
    issueFieldList := []string{
        "summary", "description", "status", "issuetype", "priority",
        "project", "created", "updated", "attachment", "comment",
    }
    _ = issueFieldsParam // kept for documentation; GetIssue joins the list itself

    cacheIssues := make([]JiraCacheIssue, 0, len(allKeys))
    for i, key := range allKeys {
        issue, gerr := client.GetIssue(ctx, key, GetIssueOpts{Fields: issueFieldList})
        if gerr != nil {
            // D-ERR-01: log (%v only, T-01-02) and skip.
            log.Printf("jira refresh: GetIssue %s failed: %v (skipping)", key, gerr)
            progress(opts.OnProgress, "fetch", i+1, len(allKeys))
            continue
        }
        cacheIssues = append(cacheIssues, buildCacheIssue(issue, opts.Config.BaseUrl, localPaths))
        progress(opts.OnProgress, "fetch", i+1, len(allKeys))
    }

    // Step 5 — marshal + atomic write (D-FLOW-05).
    progress(opts.OnProgress, "write", 0, 1)
    cache := JiraCache{
        CloudId:   opts.Config.CloudId,
        BaseUrl:   opts.Config.BaseUrl,
        AccountId: me.AccountID,
        FetchedAt: time.Now().UTC().Format(time.RFC3339),
        Issues:    cacheIssues,
    }
    data, err := json.MarshalIndent(&cache, "", "  ")
    if err != nil {
        return nil, fmt.Errorf("jira refresh: marshal cache: %v", err)
    }
    // Ensure parent directory exists (RESEARCH Pitfall 5 — fresh install has no ~/.config/waveterm yet).
    if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
        return nil, fmt.Errorf("jira refresh: mkdir cache parent: %v", err)
    }
    if err := fileutil.AtomicWriteFile(cachePath, data, 0o644); err != nil {
        return nil, fmt.Errorf("jira refresh: write cache: %v", err)
    }
    progress(opts.OnProgress, "write", 1, 1)

    return &RefreshReport{
        IssueCount:      len(cacheIssues),
        AttachmentCount: countAttachments(cacheIssues),
        CommentCount:    countComments(cacheIssues),
        Elapsed:         time.Since(start),
        CachePath:       cachePath,
    }, nil
}
```

### Pagination helper

```go
// paginateSearch calls SearchIssues repeatedly until IsLast is true or the
// server returns an empty NextPageToken. Collects issue keys only —
// per D-FLOW-02 full-field fetching happens in Step 2 via GetIssue.
func paginateSearch(ctx context.Context, client *Client, cfg Config) ([]string, error) {
    var keys []string
    var nextToken string
    // Defensive upper bound: 1000 pages × 50/page = 50_000 issues. More than
    // a realistic personal JQL. Prevents an infinite loop if the server
    // violates its own isLast contract.
    const maxPages = 1000
    for page := 0; page < maxPages; page++ {
        res, err := client.SearchIssues(ctx, SearchOpts{
            JQL:           cfg.Jql,
            NextPageToken: nextToken,
            MaxResults:    cfg.PageSize,
            Fields:        []string{"summary"}, // minimal — full fetch in Step 2
        })
        if err != nil {
            return nil, err
        }
        for _, ref := range res.Issues {
            keys = append(keys, ref.Key)
        }
        if res.IsLast || res.NextPageToken == "" {
            return keys, nil
        }
        nextToken = res.NextPageToken
    }
    return keys, fmt.Errorf("jira refresh: search pagination exceeded %d pages", maxPages)
}
```

### Mapping function (the schema bridge)

```go
// buildCacheIssue converts a wire-format *Issue into a cache-format
// JiraCacheIssue. Covers D-CACHE-03..08 and the comment transforms from
// D-CACHE-06..07. See RESEARCH Pattern 2 for the decision log.
func buildCacheIssue(issue *Issue, baseUrl string, localPaths map[string]string) JiraCacheIssue {
    // Description: ADF → markdown. Malformed ADF on ONE issue must not fail
    // refresh — degrade to "" and log.
    desc, err := ADFToMarkdown(issue.Fields.Description)
    if err != nil {
        log.Printf("jira refresh: ADF description parse error on %s: %v", issue.Key, err)
        desc = ""
    }

    // Attachments: metadata only. Always non-nil slice (Pitfall 3).
    atts := make([]JiraCacheAttachment, 0, len(issue.Fields.Attachment))
    for _, a := range issue.Fields.Attachment {
        lp := localPaths[issue.Key+"::"+a.ID] // "" if unseen
        atts = append(atts, JiraCacheAttachment{
            ID:        a.ID,
            Filename:  a.Filename,
            MimeType:  a.MimeType,
            Size:      a.Size,
            LocalPath: lp,
            WebUrl:    a.Content, // Jira already returns the site-pattern URL (A3)
        })
    }

    // Comments: wire is oldest-first. Keep LAST 10 (Pitfall 2).
    raw := issue.Fields.Comment.Comments
    keep := raw
    if len(raw) > 10 {
        keep = raw[len(raw)-10:]
    }
    cmts := make([]JiraCacheComment, 0, len(keep))
    var lastAt string
    for _, c := range keep {
        body, cerr := ADFToMarkdown(c.Body)
        if cerr != nil {
            log.Printf("jira refresh: ADF comment parse error on %s comment %s: %v", issue.Key, c.ID, cerr)
            body = ""
        }
        truncated := false
        if len(body) > 2000 {
            body = body[:2000]
            truncated = true
        }
        // Author flatten (Pitfall 1). DisplayName preferred; accountId fallback.
        author := c.Author.DisplayName
        if author == "" {
            author = c.Author.AccountID
        }
        cmts = append(cmts, JiraCacheComment{
            ID:        c.ID,
            Author:    author,
            Created:   c.Created,
            Updated:   c.Updated,
            Body:      body,
            Truncated: truncated,
        })
        // lastCommentAt = max(updated, created) across kept (D-CACHE-07).
        // ISO8601 timestamps with consistent Jira timezone are lexically
        // sortable (RESEARCH A1).
        candidate := c.Updated
        if c.Created > candidate {
            candidate = c.Created
        }
        if candidate > lastAt {
            lastAt = candidate
        }
    }

    return JiraCacheIssue{
        Key:            issue.Key,
        ID:             issue.ID,
        Summary:        issue.Fields.Summary,
        Description:    desc,
        Status:         issue.Fields.Status.Name,
        StatusCategory: statusCategoryFromKey(issue.Fields.Status.StatusCategory.Key),
        IssueType:      issue.Fields.IssueType.Name,
        Priority:       issue.Fields.Priority.Name,
        ProjectKey:     issue.Fields.Project.Key,
        ProjectName:    issue.Fields.Project.Name,
        Updated:        issue.Fields.Updated,
        Created:        issue.Fields.Created,
        WebUrl:         baseUrl + "/browse/" + issue.Key,
        Attachments:    atts,
        Comments:       cmts,
        CommentCount:   issue.Fields.Comment.Total, // wire total, may exceed len(cmts)
        LastCommentAt:  lastAt,
    }
}

// statusCategoryFromKey maps Jira's statusCategory.key to the 3-value enum
// the widget consumes (D-CACHE-08). Unknown / missing → "new".
func statusCategoryFromKey(k string) string {
    switch k {
    case "new", "indeterminate", "done":
        return k
    default:
        return "new"
    }
}
```

### Existing-cache reader (best-effort)

```go
// loadExistingLocalPaths reads ~/.config/waveterm/jira-cache.json if it
// exists and extracts a flat map of (issueKey + "::" + attachmentID) →
// localPath for every attachment with a non-empty LocalPath. Any error —
// file missing, permission denied, malformed JSON, schema drift — returns
// an empty map with no error surfaced (D-FLOW-04). This is deliberate:
// Refresh's job is to produce a fresh cache, not to validate the prior one.
func loadExistingLocalPaths(cachePath string) map[string]string {
    out := map[string]string{}
    data, err := os.ReadFile(cachePath)
    if err != nil {
        return out
    }
    var existing JiraCache
    if err := json.Unmarshal(data, &existing); err != nil {
        return out
    }
    for _, iss := range existing.Issues {
        for _, a := range iss.Attachments {
            if a.LocalPath != "" {
                out[iss.Key+"::"+a.ID] = a.LocalPath
            }
        }
    }
    return out
}
```

### Path resolver

```go
// cacheFilePath returns the literal ~/.config/waveterm/jira-cache.json path
// (D-CACHE-01). Uses os.UserHomeDir() which honors $HOME on POSIX and
// %USERPROFILE% on Windows — consistent with pkg/jira/config.go's LoadConfig.
func cacheFilePath() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("cannot resolve home directory: %w", err)
    }
    return filepath.Join(home, ".config", "waveterm", "jira-cache.json"), nil
}
```

### Progress + counter helpers

```go
// progress invokes cb if non-nil. Centralizing the nil check keeps call
// sites clean (D-PROG-01).
func progress(cb func(string, int, int), stage string, cur, total int) {
    if cb != nil {
        cb(stage, cur, total)
    }
}

func countAttachments(issues []JiraCacheIssue) int {
    n := 0
    for _, i := range issues {
        n += len(i.Attachments)
    }
    return n
}

func countComments(issues []JiraCacheIssue) int {
    n := 0
    for _, i := range issues {
        n += len(i.Comments)
    }
    return n
}
```

### Anti-patterns to avoid (cross-ref RESEARCH §Anti-Patterns)

1. Do NOT call `NewClient` and also `NewClientWithHTTP` — branch once in Refresh.
2. Do NOT hand-roll temp+rename — use `fileutil.AtomicWriteFile`.
3. Do NOT slice with `raw[:10]` for "latest 10" — Jira returns oldest-first.
4. Do NOT marshal comments with `json.Marshal(c.Author)` — flatten to string.
5. Do NOT log `err.Error()` with `%+v` or `%#v` — %v only, always, on any jira-package error.
6. Do NOT skip `os.MkdirAll` — Windows fresh installs break without it.
7. Do NOT return a partial `*RefreshReport` on fatal error — return `nil, err`. (D-PROG-02.)
8. Do NOT change any struct tag in cache_types.go to "fix" a test — the widget contract is immutable this phase.

### Golden-file note

If `TestRefresh_GoldenFile` fails after implementation with a 1-byte or 2-byte diff, it is almost always (a) a whitespace mismatch from MarshalIndent indent width (must be 2 spaces, not tab), (b) a trailing newline issue (MarshalIndent does NOT emit a trailing newline; the golden file should not end with one either), or (c) `truncated` omitempty behavior inverting expectations (confirm: when truncated=false, the field is omitted entirely; when truncated=true, field appears). If the diff is large, re-read RESEARCH §Cache Type Definitions and compare field order — Go's json marshals in struct-declaration order, which is why cache_types.go declares fields in widget-interface order.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm && go vet ./pkg/jira/... && go test ./pkg/jira -count=1</automated>
  </verify>
  <done>
- `pkg/jira/refresh.go` exists; file LOC < 400.
- `go vet ./pkg/jira/...` clean.
- `go test ./pkg/jira -count=1` passes with zero failures, covering all Phase 1 tests AND all nine `TestRefresh_*` tests.
- `grep -c 'func ' pkg/jira/refresh.go` returns at least 8 (Refresh, paginateSearch, buildCacheIssue, statusCategoryFromKey, loadExistingLocalPaths, cacheFilePath, progress, countAttachments, countComments).
- `grep '%+v' pkg/jira/refresh.go` returns empty (T-01-02 compliance).
- `grep 'fileutil.AtomicWriteFile' pkg/jira/refresh.go` returns exactly one match.
- `grep 'os.MkdirAll' pkg/jira/refresh.go` returns exactly one match (Pitfall 5 compliance).
- `grep 'raw\[len(raw)-10:\]' pkg/jira/refresh.go` returns one match (Pitfall 2 compliance).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Refresh ↔ Jira Cloud | Untrusted upstream; responses are parsed into typed Go structs; 1KB error body cap (Phase 1) prevents memory amplification |
| Refresh ↔ local filesystem | Writes only to `~/.config/waveterm/jira-cache.json` (+ its `.tmp` sibling via fileutil); no user-controlled paths |
| Refresh ↔ log sink | Error messages sanitized — `*APIError.Error()` scrubs body; format verbs restricted to %v |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-02-10 | Information Disclosure | Per-issue failure log leaking response body | mitigate | `log.Printf("... %v", err)` — never `%+v`. Phase 1's `APIError.Error()` omits Body. Verified by grep guard in Done criteria. |
| T-02-11 | Information Disclosure | apiToken leaked via error messages | mitigate | Refresh never formats `opts.Config` or any field of it. Config is passed to `NewClient`, never to `log`. Verified by grep for `log\..*Config`. |
| T-02-12 | Tampering | Malformed existing cache → panic during loadExistingLocalPaths | mitigate | Function swallows Unmarshal errors, returns empty map. No panic path (no nil deref: JiraCache has all non-pointer fields). |
| T-02-13 | Denial of Service | Runaway pagination if server violates isLast contract | mitigate | `maxPages = 1000` upper bound → at most 50,000 issues scanned before bail. Returns a non-partial error, no cache written. |
| T-02-14 | Tampering | Concurrent Refresh clobbers partial write | accept | D-CONC-01 precludes goroutine-level concurrency; two wsh processes running Refresh simultaneously is a known narrow race (RESEARCH Pitfall 7). `fileutil.AtomicWriteFile` reduces window to the rename instant. Phase 5 may add file-lock. |
| T-02-15 | Information Disclosure | temp file `jira-cache.json.tmp` with stale data | mitigate | `fileutil.AtomicWriteFile` removes the temp on rename failure (fileutil.go line 188). Permissions 0o644 match wconfig convention. |
| T-02-16 | Spoofing | Unexpected statusCategory.key from compromised Jira server | mitigate | `statusCategoryFromKey` whitelist: only "new"/"indeterminate"/"done" pass through; everything else collapses to "new". No code injection surface. |
| T-02-17 | DoS | Server returns 10,000-comment issue → 10,000 ADF parses | accept | Ceiling is Jira's (5000 per issue); ADF parser is streaming; per-issue memory bounded by 30s client timeout. Not worth rate-limiting locally this phase. |
</threat_model>

<verification>
Wave-2 gate:

1. `cd F:/Waveterm/waveterm && go build ./pkg/jira/...` succeeds.
2. `go vet ./pkg/jira/...` clean.
3. `go test ./pkg/jira -count=1` PASSES ALL tests (Phase 1 suite + all nine `TestRefresh_*`). Nyquist GREEN achieved.
4. `go test ./... -count=1` from repo root — the full repo suite still passes (no cross-package break).
5. `wc -l pkg/jira/refresh.go` reports < 400 lines.
6. No forbidden format verbs: `grep -n '%+v\|%#v' pkg/jira/refresh.go` is empty.
7. Required helper calls present:
   - `grep fileutil.AtomicWriteFile pkg/jira/refresh.go` → 1 match
   - `grep os.MkdirAll pkg/jira/refresh.go` → 1 match
   - `grep 'raw\[len(raw)-10:\]' pkg/jira/refresh.go` → 1 match
   - `grep 'GetMyself' pkg/jira/refresh.go` → exactly 1 match (in Refresh itself, not duplicated in a helper)
8. Manual smoke (optional but recommended before declaring the phase done):
   - Back up any real `~/.config/waveterm/jira-cache.json`
   - With a real `jira.json` present, run a tiny main that calls `jira.Refresh` → verify the widget still renders.
</verification>

<success_criteria>
- ROADMAP Phase 2 Success #1: `jira.Refresh()` against a fake server produces byte-identical cache JSON → proven by TestRefresh_GoldenFile.
- ROADMAP Phase 2 Success #2: commentCount total + 10-comment cap + truncated:true → proven by TestRefresh_CommentCapAndTruncation.
- ROADMAP Phase 2 Success #3: lastCommentAt = max(updated, created) → proven by TestRefresh_LastCommentAt.
- ROADMAP Phase 2 Success #4: existing localPath survives refresh → proven by TestRefresh_PreserveLocalPath.
- ROADMAP Phase 2 Success #5 (widget un-modified reads cache): verified implicitly by the cache_types.go ↔ widget interface isomorphism + golden-file test. Explicit manual smoke is Phase 3's integration gate, not Phase 2's.
- Phase 1 suite remains green.
- Zero new third-party dependencies; `go.mod` unchanged.
</success_criteria>

<output>
After completion, create `.planning/phases/02-cache-orchestration/02-02-SUMMARY.md` per `$HOME/.claude/get-shit-done/templates/summary.md`. SUMMARY should list:
- The public surface added: `Refresh`, `RefreshOpts`, `RefreshReport`.
- Internal helpers: `paginateSearch`, `buildCacheIssue`, `statusCategoryFromKey`, `loadExistingLocalPaths`, `cacheFilePath`, `progress`, `countAttachments`, `countComments`.
- RESEARCH Open Question resolutions: (1) adopted `RefreshOpts.HTTPClient`; (2) adopted regex-based fetchedAt normalization in tests; (3) no action needed for schema-drift compatibility; (4) post-call progress semantics.
- Pitfall guardrails exercised: 1 (author flatten), 2 (last-10), 3 (non-nil slices), 4 (statusCategory extension from Plan 01), 5 (MkdirAll), 6 (search total=0), 7 (accepted narrow race), 8 (%v only).
- Any surprises encountered during implementation (e.g. golden-file byte-diff debugging).
- Exact `go test ./pkg/jira -count=1` output (pass count) as evidence.
- Phase-gate note: Phase 3 can now import `pkg/jira.Refresh` directly; no further pkg/jira changes expected in Phase 3.
</output>
</content>
</invoke>
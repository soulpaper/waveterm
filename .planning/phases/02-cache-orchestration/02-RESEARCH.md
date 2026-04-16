# Phase 2: Cache Orchestration — Research

**Researched:** 2026-04-15
**Domain:** Go refresh orchestration, Jira Cloud REST v3 (search/issue/myself), atomic file write on Windows, JSON schema fidelity with an existing TS widget consumer
**Confidence:** HIGH (schema verified against widget source; Jira API shapes verified against official Atlassian docs + Phase 1 struct definitions; atomic-write pattern already exists in-repo)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (D-CACHE-01 .. D-TEST-03)

**Cache schema (D-CACHE-01 .. D-CACHE-08):**
- **D-CACHE-01** Cache path = `~/.config/waveterm/jira-cache.json` (literal, `os.UserHomeDir()` + `filepath.Join`).
- **D-CACHE-02** Top-level fields: `cloudId`, `baseUrl`, `accountId`, `fetchedAt` (ISO8601 UTC), `issues[]`.
- **D-CACHE-03** `JiraIssue` fields: `key`, `id`, `summary`, `description`, `status`, `statusCategory` (`new`|`indeterminate`|`done`), `issueType`, `priority`, `projectKey`, `projectName`, `updated`, `created`, `webUrl`, `attachments[]`, `comments[]`, `commentCount`, `lastCommentAt`.
- **D-CACHE-04** Description = ADF → markdown via `ADFToMarkdown`. Empty string when description is null.
- **D-CACHE-05** Attachment metadata only: `id`, `filename`, `mimeType`, `size`, `webUrl` in **site pattern** `https://<site>.atlassian.net/rest/api/3/attachment/content/<id>` (NOT `api.atlassian.com`); `localPath` defaults to `""`.
- **D-CACHE-06** Comments: keep latest 10 (oldest dropped). Body truncated at 2000 chars; `truncated: true` when body exceeds. `body` = ADF-flattened markdown.
- **D-CACHE-07** `commentCount` = total from API (may exceed 10). `lastCommentAt` = `max(updated, created)` across **kept** comments (ISO8601, `""` if none kept).
- **D-CACHE-08** `statusCategory.key` → `new` | `indeterminate` | `done`. Unknown → `new`.

**Refresh flow (D-FLOW-01 .. D-FLOW-05):**
- **D-FLOW-01** Step 1 — `SearchIssues` with `cfg.Jql` and `cfg.PageSize`; paginate until `isLast`. Collect issue keys only.
- **D-FLOW-02** Step 2 — per key, `GetIssue(key, fields=["summary","description","status","issuetype","priority","project","created","updated","attachment","comment"])`. ADF conversion happens here.
- **D-FLOW-03** Step 3 — once per refresh, `GET /rest/api/3/myself` → capture `accountId`. Adds one Client method.
- **D-FLOW-04** Step 4 — preserve-localPath: read existing cache if present, build `map[issueKey+attachmentId] → localPath`, carry forward non-empty values. Missing/malformed existing cache = empty map (no error).
- **D-FLOW-05** Step 5 — atomic write: `json.MarshalIndent(..., "", "  ")` → `jira-cache.json.tmp` in same directory → `os.Rename` to final path.

**Progress / report (D-PROG-01 .. D-PROG-02):**
- **D-PROG-01** `RefreshOpts.OnProgress func(stage string, current, total int)` (nillable). Stages: `"search"`, `"fetch"`, `"write"`.
- **D-PROG-02** `RefreshReport`: `IssueCount`, `AttachmentCount`, `CommentCount`, `Elapsed time.Duration`, `CachePath string`. Nil OK on fatal error before first write.

**Error handling (D-ERR-01 .. D-ERR-03):**
- **D-ERR-01** Per-issue `GetIssue` failure → `log.Printf` (no body, no auth), continue. Failed issue omitted from cache.
- **D-ERR-02** Fatal (auth / cannot write / full search failure) → wrapped error. Do NOT write partial cache.
- **D-ERR-03** `/myself` failure is fatal.

**Concurrency (D-CONC-01):** Sequential — no goroutines.

**Testing (D-TEST-01 .. D-TEST-03):**
- **D-TEST-01** `httptest.NewServer` fixture for `/search/jql`, `/issue/{key}`, `/myself`. Byte-identical cache JSON vs golden = top-level assertion (ROADMAP Success #1).
- **D-TEST-02** Preserve-localPath survives round-trip.
- **D-TEST-03** Comment truncation (2500 → 2000 + `truncated:true`); comment cap (15 → 10 kept, `commentCount:15`).

### Claude's Discretion

- Exact `RefreshOpts` struct field order
- Internal helper function names and organization
- Single file (`refresh.go`) vs multi-file split (`refresh.go` + `cache.go` + `mapping.go`) — planner decides based on file size

### Deferred Ideas (OUT OF SCOPE this phase)

- Incremental refresh based on last-seen `updated` timestamp
- Parallel issue fetching (defer behind D-CONC-01)
- Per-user notification counts
- Watcher flag
- RPC plumbing / widget changes / setup UX (Phases 3 & 4)
- Rate limiting, retry, attachment downloads (Phase 5)
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| JIRA-06 | Keep latest 10 comments per issue, each body truncated to 2000 chars; `commentCount` reflects total; `truncated:true` set where applicable | §Comment Pagination & Ordering, §Mapping Table — Comments, §Test Fixtures |
| JIRA-07 | Atomic write of `~/.config/waveterm/jira-cache.json` in the existing widget schema; ADF descriptions & comment bodies converted to markdown | §Cache Schema (Widget-Authoritative), §Atomic Write on Windows, §Mapping Table |
</phase_requirements>

---

## Summary

Phase 2 is a **pure assembly phase** — every primitive it needs already exists. Phase 1 delivered `Config`, `Client.SearchIssues`, `Client.GetIssue`, `ADFToMarkdown`, typed errors, and an `IssueFields` struct whose shape already matches what the widget needs (with one flatten step for `comment.author` → `displayName`). The in-repo helper `fileutil.AtomicWriteFile` provides exactly the temp-file-plus-rename pattern D-FLOW-05 specifies.

The biggest schema risk is **comment author**: Jira returns `author` as an **object** (`{accountId, displayName, ...}`) but the widget reads `c.author` as a **string** (rendered into `<span>{c.author}</span>` and `${c.author}` template literals). Refresh MUST flatten `author.displayName` (falling back to `accountId` when privacy settings hide display name) — this is the single transformation that a naive "marshal Jira response straight to cache" approach would miss.

Secondary critical findings: (1) `fields.comment` is a paginated **object** `{total, maxResults, startAt, comments[]}` — Phase 1's `CommentPage` already decodes this correctly; the widget consumer-side test would instantly catch this if violated. (2) Comments in the issue GET response come back **oldest-first**; "keep latest 10" means keep the **last 10** of the array (indices `[len-10:]`), not the first 10. (3) The `attachment.content` URL Jira returns is **already** in the site pattern D-CACHE-05 mandates — we can pass it through as `webUrl` unchanged, no rebuilding required. (4) `/search/jql` has **no `total` field** in the response (enhanced search is cursor-based); progress reporting for `"search"` uses running count, not `N/total`.

**Primary recommendation:** Introduce one new client method `GetMyself` (≈20 LOC mirroring `GetIssue`), then build a single `refresh.go` with a private `buildCacheIssue(*Issue, map[string]string) JiraCacheIssue` mapping function. Reuse `fileutil.AtomicWriteFile` for the write step. Test with three fixture handlers (`/search/jql`, `/issue/{key}`, `/myself`) in one `httptest.NewServer` multiplexer — pattern matches Phase 1's `client_test.go`.

---

## Standard Stack

### Core (all stdlib + Phase 1 + one in-repo helper — no new dependencies)

| Package | Purpose | Source |
|---------|---------|--------|
| `pkg/jira` (Phase 1) | `Config`, `Client`, `SearchIssues`, `GetIssue`, `ADFToMarkdown`, typed errors | [VERIFIED: pkg/jira/client.go, adf.go, errors.go, config.go] |
| `github.com/wavetermdev/waveterm/pkg/util/fileutil` | `AtomicWriteFile(fileName, data, perm)` — exact temp+rename pattern D-FLOW-05 describes | [VERIFIED: pkg/util/fileutil/fileutil.go lines 179-194] |
| `encoding/json` | `MarshalIndent`, `Unmarshal`, `RawMessage` | stdlib |
| `os`, `path/filepath` | Home dir, path joining, file read for existing-cache load | [VERIFIED: codebase pattern] |
| `time` | `time.Now().UTC().Format(time.RFC3339)` for `fetchedAt`; `time.Since` for `Elapsed` | stdlib |
| `log` | Per-issue failure logging (D-ERR-01) — matches rest of `pkg/` | [VERIFIED: 89 files in pkg/ import "log"; slog not used] |
| `context` | Pass-through to `SearchIssues` / `GetIssue` | stdlib |

**No `go.mod` edits.** Go `1.25.6` per `go.mod`. [VERIFIED: F:/Waveterm/waveterm/go.mod line 3]

### Alternatives Considered

| Instead of | Could Use | Rejected Because |
|------------|-----------|------------------|
| `fileutil.AtomicWriteFile` | Hand-roll temp+rename in `refresh.go` | D-FLOW-05 describes exactly this helper's behavior; reusing avoids subtle bugs (temp cleanup on write-failure, fully-qualified error wrapping) [VERIFIED: fileutil.go lines 179-194] |
| Goroutines over `GetIssue` calls | `sync.errgroup` with bounded concurrency | D-CONC-01 explicitly forbids this phase; defer to a future milestone |
| `json.Marshal` (compact) | `json.MarshalIndent(..., "", "  ")` | D-FLOW-05 specifies 2-space indent. Important for: (a) human diffability when teammates compare caches, (b) byte-identical golden-file test in D-TEST-01 |
| `slog` | `log` | Phase 1 established `log`; `slog` is not imported anywhere in `pkg/` — keep consistent |
| Custom `CommentPage` decode | Reuse Phase 1's `CommentPage` (already on `IssueFields.Comment`) | Already correct: `{total int, comments []Comment}` — no decode needed |

### Version verification

- `fileutil.AtomicWriteFile` — in-repo helper, no version — [VERIFIED: file exists and is used by `settingsconfig.go`-adjacent code paths]
- Go stdlib `time.RFC3339` format — produces ISO8601-compatible output with timezone offset; for UTC `Z` suffix use `time.Now().UTC().Format(time.RFC3339)` — [VERIFIED: Go docs, widget parses `i.updated` via `Date.parse()` which accepts both forms]

---

## Architecture Patterns

### Recommended File Layout

```
pkg/jira/
├── (existing from Phase 1: config.go, client.go, adf.go, errors.go, doc.go)
├── refresh.go             // NEW: Refresh, RefreshOpts, RefreshReport, buildCacheIssue, loadExistingCache, statusCategoryFromKey
├── refresh_test.go        // NEW: httptest fixture for /myself + /search/jql + /issue/{key}; golden-file round-trip; preserve-localPath; truncation/cap
└── cache_types.go         // NEW (optional): JiraCache, JiraCacheIssue, JiraCacheAttachment, JiraCacheComment — the on-disk shape (separate from client.go's Jira-wire types)
```

**Rationale for separating `cache_types.go`:**
`client.go` already has `Issue`, `IssueFields`, `Attachment`, `Comment` — these mirror Jira's wire format (including `author` as an object, `description` as `json.RawMessage`, `Comment` body as `json.RawMessage`). The cache has a **flattened, post-ADF-converted** shape (author as string, body as string, no raw ADF). Mixing both in `client.go` would confuse readers; a dedicated `cache_types.go` makes the schema contract explicit and gives the test file one place to import from.

**Alternative — single `refresh.go` with cache types inline:** acceptable if the file stays under ~400 LOC. Planner's discretion.

### One New Client Method (extension, not retrofit)

```go
// Source: mirrors pkg/jira/client.go GetIssue; response schema from
// https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-myself/
// [CITED: developer.atlassian.com/.../api-group-myself]

// Myself is the response shape of GET /rest/api/3/myself. Only AccountId is
// needed for the cache schema (D-CACHE-02); other fields are ignored.
type Myself struct {
    AccountID   string `json:"accountId"`
    DisplayName string `json:"displayName"`
    EmailAddress string `json:"emailAddress"`
}

// GetMyself calls GET /rest/api/3/myself and returns the authenticated user's
// identity. Per D-ERR-03 the caller treats any error as fatal for refresh.
func (c *Client) GetMyself(ctx context.Context) (*Myself, error) {
    endpoint := c.cfg.BaseUrl + "/rest/api/3/myself"
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
    if err != nil {
        return nil, fmt.Errorf("jira: build myself request: %w", err)
    }
    c.setCommonHeaders(req)

    var me Myself
    if err := c.doJSON(req, &me); err != nil {
        return nil, err
    }
    return &me, nil
}
```

Place in `client.go` near `GetIssue` (file stays well under 500 LOC).

### Cache Type Definitions (widget-authoritative schema)

```go
// Source: frontend/app/view/jiratasks/jiratasks.tsx lines 116-160
// [VERIFIED: Read of the widget file — every field name/type matches the TS interface]

type JiraCache struct {
    CloudId   string           `json:"cloudId"`
    BaseUrl   string           `json:"baseUrl"`
    AccountId string           `json:"accountId"`
    FetchedAt string           `json:"fetchedAt"`   // ISO8601 UTC
    Issues    []JiraCacheIssue `json:"issues"`
}

type JiraCacheIssue struct {
    Key            string                `json:"key"`
    ID             string                `json:"id"`
    Summary        string                `json:"summary"`
    Description    string                `json:"description"`    // ADF → markdown
    Status         string                `json:"status"`         // status.name
    StatusCategory string                `json:"statusCategory"` // "new"|"indeterminate"|"done"
    IssueType      string                `json:"issueType"`      // issuetype.name
    Priority       string                `json:"priority"`       // priority.name or ""
    ProjectKey     string                `json:"projectKey"`
    ProjectName    string                `json:"projectName"`
    Updated        string                `json:"updated"`
    Created        string                `json:"created"`
    WebUrl         string                `json:"webUrl"`         // baseUrl + "/browse/" + key
    Attachments    []JiraCacheAttachment `json:"attachments"`
    Comments       []JiraCacheComment    `json:"comments"`
    CommentCount   int                   `json:"commentCount"`
    LastCommentAt  string                `json:"lastCommentAt"`  // max(updated, created) of kept
}

type JiraCacheAttachment struct {
    ID        string `json:"id"`
    Filename  string `json:"filename"`
    MimeType  string `json:"mimeType"`
    Size      int64  `json:"size"`
    LocalPath string `json:"localPath"`  // "" default, preserved from prior cache
    WebUrl    string `json:"webUrl"`     // site pattern (Jira's attachment.content)
}

type JiraCacheComment struct {
    ID        string `json:"id"`
    Author    string `json:"author"`     // FLATTENED from Jira's author object
    Created   string `json:"created"`
    Updated   string `json:"updated"`
    Body      string `json:"body"`       // ADF → markdown, truncated to 2000
    Truncated bool   `json:"truncated,omitempty"` // only emitted when true
}
```

**Field notes:**
- `Truncated` uses `omitempty` so it doesn't appear on comments shorter than 2000 chars (matches how the existing widget-written cache likely looks; the widget reads `c.truncated ?? false` so both shapes work — but golden-file comparison in D-TEST-01 needs to match exactly).
- All struct fields non-pointer. Empty arrays must be emitted as `[]` not `null` — Go's `json.Marshal` of a nil slice produces `null`. **Fix:** initialize `Attachments: []JiraCacheAttachment{}` and `Comments: []JiraCacheComment{}` explicitly in `buildCacheIssue`, even when empty. [VERIFIED: widget `loadFromCache` defensively handles both (`i.attachments || []`) but golden-file byte comparison requires one canonical shape — choose `[]`].

### Pattern 1: Refresh Entry Point

```go
// Source: orchestration design from CONTEXT D-FLOW-01 .. D-FLOW-05
type RefreshOpts struct {
    Config     Config
    OnProgress func(stage string, current, total int) // may be nil
}

type RefreshReport struct {
    IssueCount      int
    AttachmentCount int
    CommentCount    int
    Elapsed         time.Duration
    CachePath       string
}

func Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error) {
    start := time.Now()
    client := NewClient(opts.Config)

    // Step 3 first (fail-fast on auth before paginating search)
    me, err := client.GetMyself(ctx)
    if err != nil {
        return nil, fmt.Errorf("jira refresh: GetMyself failed: %w", err)
    }

    // Step 1: paginate search
    progress(opts.OnProgress, "search", 0, 0)
    var allKeys []string
    var nextToken string
    for {
        res, err := client.SearchIssues(ctx, SearchOpts{
            JQL:           opts.Config.Jql,
            NextPageToken: nextToken,
            MaxResults:    opts.Config.PageSize,
            Fields:        []string{"summary"}, // minimal — full fetch is Step 2
        })
        if err != nil {
            return nil, fmt.Errorf("jira refresh: search failed: %w", err)
        }
        for _, ref := range res.Issues {
            allKeys = append(allKeys, ref.Key)
        }
        progress(opts.OnProgress, "search", len(allKeys), 0) // no total available
        if res.IsLast {
            break
        }
        nextToken = res.NextPageToken
        if nextToken == "" { // defensive: malformed server, avoid infinite loop
            break
        }
    }

    // Step 4 (read existing cache for localPath preservation)
    cachePath, err := cacheFilePath()
    if err != nil {
        return nil, err
    }
    localPaths := loadExistingLocalPaths(cachePath) // non-fatal: returns empty map on any error

    // Step 2: fetch each issue sequentially (D-CONC-01)
    issueFields := []string{"summary","description","status","issuetype","priority","project","created","updated","attachment","comment"}
    cacheIssues := make([]JiraCacheIssue, 0, len(allKeys))
    for i, key := range allKeys {
        progress(opts.OnProgress, "fetch", i+1, len(allKeys))
        issue, err := client.GetIssue(ctx, key, GetIssueOpts{Fields: issueFields})
        if err != nil {
            log.Printf("jira refresh: GetIssue %s failed: %v (skipping)", key, err)
            continue
        }
        cacheIssues = append(cacheIssues, buildCacheIssue(issue, opts.Config.BaseUrl, localPaths))
    }

    // Step 5: atomic write
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
        return nil, fmt.Errorf("jira refresh: marshal cache: %w", err)
    }
    if err := fileutil.AtomicWriteFile(cachePath, data, 0o644); err != nil {
        return nil, fmt.Errorf("jira refresh: write cache: %w", err)
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

func progress(cb func(string, int, int), stage string, cur, total int) {
    if cb != nil {
        cb(stage, cur, total)
    }
}
```

### Pattern 2: Mapping Function (the schema bridge)

```go
// Source: D-CACHE-03..D-CACHE-08 + widget interface definitions
// Takes a wire-format *Issue and produces the cache-format JiraCacheIssue.

func buildCacheIssue(issue *Issue, baseUrl string, localPaths map[string]string) JiraCacheIssue {
    // Description: ADF → markdown (D-CACHE-04)
    desc, err := ADFToMarkdown(issue.Fields.Description)
    if err != nil {
        // Malformed ADF on a single issue should NOT fail refresh — degrade to empty.
        log.Printf("jira refresh: ADF description parse error on %s: %v", issue.Key, err)
        desc = ""
    }

    // Attachments: metadata only (D-CACHE-05); carry forward localPath
    atts := make([]JiraCacheAttachment, 0, len(issue.Fields.Attachment))
    for _, a := range issue.Fields.Attachment {
        lp := localPaths[issue.Key+"::"+a.ID] // empty string if unseen
        atts = append(atts, JiraCacheAttachment{
            ID:        a.ID,
            Filename:  a.Filename,
            MimeType:  a.MimeType,
            Size:      a.Size,
            LocalPath: lp,
            WebUrl:    a.Content, // Jira already returns the site-pattern URL
        })
    }

    // Comments: oldest-first from Jira; keep LAST 10 (D-CACHE-06, D-CACHE-07)
    raw := issue.Fields.Comment.Comments
    keep := raw
    if len(raw) > 10 {
        keep = raw[len(raw)-10:]
    }
    cmts := make([]JiraCacheComment, 0, len(keep))
    var lastAt string
    for _, c := range keep {
        body, err := ADFToMarkdown(c.Body)
        if err != nil {
            log.Printf("jira refresh: ADF comment parse error on %s comment %s: %v", issue.Key, c.ID, err)
            body = ""
        }
        truncated := false
        if len(body) > 2000 {
            body = body[:2000]
            truncated = true
        }
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
        // lastCommentAt = max(updated, created) across kept (D-CACHE-07)
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
        StatusCategory: statusCategoryFromKey(getStatusCategoryKey(issue)), // see note below
        IssueType:      issue.Fields.IssueType.Name,
        Priority:       issue.Fields.Priority.Name,
        ProjectKey:     issue.Fields.Project.Key,
        ProjectName:    issue.Fields.Project.Name,
        Updated:        issue.Fields.Updated,
        Created:        issue.Fields.Created,
        WebUrl:         baseUrl + "/browse/" + issue.Key,
        Attachments:    atts,
        Comments:       cmts,
        CommentCount:   issue.Fields.Comment.Total, // Jira total, not len(kept)
        LastCommentAt:  lastAt,
    }
}

func statusCategoryFromKey(k string) string {
    switch k {
    case "new", "indeterminate", "done":
        return k
    default:
        return "new" // D-CACHE-08
    }
}
```

**Note on `statusCategory`:** Phase 1's `IssueFields.Status` currently has only `Name` and `ID` — it does NOT expose `statusCategory.key`. This is a **required Phase 2 extension** to the `IssueFields.Status` struct:

```go
// Addition to client.go IssueFields.Status:
Status struct {
    Name           string `json:"name"`
    ID             string `json:"id"`
    StatusCategory struct {
        Key string `json:"key"` // "new" | "indeterminate" | "done" | "undefined"
    } `json:"statusCategory"`
} `json:"status"`
```

This is purely additive to a JSON-decoded struct — existing Phase 1 tests are unaffected because unknown fields in source JSON decode silently and new Go fields are zero-valued when not present in the fixture. [VERIFIED: Go encoding/json behavior]

String comparison for `lastAt` works because Jira ISO8601 timestamps (`2026-04-15T08:30:00.000+0000`) are **lexicographically sortable** when timezone is consistent, which Jira guarantees. [VERIFIED: ISO8601 spec + Atlassian date format docs]

### Pattern 3: Existing-Cache Reader (best-effort, non-fatal)

```go
// Source: D-FLOW-04 — never fails; any error = empty map
func loadExistingLocalPaths(cachePath string) map[string]string {
    out := map[string]string{}
    data, err := os.ReadFile(cachePath)
    if err != nil {
        return out // file doesn't exist, permission denied, etc. — all OK
    }
    var existing JiraCache
    if err := json.Unmarshal(data, &existing); err != nil {
        return out // malformed cache — treat as empty (do NOT surface error)
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

The key format `issueKey + "::" + attachmentId` avoids collisions if two different issues ever had the same attachment ID (unlikely, but cheap insurance).

### Pattern 4: Cache Path Resolver (mirrors config.go)

```go
func cacheFilePath() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", fmt.Errorf("jira: cannot resolve home dir: %w", err)
    }
    return filepath.Join(home, ".config", "waveterm", "jira-cache.json"), nil
}
```

The planner should also ensure the parent directory exists before the atomic write. `fileutil.AtomicWriteFile` does NOT create parent directories — if `~/.config/waveterm/` does not yet exist on a fresh user, the temp write will fail. Add `os.MkdirAll(filepath.Dir(cachePath), 0o755)` before `AtomicWriteFile`. [VERIFIED: fileutil.go lines 179-194 — `os.WriteFile` on the temp path assumes parent exists]

### Anti-Patterns to Avoid

- **Mixing wire and cache types.** Don't `json:"..."` tag `JiraCacheIssue` to match `IssueFields` and hope Go sorts it out. The shapes diverge on `author` (object→string), `body` (RawMessage→string), `description` (RawMessage→string), `statusCategory` (nested→flat string), `commentCount` (synthetic), `lastCommentAt` (synthetic), `webUrl` (synthetic). Keep them as two distinct struct families.
- **Treating `fields.comment` as a bare array.** It's an object `{total, maxResults, startAt, comments[]}`. Phase 1's `CommentPage` handles this — do not redecode. [VERIFIED: pkg/jira/client.go lines 146-162 + Atlassian REST v3 docs]
- **Keeping the FIRST 10 comments instead of the LAST 10.** Jira returns them oldest-first; the widget UI and user intent is "show recent activity". `raw[len(raw)-10:]` — not `raw[:10]`.
- **Calling `os.Rename` across the temp-file suffix without a guaranteed cleanup.** If `Rename` fails mid-flight, the `.tmp` file lingers. `fileutil.AtomicWriteFile` already removes the tmp on rename failure (fileutil.go line 188) — use it rather than rolling your own.
- **Marshaling before a successful `GetMyself`.** D-ERR-03 requires fail-fast on myself; do it before paginating search to avoid doing 50 API calls only to then fail on accountId resolution.
- **Logging Config or response Body.** T-01-02 threat model — `log.Printf` on errors must not include `err.Error()` for `*APIError` without first confirming it doesn't include body. Phase 1's `APIError.Error()` deliberately omits Body, so `log.Printf("... %v", err)` is safe; but do NOT `log.Printf("... %+v", err)` (that dumps the struct including Body).

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Atomic file write | Manual tmp+rename+cleanup in refresh.go | `fileutil.AtomicWriteFile` | Already exists, handles temp-cleanup on rename failure (lines 188-190), consistent with rest of repo |
| HTTP retry on transient errors | Retry loop around `GetIssue` | Nothing — Phase 5 owns this | Phase 5's rate limiter Transport will be injected via `NewClientWithHTTP`; retries belong there |
| ADF parsing for description / comment body | Regex or string munging on ADF JSON | `jira.ADFToMarkdown` from Phase 1 | Already handles 14 node types, marks, tables, unknown-node skip |
| Comment pagination object decode | Manual JSON decode into `[]Comment` with fallback logic | `IssueFields.Comment` (type `CommentPage`) from Phase 1 | Phase 1's struct matches Jira's wire shape; just read `.Comments` and `.Total` |
| Preserve-localPath data structure | Nested map `map[string]map[string]string` | Flat `map[string]string` with `key+"::"+attId` | Simpler, one lookup not two, no nil-check on inner map |
| `accountId` resolution | Parse email → accountId via some derived rule | `GetMyself(ctx)` → `.AccountID` | Only reliable source; email is NOT the accountId in Jira Cloud |

**Key insight:** Every transformation this phase needs is already decomposable into Phase 1 primitives + `fileutil.AtomicWriteFile` + a single mapping function. Resist the urge to "improve" the `Client` interface — D-FLOW-02 gives exactly the field list to request, and Phase 1's structs already decode that field set.

---

## Runtime State Inventory

This phase creates a new file but does not rename or migrate existing state.

| Category | Items Found | Action Required |
|----------|-------------|------------------|
| Stored data | `~/.config/waveterm/jira-cache.json` may already exist from the legacy Atlassian-MCP-based flow (user memory notes this) | None — D-FLOW-04 explicitly tolerates malformed/missing existing cache; new write overwrites atomically |
| Live service config | None — no Jira-side configuration changes (read-only milestone, JIRA-Out-of-Scope §Write operations) | None |
| OS-registered state | None — no Task Scheduler / pm2 / systemd registrations tied to this refresh | None |
| Secrets/env vars | `apiToken` lives in `~/.config/waveterm/jira.json` (Phase 1) — not read/written by Phase 2 | None |
| Build artifacts | No generated TS types yet (Phase 3's concern); no egg-info / compiled binaries | None |

**Legacy cache compatibility:** If the existing `jira-cache.json` on a dev machine was written by the prior Claude-MCP path, its schema is identical (that's the whole point of D-CACHE-01..08). `loadExistingLocalPaths` will correctly extract any `localPath` values that were set there. If the old file had extra unknown fields, `json.Unmarshal` into `JiraCache` will silently ignore them — safe by default.

---

## Common Pitfalls

### Pitfall 1: Comment author type mismatch (widget breakage)

**What goes wrong:** A naive refresh copies Jira's `comment.author` object into `JiraCacheComment.Author` and the widget renders `{c.author}` as `[object Object]` in React (or fails a JSON comparison even earlier).

**Why it happens:** Jira wire format: `"author": {"accountId": "5b10...", "displayName": "Alice Park", "active": true}`. Widget expects: `"author": "Alice Park"`. Neither Phase 1's `Comment` struct nor a TypeScript type check catches the mismatch — the field is just loosely typed.

**How to avoid:** Explicit flatten in `buildCacheIssue`: `author := c.Author.DisplayName; if author == "" { author = c.Author.AccountID }` — never marshal the nested struct.

**Warning signs:** Widget shows `[object Object]` in comment header; golden-file test shows `{"accountId":"..."}` in the author position.

### Pitfall 2: Comment ordering (keeping the wrong 10)

**What goes wrong:** "Latest 10" becomes "oldest 10" because Jira returns comments **ascending by creation time** within an issue. [VERIFIED: Atlassian dev community confirmation — ascending order, up to 5000]

**Why it happens:** A developer writes `keep := raw[:10]` because that's the natural "take first N" slice.

**How to avoid:** `raw[len(raw)-10:]` (or equivalent guard: `if len(raw) > 10 { keep = raw[len(raw)-10:] } else { keep = raw }`).

**Warning signs:** D-TEST-03 fails — fixture with 15 comments where the last 10 have distinctive IDs returns the first 10.

### Pitfall 3: Empty slices marshaled as `null`

**What goes wrong:** Golden-file comparison (D-TEST-01) fails because `json.Marshal` of an unset `[]JiraCacheAttachment` produces `"attachments": null`, not `"attachments": []`.

**Why it happens:** Go's `encoding/json` marshals a nil slice as JSON `null`. The widget tolerates both (`i.attachments || []`) but the byte-identical golden-file test does not.

**How to avoid:** Always initialize `Attachments: make([]JiraCacheAttachment, 0)` and same for `Comments`, even when appending nothing. Do this **in `buildCacheIssue` itself** — not at the `JiraCache` level.

**Warning signs:** Test fails on `null` vs `[]`; manual `jq` comparison shows `null` in empty arrays.

### Pitfall 4: `statusCategory.key` missing from Phase 1 struct

**What goes wrong:** `buildCacheIssue` always emits `statusCategory: "new"` regardless of actual Jira status, because Phase 1's `IssueFields.Status` has no `statusCategory` sub-field.

**Why it happens:** Phase 1 focused on HTTP client correctness, not cache schema; it defined only `{Name, ID}` on Status. Phase 2 needs the nested `statusCategory.key` for D-CACHE-08.

**How to avoid:** Additive struct change in `client.go` (documented in Pattern 2). The planner should list this as a discrete task, not lump it with `GetMyself`.

**Warning signs:** All issues in the cache show `statusCategory: "new"` in the widget (all appearing in the todo column regardless of real state).

### Pitfall 5: Missing parent directory on fresh install

**What goes wrong:** `fileutil.AtomicWriteFile(cachePath, ...)` fails with "no such file or directory" on a machine where `~/.config/waveterm/` does not yet exist — common on new Windows installs and the first run after a clean profile setup.

**Why it happens:** `os.WriteFile` in `AtomicWriteFile` (fileutil.go line 181) assumes the directory exists. Phase 1's `LoadConfig` only reads — it doesn't create the directory.

**How to avoid:** `os.MkdirAll(filepath.Dir(cachePath), 0o755)` immediately before the atomic write.

**Warning signs:** `jira: write cache: open .../jira-cache.json.tmp: no such file or directory` at Step 5, typically on first ever use.

### Pitfall 6: `/search/jql` has no `total`

**What goes wrong:** Progress callback for `"search"` stage calls `OnProgress("search", pageNum, totalPages)` where `totalPages` is always 0, confusing UI code downstream.

**Why it happens:** Enhanced search is cursor-based — **no `total` field is returned** ([CITED: developer.atlassian.com/.../api-group-issue-search — "This request uses the token-based pagination model. ... Unlike the offset-based model, the response does not include `total`"]).

**How to avoid:** For `"search"` stage, pass `total = 0` and document that a 0 total means "unknown; use `current` as a running count". Phase 3's UI will show "{current} issues found..." for this stage; "{current}/{total} fetched" for `"fetch"`.

**Warning signs:** Downstream UI divides by zero or shows "0%" progress throughout search.

### Pitfall 7: Windows atomic rename over an open file handle

**What goes wrong:** On Windows, `os.Rename` fails with `ERROR_SHARING_VIOLATION` if the destination file is currently open (e.g., a text editor holds it open for reading).

**Why it happens:** Windows rename semantics differ from POSIX. POSIX atomic rename replaces the target even if open; Windows does not (by default). The widget reads `jira-cache.json` via `RpcApi.FileReadCommand` but **closes the handle immediately after read** — so the race is narrow but real.

**How to avoid:** Accept the narrow race for now — the widget's `FileReadCommand` is synchronous and releases immediately; concurrent refreshes are the only realistic collision, and sequential refresh (D-CONC-01) precludes that. If `Rename` fails on Windows, **do not silently succeed** — return the wrapped error so the caller logs it. `fileutil.AtomicWriteFile` already surfaces the rename error correctly (lines 187-193).

**Warning signs:** Intermittent "The process cannot access the file because it is being used by another process" on Windows during rapid refresh cycles. Phase 5 may want to add a brief retry loop on this specific Windows error; Phase 2 does not.

[VERIFIED: Go stdlib docs on `os.Rename` — "If newpath already exists and is not a directory, Rename replaces it. OS-specific restrictions may apply when oldpath and newpath are in different directories." Windows sharing-violation behavior documented in Microsoft Windows API docs for `MoveFileExW`.]

### Pitfall 8: `err.Error()` on `*APIError` is safe; `%+v` is not

**What goes wrong:** Per-issue failure logging leaks response body or auth header into logs via `log.Printf("GetIssue %s failed: %+v", key, err)`.

**Why it happens:** `%v` on `*APIError` calls `Error()` which is scrubbed (client.go: `fmt.Sprintf("jira: HTTP %d %s %s", ...)`). But `%+v` with a pointer-to-struct reveals all fields, including `Body` which is 1KB of the response. Even `%#v` or structured loggers would leak it.

**How to avoid:** Use `%v` exclusively (or `err.Error()` explicitly). Never use `%+v`, `%#v`, or `fmt.Printf` with verbose verbs on Jira-package errors.

**Warning signs:** Logs contain JSON fragments like `{"errorMessages":[...]}` or base64-looking strings.

---

## Code Examples

### Example 1: httptest fixture server for all three endpoints

```go
// refresh_test.go — handler multiplexes on URL path
// Source: pattern from pkg/jira/client_test.go TestSearchIssues_Pagination,
// extended to cover GET /myself, GET /issue/{key}, POST /search/jql in one server.

func newRefreshTestServer(t *testing.T, issues []string, issueFixtures map[string]string) *httptest.Server {
    t.Helper()
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        switch {
        case r.URL.Path == "/rest/api/3/myself" && r.Method == http.MethodGet:
            w.WriteHeader(200)
            fmt.Fprint(w, `{"accountId":"acc-dev-os","displayName":"Dev","emailAddress":"dev@example.com"}`)

        case r.URL.Path == "/rest/api/3/search/jql" && r.Method == http.MethodPost:
            // Single-page response for simplicity; pagination tested separately.
            refs := make([]string, len(issues))
            for i, k := range issues {
                refs[i] = fmt.Sprintf(`{"id":"%d","key":"%s"}`, 1000+i, k)
            }
            w.WriteHeader(200)
            fmt.Fprintf(w, `{"issues":[%s],"isLast":true,"nextPageToken":""}`, strings.Join(refs, ","))

        case strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/") && r.Method == http.MethodGet:
            key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
            body, ok := issueFixtures[key]
            if !ok {
                w.WriteHeader(404)
                fmt.Fprint(w, `{"errorMessages":["not found"]}`)
                return
            }
            w.WriteHeader(200)
            fmt.Fprint(w, body)

        default:
            t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
            w.WriteHeader(500)
        }
    }))
}
```

### Example 2: Minimal issue fixture covering every cache field

```go
// Source: D-CACHE-03 field list + Jira REST v3 response docs
// This JSON decodes into Phase 1's *Issue struct without loss.

const issueFixtureITSM1 = `{
  "id": "10001",
  "key": "ITSM-1",
  "fields": {
    "summary": "Example issue",
    "description": {"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world"}]}]},
    "status": {
      "name": "In Progress",
      "id": "3",
      "statusCategory": {"key": "indeterminate"}
    },
    "issuetype": {"name": "Bug", "id": "1", "subtask": false},
    "priority": {"name": "High", "id": "2"},
    "project": {"key": "ITSM", "name": "IT Service Management", "id": "100"},
    "created": "2026-04-01T08:00:00.000+0000",
    "updated": "2026-04-14T15:30:00.000+0000",
    "attachment": [
      {
        "id": "att1",
        "filename": "screenshot.png",
        "mimeType": "image/png",
        "size": 12345,
        "created": "2026-04-10T10:00:00.000+0000",
        "content": "https://example.atlassian.net/rest/api/3/attachment/content/att1",
        "author": {"accountId": "acc-user1", "displayName": "User One"}
      }
    ],
    "comment": {
      "total": 2,
      "maxResults": 2,
      "startAt": 0,
      "comments": [
        {
          "id": "c1",
          "author": {"accountId": "acc-user1", "displayName": "User One"},
          "body": {"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"First"}]}]},
          "created": "2026-04-02T09:00:00.000+0000",
          "updated": "2026-04-02T09:00:00.000+0000"
        },
        {
          "id": "c2",
          "author": {"accountId": "acc-user2", "displayName": "User Two"},
          "body": {"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"Second"}]}]},
          "created": "2026-04-03T10:00:00.000+0000",
          "updated": "2026-04-03T10:30:00.000+0000"
        }
      ]
    }
  }
}`
```

### Example 3: Golden-file assertion pattern (D-TEST-01)

```go
// Source: D-TEST-01 — byte-identical JSON output is the top-level success criterion.

func TestRefresh_GoldenFile(t *testing.T) {
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)            // POSIX
    t.Setenv("USERPROFILE", tmp)     // Windows — os.UserHomeDir() uses USERPROFILE on Windows
    if err := os.MkdirAll(filepath.Join(tmp, ".config", "waveterm"), 0o755); err != nil {
        t.Fatal(err)
    }

    srv := newRefreshTestServer(t,
        []string{"ITSM-1"},
        map[string]string{"ITSM-1": issueFixtureITSM1},
    )
    defer srv.Close()

    cfg := Config{
        BaseUrl:  srv.URL,
        CloudId:  "cloud-xxx",
        Email:    "dev@example.com",
        ApiToken: "tok",
        Jql:      "assignee = currentUser()",
        PageSize: 50,
    }
    // Refresh uses NewClient internally; override by either:
    // (a) exposing a package-level var `httpClient = http.DefaultClient` and overriding from tests, OR
    // (b) having Refresh accept an optional *http.Client in RefreshOpts.
    // Option (b) is cleaner — planner decides.
    report, err := Refresh(context.Background(), RefreshOpts{Config: cfg /*, HTTPClient: srv.Client() */})
    if err != nil {
        t.Fatalf("refresh: %v", err)
    }
    if report.IssueCount != 1 {
        t.Errorf("IssueCount: got %d want 1", report.IssueCount)
    }

    got, err := os.ReadFile(report.CachePath)
    if err != nil {
        t.Fatal(err)
    }
    want, err := os.ReadFile("testdata/cache.golden.json")
    if err != nil {
        t.Fatal(err)
    }
    // fetchedAt is time-dependent — either normalize before compare, or use
    // a clock injection in RefreshOpts. Simplest: regex-replace the timestamp
    // field before diffing.
    gotN := normalizeFetchedAt(got)
    wantN := normalizeFetchedAt(want)
    if !bytes.Equal(gotN, wantN) {
        t.Errorf("cache mismatch (showing diff):\n--- want\n%s\n--- got\n%s", wantN, gotN)
    }
}
```

**Design choice for test-only HTTP injection:** The current Phase 1 `Refresh`-free surface only exposes `NewClient(cfg)` and `NewClientWithHTTP(cfg, hc)`. Phase 2 needs to call `NewClient` inside `Refresh`, so tests cannot inject `srv.Client()` unless we add either:

1. **Recommended:** An optional `HTTPClient *http.Client` on `RefreshOpts`. When non-nil, `Refresh` uses `NewClientWithHTTP`; otherwise `NewClient`. Minimal public-surface change, test-friendly.
2. **Alternative:** Package-level `var httpClientFactory = http.DefaultClient` that tests reach in and replace. Less clean but zero API surface change.

Planner's discretion between (1) and (2). Recommendation: (1), because Phase 5 will similarly want to inject a rate-limiting Transport.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` + `net/http/httptest` (Go 1.25.6) |
| Config file | none — `go test` built-in |
| Quick run command | `go test ./pkg/jira -run TestRefresh -count=1` |
| Full suite command | `go test ./pkg/jira -count=1` |

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|--------------|
| JIRA-06 | Keep latest 10 comments, truncate body at 2000 chars, set `truncated:true` | unit | `go test ./pkg/jira -run TestRefresh_CommentCapAndTruncation -count=1` | ❌ Wave 0 |
| JIRA-06 | `commentCount` reflects API total (may exceed 10) | unit | `go test ./pkg/jira -run TestRefresh_CommentCountFromTotal -count=1` | ❌ Wave 0 |
| JIRA-06 | `lastCommentAt` = max(updated, created) across kept comments | unit | `go test ./pkg/jira -run TestRefresh_LastCommentAt -count=1` | ❌ Wave 0 |
| JIRA-07 | Cache JSON byte-identical to golden file (ROADMAP Success #1) | unit | `go test ./pkg/jira -run TestRefresh_GoldenFile -count=1` | ❌ Wave 0 |
| JIRA-07 | Atomic write: temp file + rename, no partial file on error | unit | `go test ./pkg/jira -run TestRefresh_AtomicWriteFailureLeavesOldCache -count=1` | ❌ Wave 0 |
| JIRA-07 | Existing `localPath` preserved across refresh (ROADMAP Success #4) | unit | `go test ./pkg/jira -run TestRefresh_PreserveLocalPath -count=1` | ❌ Wave 0 |
| JIRA-07 | ADF description converted; null description → empty string | unit | `go test ./pkg/jira -run TestRefresh_NullDescription -count=1` | ❌ Wave 0 |
| JIRA-07 | `statusCategory` mapping (new/indeterminate/done + unknown → new) | unit | `go test ./pkg/jira -run TestRefresh_StatusCategoryMapping -count=1` | ❌ Wave 0 |
| JIRA-07 | `/myself` failure is fatal; search failure is fatal; per-issue failure is skipped | unit | `go test ./pkg/jira -run TestRefresh_ErrorClassification -count=1` | ❌ Wave 0 |
| JIRA-07 | Widget round-trip — TS-decoded cache has every expected field (ROADMAP Success #5) | integration-smoke | Manual: run `wsh jira refresh`-equivalent, open widget, verify issues render | ❌ Phase 3 gate |
| — | Progress callback invoked with correct stages | unit | `go test ./pkg/jira -run TestRefresh_ProgressCallback -count=1` | ❌ Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./pkg/jira -count=1` (~1 sec for the whole jira package — no external I/O)
- **Per wave merge:** `go test ./... -count=1` from repo root (full repo suite)
- **Phase gate:** All `TestRefresh_*` green + widget smoke check before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `pkg/jira/refresh_test.go` — covers JIRA-06, JIRA-07 (all automated criteria above)
- [ ] `pkg/jira/testdata/cache.golden.json` — byte-exact expected output for one-issue fixture (D-TEST-01)
- [ ] `pkg/jira/testdata/cache-with-localpath.json` — pre-seeded cache for D-TEST-02
- [ ] `pkg/jira/refresh.go` — the production code under test (obviously)
- [ ] `pkg/jira/cache_types.go` — struct definitions (or inline in refresh.go per planner choice)
- [ ] Framework install: none — `go test` is built in

---

## Security Domain

`security_enforcement` is not set to `false` in `.planning/config.json`, so treat as enabled.

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|------------------|
| V2 Authentication | yes | Reuse Phase 1's `Basic base64(email:apiToken)` via `setCommonHeaders` — Phase 2 does not re-implement auth |
| V3 Session Management | no | Stateless HTTP; no cookies or server-side sessions |
| V4 Access Control | no | Read-only against authenticated user's own issues (Jira enforces; we don't do local ACL) |
| V5 Input Validation | yes — **light** | JSON decode into typed structs (Phase 1 patterns); no SQL, no shell; JQL is user-provided but sent verbatim to Jira (Jira parses, not us) |
| V6 Cryptography | no | No new crypto — `apiToken` handling unchanged from Phase 1 |
| V7 Error Handling & Logging | yes — **critical** | See Pitfall 8 + threat model below |
| V8 Data Protection | yes | `apiToken` never written to cache; `jira-cache.json` contains only public-at-site-scope data (issues user already has access to) |
| V14 Configuration | yes | Cache path hardcoded per D-CACHE-01; no env-var injection of paths |

### Known Threat Patterns for this phase

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| Log leak of `apiToken` or `Authorization` header | Information Disclosure | **Rule:** only use `%v` on errors; never `%+v`. `*APIError.Error()` already scrubs Body. Never log `opts.Config` or any field of it. (Phase 1 threat T-01-02, extended to refresh.) |
| Log leak of response Body (comments may contain sensitive team info) | Information Disclosure | Same as above. Per-issue failure log: `log.Printf("GetIssue %s failed: %v", key, err)` — `%v` only. |
| Malformed existing cache → panic → corrupted state | DoS / Tampering | `loadExistingLocalPaths` catches `json.Unmarshal` error and returns empty map; no panic path |
| Temp file (`jira-cache.json.tmp`) left world-readable with stale data | Information Disclosure | `0o644` matches existing `wconfig` convention (user-readable only on Windows via ACL mapping); acceptable. If Waveterm ever moves to stricter perms, update constant in one place. |
| `apiToken` leaked via error Body (Jira echoing request headers) | Information Disclosure | Jira does NOT echo Authorization in error bodies per spec; but `APIError.Body` is capped at 1KB by Phase 1 and scrubbed from `Error()`. Defense in depth. |
| JSON parsing of attacker-controlled server response causing extreme memory use | DoS | `http.Client` timeout 30s + Go's streaming `json.Decoder` + 1KB error body cap. Response size for a 50-issue page is bounded. Not a realistic attack from legitimate Jira. |
| Path traversal via attachment filename into cache | Tampering | Cache stores `filename` as a string; Phase 2 never opens it as a path. Phase 5's `wsh jira download` will need to sanitize — not this phase. |

---

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Use Claude/MCP to fetch issues and write cache | Go backend direct REST → atomic write | Milestone v1.0 | Removes Claude dependency; this phase delivers the core shift |
| `/rest/api/3/search` (legacy, `startAt/maxResults`) | `POST /rest/api/3/search/jql` (enhanced, `nextPageToken`) | Atlassian deprecation notice 2024–2025 | Phase 1 already on enhanced; Phase 2 inherits no-total caveat |
| `total`-based progress UI | Running-count UI for search, `N/total` for fetch | Cursor-based search has no total | Phase 3 UI must handle total=0 |

**Deprecated / outdated:**
- Legacy `/rest/api/3/search` endpoint — not used (Phase 1 decision D-01)
- `CommentPage.maxResults` / `startAt` — returned by Jira but **ignored** by us (we cap at 10 locally)

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | ISO8601 timestamps in Jira responses are lexicographically comparable for `max(updated, created)` | Pattern 2 | LOW — Jira always uses same zone format (`+0000`) per spec; a mixed-zone response would skew ordering by seconds, acceptable for `lastCommentAt` |
| A2 | The existing (legacy-written) `jira-cache.json` on dev machines has `truncated: true` only (never `truncated: false`) for comments | cache_types Truncated omitempty | LOW — If legacy emitted `truncated: false` explicitly, golden-file test fails. Fix: match whatever the actual dev-machine cache shows; if unknown, prefer `omitempty` and regenerate the golden file from a real refresh |
| A3 | Jira's `attachment.content` URL is always in the site-pattern (`<site>.atlassian.net`) format, never in `api.atlassian.com` format | Pattern 2 Attachments | LOW — [VERIFIED: Atlassian docs example + community confirmation]. If ever not, treat as bug and rebuild URL from `baseUrl + "/rest/api/3/attachment/content/" + id` |
| A4 | `comment.author.displayName` is non-empty for normal users; empty only when Atlassian privacy settings hide it | Pattern 2 Author flatten | LOW — fallback to `accountId` in that case makes the widget show a cryptic ID, but does not break rendering |
| A5 | `fileutil.AtomicWriteFile` is stable public API within the repo (not about to be renamed/removed) | Don't Hand-Roll | LOW — used by wconfig and other core paths; no TODO/deprecation markers in the file |

**All claims not in this table are `[VERIFIED]` (codebase grep, file read, or Atlassian official doc URL cited) or `[CITED]` (external URL, non-negotiable fact like ISO8601 format).**

---

## Open Questions

1. **Should `Refresh` accept `*http.Client` injection directly, or should tests use a package-level factory variable?**
   - What we know: Phase 1 exposed `NewClientWithHTTP(cfg, hc)` for tests.
   - What's unclear: Whether to surface HTTP injection on `RefreshOpts` (public API change) or keep it internal (test-only hook).
   - Recommendation: Add `RefreshOpts.HTTPClient *http.Client` (nillable → uses default). Phase 5 will want this for the rate-limiting Transport anyway. Minimal surface cost for future-proofing.

2. **Golden-file determinism: where to inject `time.Now()`?**
   - What we know: `fetchedAt` changes on every run.
   - What's unclear: Inject `func() time.Time` clock, or post-process the golden file with a regex?
   - Recommendation: Post-process for the initial golden test (simpler, test-only); introduce a `Clock` seam only if more timestamp-dependent fields appear.

3. **Should existing-cache preservation survive schema evolution (e.g., old cache had a field we removed)?**
   - What we know: D-FLOW-04 says "malformed = empty map".
   - What's unclear: Is a schema-mismatched-but-still-valid-JSON cache "malformed"? (Go's `json.Unmarshal` will silently ignore unknown fields and zero-value missing ones — which means `LocalPath` extraction still works even if e.g. a `size` field type changed.)
   - Recommendation: No action needed — Go's lenient decode handles this naturally. Document in a `refresh.go` comment.

4. **Progress callback cadence for `"fetch"` — before or after each `GetIssue`?**
   - What we know: D-PROG-01 says `current, total` = `issue index / total issues`.
   - What's unclear: Does `current` = "started fetching issue N" (pre-call) or "finished fetching issue N" (post-call)?
   - Recommendation: Post-call (`cb("fetch", i+1, total)` after `GetIssue` returns — whether success or skip). Matches how most progress UIs read.

---

## Sources

### Primary (HIGH confidence)

- **Phase 1 source code** (verified by Read tool):
  - `pkg/jira/client.go` — `Client`, `Issue`, `IssueFields`, `Attachment`, `Comment`, `CommentPage`, `setCommonHeaders`, `doJSON`
  - `pkg/jira/adf.go` — `ADFToMarkdown`
  - `pkg/jira/errors.go` — `APIError`, sentinels, `parseRetryAfter`
  - `pkg/jira/config.go` — `Config`, `LoadConfig`, `DefaultJQL`, `DefaultPageSize`
  - `pkg/jira/client_test.go` — httptest pattern to mirror
- **Widget source** (verified by Read tool): `frontend/app/view/jiratasks/jiratasks.tsx` lines 61, 116-160, 469-510, 540-547, 795-808 — defines `JiraCache`, `JiraIssue`, `JiraAttachment`, `JiraComment`, `CACHE_PATH`, consumer semantics
- **In-repo helper** (verified by Read tool): `pkg/util/fileutil/fileutil.go` lines 179-194 — `AtomicWriteFile`
- **Atlassian official docs**:
  - https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-myself/ — `/myself` response shape, `accountId` field
  - https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-attachments/#api-rest-api-3-attachment-content-id-get — attachment.content URL in tenant site pattern
  - https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-post — enhanced search response has no `total`

### Secondary (MEDIUM confidence — web search cross-verified with official)

- https://community.developer.atlassian.com/t/deprecation-notice-limiting-the-number-of-comments-returned-by-get-issue-rest-api-and-issue-deleted-events/42138 — GET issue returns comments ascending up to 5000 (confirms ordering for Pitfall 2)
- Go `os.Rename` on Windows behavior — microsoft.com WinAPI docs + Go stdlib `os` package notes

### Tertiary (LOW confidence — WebFetch output contradicted primary; discarded)

- A WebFetch query against issue-group-issues mistakenly reported `fields.comment` as a "flat array not paginated" — **discarded** in favor of Phase 1's `CommentPage` struct shape, which is in line with the deprecation-notice thread above and the issue-comments group docs.

---

## Metadata

**Confidence breakdown:**
- Cache schema: HIGH — every field cross-verified against widget source (authoritative consumer)
- Refresh flow: HIGH — all decisions locked in CONTEXT.md; only Claude's-discretion items are HTTP injection style (A1) and file split (documented)
- Pitfalls: HIGH for Pitfall 1-6 (verified via code + docs); MEDIUM for Pitfall 7 (Windows rename edge cases documented but not exercised this milestone)
- Security: HIGH — Phase 1 threat model extends cleanly; no new auth surface

**Research date:** 2026-04-15
**Valid until:** 2026-05-15 (30 days — Jira Cloud REST v3 is stable; enhanced-search cutover is already complete; next refresh of research warranted only if Atlassian deprecates an endpoint used here)

---
phase: 02-cache-orchestration
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - pkg/jira/client.go
  - pkg/jira/cache_types.go
  - pkg/jira/refresh_test.go
  - pkg/jira/testdata/cache.golden.json
  - pkg/jira/testdata/cache-with-localpath.seed.json
  - pkg/jira/testdata/issue-itsm1.json
  - pkg/jira/testdata/issue-comment-cap.json
  - pkg/jira/testdata/issue-comment-truncation.json
  - pkg/jira/testdata/issue-null-description.json
autonomous: true
requirements: [JIRA-06, JIRA-07]

must_haves:
  truths:
    - "IssueFields.Status struct gains nested StatusCategory.Key field — additive, decodes existing Phase 1 fixtures unchanged (D-CACHE-08, RESEARCH Pitfall 4)"
    - "Client.GetMyself(ctx) (*Myself, error) calls GET {baseUrl}/rest/api/3/myself with Basic auth headers and returns AccountId/DisplayName/EmailAddress (D-FLOW-03)"
    - "Myself struct exported from pkg/jira with json tags accountId, displayName, emailAddress"
    - "JiraCache, JiraCacheIssue, JiraCacheAttachment, JiraCacheComment structs exist in cache_types.go with json tags matching widget consumer shape (D-CACHE-02..07, RESEARCH §Cache Type Definitions)"
    - "JiraCacheComment.Truncated carries json tag `truncated,omitempty` so unset comments do not emit the field (D-CACHE-06, RESEARCH A2)"
    - "refresh_test.go exists with 9 failing tests covering every JIRA-06/JIRA-07 criterion from RESEARCH §Phase Requirements → Test Map"
    - "testdata/cache.golden.json encodes the expected byte-exact cache output for the single-issue ITSM-1 fixture (D-TEST-01, ROADMAP Success #1)"
    - "testdata/cache-with-localpath.seed.json encodes a pre-existing cache with non-empty localPath that Plan 02 must preserve (D-FLOW-04, D-TEST-02)"
    - "All refresh_test.go tests COMPILE and FAIL with a clear 'undefined: Refresh' or equivalent missing-symbol error — Nyquist RED discipline"
    - "All four Phase 1 client tests still pass after the additive Status.StatusCategory struct change"
  artifacts:
    - path: "pkg/jira/client.go"
      provides: "Extended IssueFields.Status with nested StatusCategory.Key + new Myself type + Client.GetMyself method"
      contains: "func (c *Client) GetMyself"
    - path: "pkg/jira/cache_types.go"
      provides: "JiraCache, JiraCacheIssue, JiraCacheAttachment, JiraCacheComment — the widget-authoritative on-disk schema"
      contains: "type JiraCache struct"
    - path: "pkg/jira/refresh_test.go"
      provides: "Failing TDD RED tests that specify every Plan 02 behavior"
      contains: "func TestRefresh_GoldenFile"
    - path: "pkg/jira/testdata/cache.golden.json"
      provides: "Byte-exact expected cache content for single-issue fixture"
      contains: "\"cloudId\""
    - path: "pkg/jira/testdata/cache-with-localpath.seed.json"
      provides: "Pre-existing cache seed for preserve-localPath test (D-TEST-02)"
      contains: "\"localPath\""
    - path: "pkg/jira/testdata/issue-itsm1.json"
      provides: "Canonical issue fixture covering every cache field (RESEARCH §Example 2)"
      contains: "\"ITSM-1\""
    - path: "pkg/jira/testdata/issue-comment-cap.json"
      provides: "Issue fixture with 15 comments (kept=10, total=15) for D-TEST-03"
      contains: "\"total\": 15"
    - path: "pkg/jira/testdata/issue-comment-truncation.json"
      provides: "Issue fixture with a 2500-char comment body (truncated to 2000)"
      contains: "\"truncated\""
    - path: "pkg/jira/testdata/issue-null-description.json"
      provides: "Issue fixture with description=null to verify empty-string fallback (D-CACHE-04)"
      contains: "\"description\": null"
  key_links:
    - from: "refresh_test.go"
      to: "pkg/jira.Refresh (NOT YET DEFINED — symbol missing in Wave 1)"
      via: "import \"github.com/wavetermdev/waveterm/pkg/jira\" (white-box: package jira test file)"
      pattern: "Refresh\\(context"
    - from: "refresh_test.go fixture server"
      to: "/rest/api/3/myself, /rest/api/3/search/jql, /rest/api/3/issue/{key}"
      via: "httptest.NewServer with http.HandlerFunc path multiplexer (RESEARCH §Example 1)"
      pattern: "httptest\\.NewServer"
    - from: "Client.GetMyself"
      to: "GET {baseUrl}/rest/api/3/myself"
      via: "setCommonHeaders + doJSON (reuses Phase 1 infrastructure)"
      pattern: "/rest/api/3/myself"
    - from: "cache_types.go structs"
      to: "widget interface in frontend/app/view/jiratasks/jiratasks.tsx lines 116-160"
      via: "field-name and type isomorphism (Go json tags ↔ TS field names)"
      pattern: "json:\"(cloudId|baseUrl|accountId|fetchedAt|issues|statusCategory|lastCommentAt|commentCount)\""
---

<objective>
Prepare every piece of scaffolding Plan 02 needs to turn RED → GREEN:
(1) one additive struct change + one new method on Client,
(2) the cache-shape type family distinct from the wire-shape types,
(3) the full `refresh_test.go` suite with fixtures and golden file,
all written to FAIL until Plan 02 implements `refresh.go`.

Purpose: Nyquist TDD discipline — tests exist before the code they verify.
Every D-CACHE / D-FLOW / D-PROG / D-ERR decision gets a test asserting it,
so Plan 02's implementation is a mechanical "make these tests green"
exercise with no room to drift from CONTEXT.md.

Output:
- One additive edit to `pkg/jira/client.go` (Status.StatusCategory + Myself + GetMyself)
- Four new files in `pkg/jira/` (cache_types.go, refresh_test.go, testdata/*.json)
- Five JSON fixture files under `pkg/jira/testdata/`
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/02-cache-orchestration/02-CONTEXT.md
@.planning/phases/02-cache-orchestration/02-RESEARCH.md
@pkg/jira/client.go
@pkg/jira/client_test.go
@pkg/jira/config.go
@pkg/jira/adf.go
@pkg/jira/errors.go
@frontend/app/view/jiratasks/jiratasks.tsx

<interfaces>
<!-- Phase 1 primitives this plan EXTENDS (does not redefine). -->

From pkg/jira/client.go (Phase 1, already shipped):
```go
type Client struct { /* ... */ }
func NewClient(cfg Config) *Client
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client
func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error)
func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error)
func (c *Client) setCommonHeaders(req *http.Request)   // unexported, reused by GetMyself
func (c *Client) doJSON(req *http.Request, out any) error // unexported, reused by GetMyself

type Issue struct {
    ID     string
    Key    string
    Self   string
    Fields IssueFields
}

type IssueFields struct {
    Summary     string
    Description json.RawMessage
    Status      struct {
        Name string `json:"name"`
        ID   string `json:"id"`
        // THIS PLAN ADDS: StatusCategory struct { Key string `json:"key"` } `json:"statusCategory"`
    } `json:"status"`
    IssueType   struct{ Name, ID string; Subtask bool } `json:"issuetype"`
    Priority    struct{ Name, ID string }                 `json:"priority"`
    Project     struct{ Key, Name, ID string }            `json:"project"`
    Created     string
    Updated     string
    Attachment  []Attachment
    Comment     CommentPage
}

type Attachment struct {
    ID, Filename, MimeType, Created, Content, Thumbnail string
    Size int64
    Author struct{ AccountID, DisplayName string }
}

type CommentPage struct {
    Total    int       `json:"total"`
    Comments []Comment `json:"comments"`
}

type Comment struct {
    ID string
    Author struct{ AccountID, DisplayName string }
    Body    json.RawMessage
    Created string
    Updated string
}
```

From pkg/jira/config.go (Phase 1):
```go
type Config struct { BaseUrl, CloudId, Email, ApiToken, Jql string; PageSize int }
const DefaultPageSize = 50
```

From pkg/jira/adf.go (Phase 1): `func ADFToMarkdown(raw json.RawMessage) (string, error)`
From pkg/jira/errors.go (Phase 1): `*APIError` with scrubbed `Error()` — use `%v` only, never `%+v` (T-01-02 / RESEARCH Pitfall 8)
</interfaces>

<widget_schema>
<!-- READ-ONLY reference — frontend/app/view/jiratasks/jiratasks.tsx lines 116-160.
     cache_types.go must match THIS shape byte-for-byte in json output. -->

interface JiraAttachment { id, filename, mimeType: string; size: number; localPath, webUrl: string }
interface JiraComment    { id, author, created, updated, body: string; truncated?: boolean }
interface JiraIssue      { key, id, summary, description, status, statusCategory, issueType,
                           priority, projectKey, projectName, updated, created, webUrl: string;
                           attachments: JiraAttachment[]; comments: JiraComment[];
                           commentCount: number; lastCommentAt: string }
interface JiraCache      { cloudId, baseUrl, accountId, fetchedAt: string; issues: JiraIssue[] }
</widget_schema>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Extend client.go with Status.StatusCategory + Myself + GetMyself (additive only)</name>
  <files>pkg/jira/client.go</files>
  <action>
Make three additive changes to `pkg/jira/client.go`. NO other lines change. Purpose: close Phase 1 gaps flagged in RESEARCH Pitfall 4 + D-FLOW-03, without touching any tested Phase 1 behavior.

Change 1 — Extend IssueFields.Status with StatusCategory (RESEARCH Pitfall 4 fix):

Current (lines 103-106):
```go
Status      struct {
    Name string `json:"name"`
    ID   string `json:"id"`
} `json:"status"`
```

Replace with:
```go
Status struct {
    Name           string `json:"name"`
    ID             string `json:"id"`
    StatusCategory struct {
        Key string `json:"key"` // "new" | "indeterminate" | "done" | "undefined" (D-CACHE-08)
    } `json:"statusCategory"`
} `json:"status"`
```

This is a pure struct extension — Go's `encoding/json` decodes missing fields as zero values, so every existing Phase 1 test fixture remains valid. Unknown JSON fields continue to be silently ignored. NO test in client_test.go is modified.

Change 2 — Add Myself type. Insert AFTER the `Comment` struct definition (after line 162). Place BEFORE the `searchRequest` private type:

```go
// Myself is the response shape of GET /rest/api/3/myself. Only AccountID is
// needed for the cache schema (D-CACHE-02, D-FLOW-03); other fields are
// decoded for future use (e.g. empty-state "Hello {displayName}" UX in Phase 4).
// Source: https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-myself/
type Myself struct {
    AccountID    string `json:"accountId"`
    DisplayName  string `json:"displayName"`
    EmailAddress string `json:"emailAddress"`
}
```

Change 3 — Add GetMyself method. Insert AFTER the `GetIssue` method (after line 234, before `setCommonHeaders`):

```go
// GetMyself calls GET /rest/api/3/myself and returns the authenticated user's
// identity. Per D-ERR-03 the refresh orchestrator treats any error as fatal
// (we cannot emit a correct cache schema without accountId).
//
// Uses the same setCommonHeaders + doJSON path as every other Client method,
// so Basic auth, User-Agent, Accept, 4xx/5xx -> *APIError classification
// all work identically to SearchIssues / GetIssue.
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

Do NOT: add new imports (all required packages — `context`, `fmt`, `net/http` — are already imported). Do NOT: touch Phase 1 test fixtures. Do NOT: refactor setCommonHeaders / doJSON / any existing method.

Naming note: Keep `AccountID` (not `AccountId`) on the struct field — matches Phase 1's `Attachment.Author.AccountID` convention. The json tag is `accountId` either way, so the wire format is unchanged.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm && go build ./pkg/jira/... && go test ./pkg/jira -run 'TestAuthHeader|TestSearchIssues|TestGetIssue|TestLoadConfig|TestADF' -count=1</automated>
  </verify>
  <done>
client.go compiles. All Phase 1 tests still pass (TestAuthHeader_*, TestSearchIssues_*, TestGetIssue_*, TestLoadConfig_*, TestADF*). `go vet ./pkg/jira/...` is clean. `grep 'func (c \*Client) GetMyself' pkg/jira/client.go` produces one match. `grep 'StatusCategory struct' pkg/jira/client.go` produces one match.
  </done>
</task>

<task type="auto">
  <name>Task 2: Create cache_types.go with the widget-authoritative on-disk schema</name>
  <files>pkg/jira/cache_types.go</files>
  <action>
Create `pkg/jira/cache_types.go` containing the cache-shape type family. These are DISTINCT from `client.go`'s wire-shape types (see RESEARCH §Anti-Patterns — "Mixing wire and cache types").

File contents (exact):

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

// This file defines the on-disk cache schema — the shape that
// ~/.config/waveterm/jira-cache.json holds and that the widget at
// frontend/app/view/jiratasks/jiratasks.tsx (lines 116-160) consumes.
//
// These types are deliberately kept separate from the wire-format types in
// client.go (Issue, IssueFields, Attachment, Comment) because the shapes
// diverge on several fields after ADF conversion and author flattening:
//
//   - Comment.Author is an object on the wire, a string in the cache.
//   - Description / Comment.Body are ADF JSON on the wire, markdown strings
//     in the cache.
//   - StatusCategory is a nested struct on the wire, a flat string in the
//     cache (mapped via statusCategoryFromKey; unknown → "new" per D-CACHE-08).
//   - commentCount / lastCommentAt / webUrl are synthetic — computed by
//     refresh.go's buildCacheIssue, not present on the wire.
//
// Every field and json tag here corresponds 1:1 to the TypeScript interface
// the widget declares. Changing a tag here is a widget-breaking change.

// JiraCache is the top-level JSON object at ~/.config/waveterm/jira-cache.json
// (D-CACHE-01, D-CACHE-02).
type JiraCache struct {
    CloudId   string           `json:"cloudId"`
    BaseUrl   string           `json:"baseUrl"`
    AccountId string           `json:"accountId"`
    FetchedAt string           `json:"fetchedAt"` // ISO8601 UTC, e.g. "2026-04-15T08:30:00Z"
    Issues    []JiraCacheIssue `json:"issues"`
}

// JiraCacheIssue is the cache representation of one Jira issue (D-CACHE-03).
// Every field maps to the widget's JiraIssue interface.
type JiraCacheIssue struct {
    Key            string                `json:"key"`
    ID             string                `json:"id"`
    Summary        string                `json:"summary"`
    Description    string                `json:"description"`    // ADF → markdown; "" when null (D-CACHE-04)
    Status         string                `json:"status"`         // status.name
    StatusCategory string                `json:"statusCategory"` // "new" | "indeterminate" | "done" (D-CACHE-08)
    IssueType      string                `json:"issueType"`      // issuetype.name
    Priority       string                `json:"priority"`       // priority.name, "" if none
    ProjectKey     string                `json:"projectKey"`
    ProjectName    string                `json:"projectName"`
    Updated        string                `json:"updated"`
    Created        string                `json:"created"`
    WebUrl         string                `json:"webUrl"` // baseUrl + "/browse/" + key
    Attachments    []JiraCacheAttachment `json:"attachments"`    // ALWAYS non-nil (RESEARCH Pitfall 3)
    Comments       []JiraCacheComment    `json:"comments"`       // ALWAYS non-nil
    CommentCount   int                   `json:"commentCount"`   // wire total, may exceed len(Comments) (D-CACHE-07)
    LastCommentAt  string                `json:"lastCommentAt"`  // max(updated,created) across kept; "" if none
}

// JiraCacheAttachment is the cache representation of an issue attachment
// (D-CACHE-05). Metadata only — binary content is never embedded.
type JiraCacheAttachment struct {
    ID        string `json:"id"`
    Filename  string `json:"filename"`
    MimeType  string `json:"mimeType"`
    Size      int64  `json:"size"`
    LocalPath string `json:"localPath"`            // "" default; preserved from prior cache (D-FLOW-04)
    WebUrl    string `json:"webUrl"`               // site-pattern URL (D-CACHE-05, RESEARCH A3)
}

// JiraCacheComment is the cache representation of one comment (D-CACHE-06).
// Body is markdown (ADF-flattened) and capped at 2000 chars; Truncated flags
// the cap was hit.
//
// Author is a STRING (displayName, or accountId fallback) — Jira's wire
// shape is an object; the widget reads `c.author` as a string. See
// RESEARCH Pitfall 1.
type JiraCacheComment struct {
    ID        string `json:"id"`
    Author    string `json:"author"`
    Created   string `json:"created"`
    Updated   string `json:"updated"`
    Body      string `json:"body"`
    Truncated bool   `json:"truncated,omitempty"` // omit when false (RESEARCH A2 / D-TEST-01 golden-file)
}
```

Do NOT: add any methods, constructors, or conversion functions to this file — those belong in `refresh.go` (Plan 02). This file is pure type definitions so that `refresh_test.go` can reference `JiraCache` / `JiraCacheIssue` etc. without compilation errors.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm && go build ./pkg/jira/... && go vet ./pkg/jira/...</automated>
  </verify>
  <done>
`pkg/jira/cache_types.go` exists. Package compiles. All four type names (`JiraCache`, `JiraCacheIssue`, `JiraCacheAttachment`, `JiraCacheComment`) are exported and discoverable via `go doc ./pkg/jira JiraCache`. Every Phase 1 test still passes.
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 3: Create refresh_test.go + fixture JSONs + golden file (Nyquist RED)</name>
  <files>pkg/jira/refresh_test.go, pkg/jira/testdata/cache.golden.json, pkg/jira/testdata/cache-with-localpath.seed.json, pkg/jira/testdata/issue-itsm1.json, pkg/jira/testdata/issue-comment-cap.json, pkg/jira/testdata/issue-comment-truncation.json, pkg/jira/testdata/issue-null-description.json</files>
  <behavior>
Every test in this task must COMPILE and FAIL (symbol `Refresh` / `RefreshOpts` / `RefreshReport` undefined, or first-run returns zero values). Plan 02 turns them GREEN. Tests specify:

  - Test 1 `TestRefresh_GoldenFile` — single-issue fixture (ITSM-1) produces cache byte-identical to `testdata/cache.golden.json` after normalizing the `fetchedAt` field. Covers JIRA-07 Success #1 (D-TEST-01).
  - Test 2 `TestRefresh_PreserveLocalPath` — seed an existing cache with `localPath="/downloaded/file.png"` on ITSM-1's attachment `att1`, run Refresh, assert the new cache still has that localPath value. Covers JIRA-07 Success #4 (D-TEST-02, D-FLOW-04).
  - Test 3 `TestRefresh_CommentCapAndTruncation` — issue fixture with 15 comments where comment #3 has a 2500-char body: verify cache has exactly 10 comments (the LAST 10, not the first 10 — RESEARCH Pitfall 2), commentCount = 15, and any kept comment with body > 2000 chars has body truncated to 2000 with `truncated: true`. Covers JIRA-06 (D-CACHE-06, D-CACHE-07, D-TEST-03).
  - Test 4 `TestRefresh_LastCommentAt` — fixture with 2 kept comments where one has `updated > created`: lastCommentAt equals `max(updated, created)` across kept (D-CACHE-07).
  - Test 5 `TestRefresh_NullDescription` — issue fixture with `description: null`: cache has `description: ""` with no error (D-CACHE-04).
  - Test 6 `TestRefresh_StatusCategoryMapping` — fixtures with statusCategory.key values "new", "indeterminate", "done", "undefined", missing: map to "new", "indeterminate", "done", "new", "new" respectively (D-CACHE-08).
  - Test 7 `TestRefresh_ErrorClassification` — three sub-cases: (a) /myself returns 401 → Refresh returns non-nil error AND no cache file is written; (b) /search/jql returns 500 → same; (c) one /issue/KEY returns 404 while others succeed → Refresh returns nil error, cache omits the failed issue, IssueCount reflects successful count (D-ERR-01..03).
  - Test 8 `TestRefresh_ProgressCallback` — record every `(stage, current, total)` invocation; assert sequence starts with `("search", 0, 0)`, includes `("fetch", 1, 1)` for a one-issue run, and ends with `("write", 1, 1)` (D-PROG-01).
  - Test 9 `TestRefresh_AttachmentWebUrlPassthrough` — fixture with attachment `content` URL in site-pattern format: cache `webUrl` equals wire `content` unchanged (D-CACHE-05, RESEARCH A3).
  </behavior>
  <action>
Create `pkg/jira/refresh_test.go` and six JSON files under `pkg/jira/testdata/`. White-box test (`package jira`, not `jira_test`) matching Phase 1 convention.

### Step A — create fixture JSON files under `pkg/jira/testdata/`

**`pkg/jira/testdata/issue-itsm1.json`** — canonical minimal fixture. Copy exactly from RESEARCH §Example 2 (the `issueFixtureITSM1` block). Keep the file compact JSON (NOT pretty-printed) so the `strings.HasPrefix` path-matching in the fixture server doesn't have to handle interspersed whitespace.

**`pkg/jira/testdata/issue-comment-cap.json`** — issue key "ITSM-CAP-1", `comment.total: 15`, `comment.comments: [c01..c15]` where c01 is oldest (`created: 2026-03-01T00:00:00.000+0000`), c15 is newest (`created: 2026-04-15T00:00:00.000+0000`). Each comment has a SHORT body EXCEPT c03 which has a 2500-character body (use ADF paragraph with a long text node: `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"<X repeated 2500 times>"}]}]}` — generate in-file with a Python/jq script or hand-write with string replication; the `X` character avoids JSON escape issues). After the LAST-10 rule is applied, c03 must still be in the kept set — so c03 is among c06..c15 in creation order. To make c03 one of the kept 10, name the oldest-dropped ones c01..c05 and kept c06..c15, with c06 being the one that has the 2500-char body. Renumber to keep clarity: in the fixture, the 6th comment in chronological order (index 5, id "c06") has the 2500-char body. Oldest 5 (c01..c05) are dropped; kept 10 = c06..c15; c06 body gets truncated.

**`pkg/jira/testdata/issue-comment-truncation.json`** — single-issue helper. OPTIONAL if issue-comment-cap.json already covers the truncation case. Planner note: merge into issue-comment-cap.json; skip this separate file. (Update the files_modified frontmatter will NOT list this one if merged — acceptable because frontmatter lists intent; check Step E.)

**`pkg/jira/testdata/issue-null-description.json`** — issue key "ITSM-NULL-1", all fields populated normally EXCEPT `"description": null`.

**`pkg/jira/testdata/cache-with-localpath.seed.json`** — valid `JiraCache` JSON with one issue "ITSM-1" whose single attachment `att1` has `"localPath": "C:/Users/dev/downloaded/att1-screenshot.png"` (use forward slashes in the string — test must pass on both POSIX and Windows). This is the PRIOR cache state for Test 2.

**`pkg/jira/testdata/cache.golden.json`** — the expected byte-exact cache for Test 1 (single-issue ITSM-1 run). Format: `json.MarshalIndent(&cache, "", "  ")` — 2-space indent. `fetchedAt` value is a placeholder the test normalizes; put `"fetchedAt": "NORMALIZED"` in the golden, and the test replaces the real `fetchedAt` value with that literal before comparing. The golden file contains:

```json
{
  "cloudId": "cloud-xxx",
  "baseUrl": "<<REPLACED-AT-TEST>>",
  "accountId": "acc-dev-os",
  "fetchedAt": "NORMALIZED",
  "issues": [
    {
      "key": "ITSM-1",
      "id": "10001",
      "summary": "Example issue",
      "description": "Hello world",
      "status": "In Progress",
      "statusCategory": "indeterminate",
      "issueType": "Bug",
      "priority": "High",
      "projectKey": "ITSM",
      "projectName": "IT Service Management",
      "updated": "2026-04-14T15:30:00.000+0000",
      "created": "2026-04-01T08:00:00.000+0000",
      "webUrl": "<<REPLACED-AT-TEST>>/browse/ITSM-1",
      "attachments": [
        {
          "id": "att1",
          "filename": "screenshot.png",
          "mimeType": "image/png",
          "size": 12345,
          "localPath": "",
          "webUrl": "https://example.atlassian.net/rest/api/3/attachment/content/att1"
        }
      ],
      "comments": [
        {
          "id": "c1",
          "author": "User One",
          "created": "2026-04-02T09:00:00.000+0000",
          "updated": "2026-04-02T09:00:00.000+0000",
          "body": "First"
        },
        {
          "id": "c2",
          "author": "User Two",
          "created": "2026-04-03T10:00:00.000+0000",
          "updated": "2026-04-03T10:30:00.000+0000",
          "body": "Second"
        }
      ],
      "commentCount": 2,
      "lastCommentAt": "2026-04-03T10:30:00.000+0000"
    }
  ]
}
```

The `<<REPLACED-AT-TEST>>` tokens are placeholders for the httptest server URL (which is random per run). The test does a literal string-replace of `<<REPLACED-AT-TEST>>` in the golden bytes with `srv.URL` before byte-comparing. Note: `baseUrl` and `webUrl` must BOTH use the same substitution so the URL prefix stays consistent.

### Step B — create `pkg/jira/refresh_test.go`

White-box test file (`package jira`). Imports: `bytes`, `context`, `encoding/json`, `errors`, `fmt`, `io`, `net/http`, `net/http/httptest`, `os`, `path/filepath`, `regexp`, `strings`, `testing`.

Shared helpers at top of file:

```go
// setFakeHome sets HOME and USERPROFILE to t.TempDir() and creates the
// ~/.config/waveterm/ subdirectory. Returns the tmpdir. Ensures Refresh
// writes its cache into the test's isolated directory.
func setFakeHome(t *testing.T) string {
    t.Helper()
    tmp := t.TempDir()
    t.Setenv("HOME", tmp)
    t.Setenv("USERPROFILE", tmp) // Windows — os.UserHomeDir() reads USERPROFILE first
    // Do NOT pre-create ~/.config/waveterm/ — Refresh itself must MkdirAll (Pitfall 5).
    return tmp
}

// readFixture returns the bytes of a file under pkg/jira/testdata/.
func readFixture(t *testing.T, name string) []byte {
    t.Helper()
    data, err := os.ReadFile(filepath.Join("testdata", name))
    if err != nil {
        t.Fatalf("read fixture %s: %v", name, err)
    }
    return data
}

// newRefreshTestServer returns an httptest.Server that answers /myself,
// /search/jql (single-page), and /issue/{key}. issueFixtures maps issue key
// to the JSON response body. Unknown issue keys return 404.
func newRefreshTestServer(t *testing.T, issueKeys []string, issueFixtures map[string]string) *httptest.Server {
    t.Helper()
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        switch {
        case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself":
            fmt.Fprint(w, `{"accountId":"acc-dev-os","displayName":"Dev","emailAddress":"dev@example.com"}`)
        case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
            // Drain body to simulate real server behavior
            _, _ = io.ReadAll(r.Body)
            refs := make([]string, 0, len(issueKeys))
            for i, k := range issueKeys {
                refs = append(refs, fmt.Sprintf(`{"id":"%d","key":"%s"}`, 10000+i, k))
            }
            fmt.Fprintf(w, `{"issues":[%s],"isLast":true,"nextPageToken":""}`, strings.Join(refs, ","))
        case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/"):
            key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
            body, ok := issueFixtures[key]
            if !ok {
                w.WriteHeader(http.StatusNotFound)
                fmt.Fprint(w, `{"errorMessages":["not found"]}`)
                return
            }
            fmt.Fprint(w, body)
        default:
            t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
            w.WriteHeader(http.StatusInternalServerError)
        }
    }))
}

// baseConfig returns a Config pointed at srv with dev-style values.
// Tests call this to build the RefreshOpts.Config.
func baseConfig(srv *httptest.Server) Config {
    return Config{
        BaseUrl:  srv.URL,
        CloudId:  "cloud-xxx",
        Email:    "dev@example.com",
        ApiToken: "tok",
        Jql:      "assignee = currentUser()",
        PageSize: 50,
    }
}

// normalizeFetchedAt replaces the fetchedAt value with a stable literal so
// golden-file comparison is deterministic.
var fetchedAtRe = regexp.MustCompile(`"fetchedAt":\s*"[^"]+"`)
func normalizeFetchedAt(b []byte) []byte {
    return fetchedAtRe.ReplaceAll(b, []byte(`"fetchedAt": "NORMALIZED"`))
}
```

**Test 1 — `TestRefresh_GoldenFile`:**

```go
func TestRefresh_GoldenFile(t *testing.T) {
    setFakeHome(t)
    srv := newRefreshTestServer(t,
        []string{"ITSM-1"},
        map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
    )
    defer srv.Close()

    cfg := baseConfig(srv)
    report, err := Refresh(context.Background(), RefreshOpts{
        Config:     cfg,
        HTTPClient: srv.Client(),
    })
    if err != nil {
        t.Fatalf("refresh: %v", err)
    }
    if report == nil || report.IssueCount != 1 {
        t.Fatalf("report: got %+v, want IssueCount=1", report)
    }

    got, err := os.ReadFile(report.CachePath)
    if err != nil {
        t.Fatalf("read cache: %v", err)
    }
    want := readFixture(t, "cache.golden.json")
    // Substitute placeholder with actual test-server URL
    want = bytes.ReplaceAll(want, []byte("<<REPLACED-AT-TEST>>"), []byte(srv.URL))

    gotN := normalizeFetchedAt(got)
    wantN := normalizeFetchedAt(want)
    if !bytes.Equal(gotN, wantN) {
        t.Errorf("cache mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantN, gotN)
    }
}
```

**Test 2 — `TestRefresh_PreserveLocalPath`:**

```go
func TestRefresh_PreserveLocalPath(t *testing.T) {
    tmp := setFakeHome(t)
    // Pre-seed ~/.config/waveterm/jira-cache.json with a non-empty localPath.
    dir := filepath.Join(tmp, ".config", "waveterm")
    if err := os.MkdirAll(dir, 0o755); err != nil {
        t.Fatal(err)
    }
    seed := readFixture(t, "cache-with-localpath.seed.json")
    if err := os.WriteFile(filepath.Join(dir, "jira-cache.json"), seed, 0o644); err != nil {
        t.Fatal(err)
    }

    srv := newRefreshTestServer(t,
        []string{"ITSM-1"},
        map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
    )
    defer srv.Close()

    report, err := Refresh(context.Background(), RefreshOpts{
        Config:     baseConfig(srv),
        HTTPClient: srv.Client(),
    })
    if err != nil {
        t.Fatalf("refresh: %v", err)
    }

    var out JiraCache
    data, _ := os.ReadFile(report.CachePath)
    if err := json.Unmarshal(data, &out); err != nil {
        t.Fatalf("unmarshal cache: %v", err)
    }
    if len(out.Issues) != 1 || len(out.Issues[0].Attachments) != 1 {
        t.Fatalf("expected 1 issue with 1 attachment, got %+v", out)
    }
    got := out.Issues[0].Attachments[0].LocalPath
    want := "C:/Users/dev/downloaded/att1-screenshot.png"
    if got != want {
        t.Errorf("LocalPath: got %q, want %q", got, want)
    }
}
```

**Test 3 — `TestRefresh_CommentCapAndTruncation`:**

```go
func TestRefresh_CommentCapAndTruncation(t *testing.T) {
    setFakeHome(t)
    srv := newRefreshTestServer(t,
        []string{"ITSM-CAP-1"},
        map[string]string{"ITSM-CAP-1": string(readFixture(t, "issue-comment-cap.json"))},
    )
    defer srv.Close()

    report, err := Refresh(context.Background(), RefreshOpts{
        Config:     baseConfig(srv),
        HTTPClient: srv.Client(),
    })
    if err != nil {
        t.Fatalf("refresh: %v", err)
    }
    var out JiraCache
    data, _ := os.ReadFile(report.CachePath)
    json.Unmarshal(data, &out)
    iss := out.Issues[0]
    if iss.CommentCount != 15 {
        t.Errorf("CommentCount: got %d, want 15", iss.CommentCount)
    }
    if len(iss.Comments) != 10 {
        t.Fatalf("Comments: got %d, want 10", len(iss.Comments))
    }
    // Last-10 rule: oldest dropped = c01..c05; kept = c06..c15 in that order.
    if iss.Comments[0].ID != "c06" {
        t.Errorf("first kept comment id: got %q, want c06 (oldest-first within kept)", iss.Comments[0].ID)
    }
    if iss.Comments[9].ID != "c15" {
        t.Errorf("last kept comment id: got %q, want c15", iss.Comments[9].ID)
    }
    // c06 has the 2500-char body → must be truncated to 2000 + truncated:true.
    c06 := iss.Comments[0]
    if len(c06.Body) != 2000 {
        t.Errorf("c06 body len: got %d, want 2000", len(c06.Body))
    }
    if !c06.Truncated {
        t.Errorf("c06 Truncated: got false, want true")
    }
    // c07 has a short body → not truncated.
    c07 := iss.Comments[1]
    if c07.Truncated {
        t.Errorf("c07 Truncated: got true, want false")
    }
}
```

**Test 4 — `TestRefresh_LastCommentAt`:** Uses the ITSM-1 fixture. c2 has `updated: 2026-04-03T10:30:00.000+0000` and `created: 2026-04-03T10:00:00.000+0000`. Expected `lastCommentAt == "2026-04-03T10:30:00.000+0000"` (max of the two on c2, which is the max across both kept comments).

**Test 5 — `TestRefresh_NullDescription`:** Uses `issue-null-description.json` for key `ITSM-NULL-1`. Asserts `out.Issues[0].Description == ""`.

**Test 6 — `TestRefresh_StatusCategoryMapping`:** Build five issue fixtures inline (as Go string literals, no disk files needed) with different `statusCategory.key` values (including missing/null), use each as the sole issue in a fresh run, assert the resulting `StatusCategory` field. Use a table-driven loop.

**Test 7 — `TestRefresh_ErrorClassification`:** Three subtests using `t.Run`. Construct CUSTOM httptest servers for (a) and (b) that return the error status. For (c) reuse `newRefreshTestServer` with a fixture map that omits one key listed in issueKeys. For (a) and (b) additionally assert the cache file does NOT exist after the failed call.

**Test 8 — `TestRefresh_ProgressCallback`:** Pass an `OnProgress` closure that appends every `(stage, current, total)` tuple to a slice. After a one-issue ITSM-1 run, assert the slice contains at least one `("search", _, 0)`, one `("fetch", 1, 1)`, and one `("write", 1, 1)`.

**Test 9 — `TestRefresh_AttachmentWebUrlPassthrough`:** Use ITSM-1 fixture. Assert `out.Issues[0].Attachments[0].WebUrl == "https://example.atlassian.net/rest/api/3/attachment/content/att1"` — byte-identical to the wire `content` field.

### Step C — RED verification

Before committing, run `go test ./pkg/jira -run TestRefresh -count=1`. Expected outcome: **compile failure** with messages like `undefined: Refresh`, `undefined: RefreshOpts`, `undefined: RefreshReport`. This is the Nyquist RED signal.

If the tests COMPILE and PASS trivially, something is wrong — Refresh must not exist yet. If they COMPILE and FAIL with assertion errors, Plan 02 has already leaked forward — revert.

### Step D — safe failure mode for CI

Since Plan 02 has not shipped, `go test ./pkg/jira -count=1` will fail compilation. This is intentional. The Wave 1 DoD verify command explicitly RUNS those tests and expects them to fail at compile time; Plan 02's verify will expect them to pass.

### Step E — fixture merge note

If you opted to merge truncation into `issue-comment-cap.json` (recommended), DO NOT create a separate `issue-comment-truncation.json`. The frontmatter `files_modified` list is intent; creating fewer files is allowed. Update a short comment in the PR description noting "issue-comment-truncation.json merged into issue-comment-cap.json — comment c06 carries the 2500-char body."

### Threat-model guardrails (T-01-02 extension, RESEARCH Pitfall 8)

- Test code MUST NOT log `config`, `apiToken`, or any auth header. Test servers assert request path only.
- Error messages from Refresh test assertions use `%v` (never `%+v`) when formatting `*APIError`.
- No test sets real Atlassian credentials. `ApiToken: "tok"` is the only acceptable value.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm && ( go test ./pkg/jira -run TestRefresh -count=1 2>&1 | grep -qE 'undefined: Refresh|undefined: RefreshOpts|undefined: RefreshReport' && echo RED-OK ) && go test ./pkg/jira -run 'TestAuthHeader|TestSearchIssues|TestGetIssue|TestLoadConfig|TestADF' -count=1</automated>
  </verify>
  <done>
`pkg/jira/refresh_test.go` exists, uses `package jira`, declares nine `TestRefresh_*` functions. All fixture JSON files exist under `pkg/jira/testdata/` (at least cache.golden.json, cache-with-localpath.seed.json, issue-itsm1.json, issue-comment-cap.json, issue-null-description.json). `go test ./pkg/jira -run TestRefresh -count=1` fails at compile time with `undefined: Refresh` (or equivalent missing-symbol error). Every Phase 1 test (`TestAuthHeader_*`, `TestSearchIssues_*`, `TestGetIssue_*`, `TestLoadConfig_*`, `TestADF*`) still passes.
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| test ↔ httptest | Fake Jira server runs in-process; no real credentials cross it |
| process ↔ tmp filesystem | `t.TempDir()` is isolated per-test; `HOME` env var override prevents polluting real user config |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-02-01 | Information Disclosure | refresh_test.go helpers logging | mitigate | Assertions format errors with `%v` only; never `%+v` on `*APIError`; test servers echo only path, never headers |
| T-02-02 | Information Disclosure | testdata fixtures committed to repo | accept | Fixture files contain only synthetic values (`dev@example.com`, `tok`, `cloud-xxx`) — no real PAT, email, or cloudId leaks |
| T-02-03 | Tampering | Test writes pollute real `~/.config/waveterm/` | mitigate | Every test calls `setFakeHome(t)` which sets HOME + USERPROFILE to `t.TempDir()` — writes are confined |
| T-02-04 | Denial of Service | Malformed ADF in comment body crashing ADFToMarkdown | mitigate (tested in Plan 02) | Refresh catches per-comment ADF errors → degrades body to `""` + log; covered by a Plan 02 test (deferred out of Wave 1 RED set to keep scope tight) |
| T-02-05 | Spoofing | Test server impersonates real Jira endpoints | accept | Test-only; production code never trusts a non-HTTPS endpoint unless Config.BaseUrl says so (user controls) |
</threat_model>

<verification>
Wave-1 gate:

1. `cd F:/Waveterm/waveterm && go build ./pkg/jira/...` succeeds.
2. `go vet ./pkg/jira/...` clean.
3. `go test ./pkg/jira -run TestRefresh -count=1` FAILS at compile time with `undefined: Refresh` / `undefined: RefreshOpts` / `undefined: RefreshReport`. (Nyquist RED — tests exist before the code.)
4. `go test ./pkg/jira -run 'TestAuthHeader|TestSearchIssues|TestGetIssue|TestLoadConfig|TestADF' -count=1` PASSES — all Phase 1 tests still green.
5. `ls pkg/jira/testdata/` lists at least: cache.golden.json, cache-with-localpath.seed.json, issue-itsm1.json, issue-comment-cap.json, issue-null-description.json.
6. `grep -c 'func TestRefresh_' pkg/jira/refresh_test.go` ≥ 9.
7. `grep 'StatusCategory struct' pkg/jira/client.go` matches exactly once; `grep 'func (c \*Client) GetMyself' pkg/jira/client.go` matches once.
8. No test or helper in `refresh_test.go` contains the characters `%+v` (forbidden per T-01-02 / RESEARCH Pitfall 8): `grep -n '%+v' pkg/jira/refresh_test.go` returns empty.
</verification>

<success_criteria>
- Plan 02 can be implemented entirely by reading `refresh_test.go` + this plan — no need to re-read CONTEXT.md/RESEARCH.md except for cross-reference.
- Every decision D-CACHE-01..08, D-FLOW-01..05, D-PROG-01..02, D-ERR-01..03 is asserted by at least one test in `refresh_test.go`.
- Adding the additive fields to `client.go` broke zero Phase 1 tests (proven by verify command #4).
- Tests fail at compile time, not at runtime — downstream Plan 02 gets a clean RED-to-GREEN transition.
</success_criteria>

<output>
After completion, create `.planning/phases/02-cache-orchestration/02-01-SUMMARY.md` per `$HOME/.claude/get-shit-done/templates/summary.md`. SUMMARY should emphasize:
- Exact list of tests added (names) and which D-decision each covers.
- Choice made on HTTP injection: `RefreshOpts.HTTPClient *http.Client` (RESEARCH Open Question 1, Recommendation adopted).
- Any discretionary choices (e.g. whether issue-comment-truncation.json was merged into issue-comment-cap.json).
- The exact `client.go` diff (additive — no removed or modified lines).
</output>
</content>
</invoke>
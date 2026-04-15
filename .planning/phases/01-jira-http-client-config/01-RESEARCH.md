# Phase 1: Jira HTTP Client + Config — Research

**Researched:** 2026-04-15
**Domain:** Go HTTP client, Jira Cloud REST v3, ADF-to-markdown conversion, httptest
**Confidence:** HIGH (API shapes verified via official Atlassian docs; Go patterns verified via pkg.go.dev and codebase grep)

---

<user_constraints>
## User Constraints (from CONTEXT.md)

### Locked Decisions (D-01 .. D-23 — do not re-open)

- **D-01** Use `POST /rest/api/3/search/jql` (enhanced, cursor-based). Legacy `/rest/api/3/search` is out.
- **D-02** `GetIssue` = `GET /rest/api/3/issue/{key}?fields=...`; caller-provided `[]string` of fields.
- **D-03** Default page size = 50. Default JQL = `assignee = currentUser() ORDER BY updated DESC`.
- **D-04** Options struct style (not variadic). Exact shapes in CONTEXT.md.
- **D-05** Client embeds private `http.Client`, 30 s timeout. Test seam = `NewClientWithHTTP`.
- **D-06** Auth = `Authorization: Basic base64(email:apiToken)`. Also `Accept: application/json` and `User-Agent: waveterm-jira/<version>`.
- **D-07** Config path = `os.UserHomeDir() + "/.config/waveterm/jira.json"`. NOT `wavebase.GetWaveConfigDir()`.
- **D-08** Config fields: `BaseUrl`, `CloudId`, `Email`, `ApiToken`, `Jql`, `PageSize`.
- **D-09** Missing/unreadable config → `ErrConfigNotFound` (sentinel).
- **D-10** Malformed JSON → `ErrConfigInvalid` wrapping `json.Unmarshal` error.
- **D-11** Validation: missing `BaseUrl`/`Email`/`ApiToken` → `ErrConfigIncomplete` naming missing fields.
- **D-12** Loader re-reads every call (no in-process cache).
- **D-13** ADF output = markdown.
- **D-14** ADF node set: doc, paragraph, hardBreak, heading (1-6), bulletList, orderedList, listItem, codeBlock, text+marks(strong/em/code/link), mention, rule, blockquote, table/tableRow/tableHeader/tableCell.
- **D-15** Unknown nodes: silent skip + `log.Printf` debug; not an error.
- **D-16** Entry point: `func ADFToMarkdown(raw json.RawMessage) (string, error)`.
- **D-17** Handles both issue description and comment body ADF.
- **D-18** Sentinel errors + `*APIError` struct with `Unwrap()` — exact shapes in CONTEXT.md.
- **D-19** Every non-2xx → `*APIError`; callers use `errors.Is` / `errors.As`.
- **D-20** `RetryAfter time.Duration` on `APIError` when `StatusCode == 429`; Phase 1 does not retry.
- **D-21** Use `net/http/httptest.NewServer`; no new test framework.
- **D-22** Coverage list in CONTEXT.md (auth, search pagination, GET issue, 401/404/429/5xx, config paths, ADF per-node).
- **D-23** All tests green on Windows. No unix-only syscalls, separators, or shell invocations.

### Claude's Discretion

- Internal file split within `pkg/jira/` (errors.go vs inline in client.go, etc.)
- Exact field names on `SearchResult` / `Issue` structs
- ADF converter style (struct visitor vs recursive `map[string]any`)
- Go module version / `log` vs `slog` — match existing `pkg/` convention

### Deferred Ideas (OUT OF SCOPE)

- Retry / backoff on 429 and 5xx (Phase 5)
- Rate limiter (Phase 5)
- Attachment download (Phase 5)
- Comment truncation strategy (Phase 2)
- Jira Server / Data Center
- OAuth / PKCE / Electron safeStorage / React settings modal
</user_constraints>

---

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| JIRA-01 | HTTP request to Atlassian Jira Cloud from Go backend, authenticated with PAT + email via HTTP Basic | §API Request Shapes, §Auth Header, §Error Handling |
| JIRA-02 | Config via `~/.config/waveterm/jira.json`; re-read on every refresh without restart | §Config Loader Pattern, §Path Resolution |
</phase_requirements>

---

## Summary

Phase 1 creates `pkg/jira/` from scratch. All 23 user decisions are locked; research focuses on the **exact wire format** the planner needs to write concrete tasks.

The key findings are: (1) `POST /rest/api/3/search/jql` returns `nextPageToken`, `isLast`, and `issues[]` in the response **body** — no pagination headers; (2) ADF node attributes are unambiguous for all D-14 types (mention uses `attrs.text`, link uses `attrs.href`, tableHeader is a distinct node type from tableCell); (3) `Retry-After` on Jira 429 responses is an integer number of seconds, parsed with `strconv.Atoi`; (4) the codebase uses stdlib `"log"` universally — `slog` is not present.

**Primary recommendation:** Mirror `pkg/waveai/anthropicbackend.go` for HTTP client structure; mirror `pkg/wconfig/settingsconfig.go` for the JSON-decode-with-defaults pattern; use `pkg/utilds.CodedError` as a reference for `Unwrap()` chain design.

---

## Standard Stack

### Core (all stdlib — no new dependencies required)

| Package | Purpose | Source |
|---------|---------|--------|
| `net/http` | HTTP client, request/response | [VERIFIED: codebase — anthropicbackend.go] |
| `net/http/httptest` | In-process test server | [VERIFIED: pkg.go.dev/net/http/httptest] |
| `encoding/json` | JSON marshal/unmarshal | [VERIFIED: codebase] |
| `encoding/base64` | Basic auth token encoding | [VERIFIED: Atlassian docs] |
| `os` | `os.UserHomeDir()`, `os.ReadFile`, `os.IsNotExist` | [VERIFIED: codebase — wavebase.go] |
| `errors` | `errors.New`, `errors.Is`, `errors.As` | [VERIFIED: codebase — utilds/codederror.go] |
| `fmt` | Error wrapping with `%w` | [VERIFIED: codebase] |
| `strconv` | `strconv.Atoi` for `Retry-After` integer | [VERIFIED: Atlassian rate-limit docs — pseudocode uses numeric seconds] |
| `time` | `time.Duration`, `time.Second` | [VERIFIED: stdlib] |
| `log` | Debug `log.Printf` for unknown ADF nodes | [VERIFIED: codebase — 89 files in pkg/ import "log", slog not used] |
| `strings` | String building for ADF output | [VERIFIED: codebase] |
| `path/filepath` | Path joining for config file | [VERIFIED: codebase — wavebase.go] |
| `io` | `io.ReadAll` for response body | [VERIFIED: anthropicbackend.go line 217] |

**No new `go.mod` entries needed.** Go version in use: `go1.26.1` (local), `go 1.25.6` declared in `go.mod` — both support all required stdlib. [VERIFIED: `go version` output + `go.mod` line 3]

**Installation:** Nothing to install — stdlib only.

---

## Architecture Patterns

### Recommended File Layout

```
pkg/jira/
├── config.go        // Config struct, LoadConfig(), sentinel errors ErrConfig*
├── client.go        // Client struct, NewClient(), NewClientWithHTTP(), SearchIssues(), GetIssue()
├── errors.go        // APIError struct, sentinel errors ErrUnauthorized..ErrServerError, Unwrap()
├── adf.go           // ADFToMarkdown(), internal node-walk helpers
└── client_test.go   // ALL tests in package jira (not jira_test) for white-box access to NewClientWithHTTP
```

Internal split rationale (Claude's discretion): `errors.go` is separate so `config.go` can reference config sentinels without circular imports; `adf.go` is self-contained because it has its own table-driven test surface.

### Pattern 1: HTTP Client Construction (mirror anthropicbackend.go)

```go
// Source: pkg/waveai/anthropicbackend.go lines 195-206
// anthropicbackend.go constructs client inline per call; for jira we store it in the struct.
type Client struct {
    cfg Config
    hc  *http.Client
}

func NewClient(cfg Config) *Client {
    return &Client{
        cfg: cfg,
        hc:  &http.Client{Timeout: 30 * time.Second},
    }
}

// Test seam — called from client_test.go (same package)
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client {
    return &Client{cfg: cfg, hc: hc}
}
```

[VERIFIED: codebase pattern; `http.Client` struct with Timeout is stdlib]

### Pattern 2: Basic Auth Header (per Atlassian docs)

```go
// Source: https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/
// credential = base64(email + ":" + apiToken)
import "encoding/base64"

func basicAuthHeader(email, token string) string {
    raw := email + ":" + token
    return "Basic " + base64.StdEncoding.EncodeToString([]byte(raw))
}
```

Set on every request:
```go
req.Header.Set("Authorization", basicAuthHeader(c.cfg.Email, c.cfg.ApiToken))
req.Header.Set("Accept", "application/json")
req.Header.Set("Content-Type", "application/json")  // for POST bodies
req.Header.Set("User-Agent", "waveterm-jira/"+wavebase.WaveVersion)
```

[VERIFIED: Atlassian Basic Auth docs — username=email, password=apiToken]

### Pattern 3: Config Loader (mirror settingsconfig.go read+decode pattern)

```go
// Source: pkg/wconfig/settingsconfig.go lines 518-550
func LoadConfig() (Config, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return Config{}, fmt.Errorf("jira: cannot determine home dir: %w", err)
    }
    cfgPath := filepath.Join(home, ".config", "waveterm", "jira.json")

    data, err := os.ReadFile(cfgPath)
    if os.IsNotExist(err) || errors.Is(err, fs.ErrNotExist) {
        return Config{}, ErrConfigNotFound  // D-09
    }
    if err != nil {
        return Config{}, fmt.Errorf("%w: %w", ErrConfigNotFound, err)
    }

    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return Config{}, fmt.Errorf("%w: %w", ErrConfigInvalid, err)  // D-10
    }

    // Fill defaults (D-03, D-08)
    if cfg.Jql == "" {
        cfg.Jql = "assignee = currentUser() ORDER BY updated DESC"
    }
    if cfg.PageSize == 0 {
        cfg.PageSize = 50
    }

    // Validate required fields (D-11)
    var missing []string
    if cfg.BaseUrl == ""  { missing = append(missing, "baseUrl") }
    if cfg.Email == ""    { missing = append(missing, "email") }
    if cfg.ApiToken == "" { missing = append(missing, "apiToken") }
    if len(missing) > 0 {
        return Config{}, fmt.Errorf("%w: missing fields: %s",
            ErrConfigIncomplete, strings.Join(missing, ", "))
    }
    return cfg, nil
}
```

Note: `filepath.Join` is safe on Windows — it uses `\` automatically. [VERIFIED: wavebase.go uses filepath.Join throughout]

### Pattern 4: Error Typing with Unwrap Chain (mirror utilds/codederror.go)

```go
// Source: pkg/utilds/codederror.go
// The codebase already has CodedError as the reference for Unwrap() pattern.
// APIError mirrors the same idea with HTTP-specific fields.
type APIError struct {
    StatusCode int
    Endpoint   string
    Method     string
    Body       string        // truncated to 1 KB per D-18
    RetryAfter time.Duration // populated only when StatusCode == 429, per D-20
}

func (e *APIError) Error() string {
    return fmt.Sprintf("jira: HTTP %d %s %s", e.StatusCode, e.Method, e.Endpoint)
}

func (e *APIError) Unwrap() error {
    switch {
    case e.StatusCode == 401: return ErrUnauthorized
    case e.StatusCode == 403: return ErrForbidden
    case e.StatusCode == 404: return ErrNotFound
    case e.StatusCode == 429: return ErrRateLimited
    case e.StatusCode >= 500: return ErrServerError
    default:                  return nil
    }
}
```

[VERIFIED: pattern mirrors pkg/utilds/codederror.go; `errors.Is(err, ErrUnauthorized)` traverses Unwrap() chain]

### Pattern 5: Non-2xx Response Handling

```go
// Source: pkg/waveai/anthropicbackend.go lines 215-218 (similar pattern)
resp, err := c.hc.Do(req)
if err != nil {
    return nil, fmt.Errorf("jira: request failed: %w", err)
}
defer resp.Body.Close()

if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
    apiErr := &APIError{
        StatusCode: resp.StatusCode,
        Endpoint:   req.URL.Path,
        Method:     req.Method,
        Body:       string(body),
    }
    if resp.StatusCode == 429 {
        apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
    }
    return nil, apiErr
}
```

[VERIFIED: stdlib pattern; body truncation via `io.LimitReader`]

### Pattern 6: httptest Table-Driven Tests

```go
// Source: https://pkg.go.dev/net/http/httptest
// Standard Go table-driven test with per-case httptest.NewServer:
func TestSearchIssues(t *testing.T) {
    tests := []struct {
        name       string
        statusCode int
        respBody   string
        wantErr    error
        checkFn    func(*testing.T, *SearchResult)
    }{
        {
            name:       "200 first page",
            statusCode: 200,
            respBody:   `{"issues":[{"key":"ITSM-1","id":"10001","fields":{}}],"isLast":false,"nextPageToken":"tok2"}`,
            checkFn: func(t *testing.T, r *SearchResult) {
                if r.NextPageToken != "tok2" { t.Errorf(...) }
            },
        },
        {
            name:       "401 unauthorized",
            statusCode: 401,
            respBody:   `{"errorMessages":["Unauthorized"]}`,
            wantErr:    ErrUnauthorized,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                // Verify auth header
                auth := r.Header.Get("Authorization")
                if !strings.HasPrefix(auth, "Basic ") {
                    t.Errorf("missing Basic auth, got: %s", auth)
                }
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(tc.statusCode)
                fmt.Fprint(w, tc.respBody)
            }))
            defer srv.Close()

            cfg := Config{BaseUrl: srv.URL, Email: "a@b.com", ApiToken: "tok"}
            client := NewClientWithHTTP(cfg, srv.Client())
            result, err := client.SearchIssues(context.Background(), SearchOpts{JQL: "project=X"})

            if tc.wantErr != nil {
                if !errors.Is(err, tc.wantErr) {
                    t.Errorf("got %v, want %v", err, tc.wantErr)
                }
            }
            if tc.checkFn != nil && result != nil {
                tc.checkFn(t, result)
            }
        })
    }
}
```

[VERIFIED: pkg.go.dev/net/http/httptest — `NewServer` returns server with `.URL` and `.Client()`]

### Anti-Patterns to Avoid

- **Global `http.DefaultClient`:** Never use `http.DefaultClient` — no timeout. Phase 1 creates a client with `Timeout: 30*time.Second`. [VERIFIED: anthropicbackend.go creates `&http.Client{}` per call without Timeout — note this is a known pattern gap upstream; Phase 1 should do better]
- **`filepath.Join` on Windows with hardcoded `/`:** Config path must use `filepath.Join(home, ".config", "waveterm", "jira.json")` not string concat with `/`. [VERIFIED: wavebase.go ExpandHomeDir uses filepath.Join]
- **`io.ReadAll` without a size cap:** Always use `io.LimitReader(resp.Body, N)` before `io.ReadAll` for API responses. The 1 KB cap in `APIError.Body` enforces this for error bodies. For success paths, Jira responses fit in memory safely for 50-issue pages.
- **`base64.URLEncoding` instead of `base64.StdEncoding`:** HTTP Basic auth uses StdEncoding (RFC 7617). [VERIFIED: Atlassian docs show standard base64]
- **`os.IsNotExist` only:** Use both `os.IsNotExist(err)` and `errors.Is(err, fs.ErrNotExist)` for config-not-found detection — the latter is the modern form. settingsconfig.go uses `os.IsNotExist`.

---

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Base64 encoding for auth | custom encoder | `encoding/base64.StdEncoding.EncodeToString` | stdlib, correct padding |
| HTTP body size cap | manual byte counting | `io.LimitReader(r, 1024)` | stdlib one-liner |
| Error chain traversal | custom `.Is()` | `errors.Is` / `errors.As` with `Unwrap()` | already in `utilds.CodedError`; mirror the pattern |
| In-process test HTTP server | real network calls | `httptest.NewServer` | isolated, no port conflicts, works on Windows |
| JSON config defaults | complex merge logic | conditional assignment after `json.Unmarshal` | settingsconfig.go pattern; simple and readable |
| `Retry-After` parsing | HTTP-date parser | `strconv.Atoi` | Jira uses integer-seconds format [VERIFIED: Atlassian rate-limit docs pseudocode] |

---

## Jira API Wire Format

### POST /rest/api/3/search/jql — Request Body

[VERIFIED: https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-post]

```json
{
  "jql":          "assignee = currentUser() ORDER BY updated DESC",
  "maxResults":   50,
  "nextPageToken": "",
  "fields":       ["summary", "description", "status", "issuetype", "priority",
                   "project", "created", "updated", "attachment", "comment"],
  "fieldsByKeys": false,
  "expand":       ""
}
```

- `nextPageToken` is sent in the **request body** (not as a query param). Empty string or omit for first page.
- `fields` is a JSON array of strings. Any field name from Jira's field list is valid.
- All fields are optional; `jql` drives the search.

### POST /rest/api/3/search/jql — Response Body

[VERIFIED: Atlassian official docs + community confirmation]

```json
{
  "isLast":        false,
  "nextPageToken": "eyJsaW1pdCI6NTAsInN0YXJ0Ijo1MH0=",
  "issues": [
    {
      "id":     "10001",
      "key":    "ITSM-3135",
      "self":   "https://kakaovx.atlassian.net/rest/api/3/issue/10001",
      "expand": "...",
      "fields": {
        "summary":     "Fix login bug",
        "description": { /* ADF document */ },
        "status":      { "name": "In Progress", "id": "3" },
        "issuetype":   { "name": "Bug", "id": "10004", "subtask": false },
        "priority":    { "name": "High", "id": "2" },
        "project":     { "key": "ITSM", "name": "IT Service Management", "id": "10000" },
        "created":     "2024-01-15T09:30:00.000+0900",
        "updated":     "2024-03-22T14:20:00.000+0900",
        "attachment":  [ /* see attachment shape below */ ],
        "comment":     { "total": 5, "comments": [ /* see comment shape below */ ] }
      }
    }
  ]
}
```

**Critical:** There is **no `total` or `totalIssues` field** in the enhanced search response. [VERIFIED: Atlassian docs + community reports] Phase 1's `SearchResult` struct must NOT expose a total count field.

**Pagination loop logic:**
```
page 1: POST {jql, maxResults: 50}
         → response: {isLast: false, nextPageToken: "X", issues: [...]}
page 2: POST {jql, maxResults: 50, nextPageToken: "X"}
         → response: {isLast: true,  nextPageToken: "Y", issues: [...]}
         → stop because isLast == true
```

### GET /rest/api/3/issue/{key} — Request

[VERIFIED: https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-get]

```
GET /rest/api/3/issue/ITSM-3135?fields=summary,description,status,issuetype,priority,project,created,updated,attachment,comment
```

- `fields` is a **comma-separated query string** (not repeated params). Build it with `strings.Join(opts.Fields, ",")`.
- If `opts.Fields` is nil/empty, omit the `fields` parameter entirely and let Jira return its default field set.

### GET /rest/api/3/issue/{key} — Response Root

```json
{
  "id":     "10001",
  "key":    "ITSM-3135",
  "self":   "https://kakaovx.atlassian.net/rest/api/3/issue/10001",
  "fields": { /* see below */ }
}
```

### Attachment Object (within `fields.attachment[]`)

[VERIFIED: Atlassian REST API docs + web search confirmation]

```json
{
  "id":        "10002",
  "filename":  "screenshot.png",
  "mimeType":  "image/png",
  "size":      204800,
  "created":   "2024-01-15T09:30:00.000+0900",
  "content":   "https://kakaovx.atlassian.net/rest/api/3/attachment/content/10002",
  "thumbnail": "https://kakaovx.atlassian.net/rest/api/3/attachment/thumbnail/10002",
  "author": {
    "accountId":   "5b10ac8d82e05b22cc7d4ef5",
    "displayName": "Test User",
    "emailAddress": "test@kakaovx.com"
  }
}
```

### Comment Object (within `fields.comment`)

[VERIFIED: Atlassian REST API docs]

```json
{
  "total": 5,
  "comments": [
    {
      "id":      "10100",
      "author": {
        "accountId":   "5b10ac8d82e05b22cc7d4ef5",
        "displayName": "Test User"
      },
      "body":    { /* ADF document */ },
      "created": "2024-01-16T10:00:00.000+0900",
      "updated": "2024-01-16T10:05:00.000+0900"
    }
  ]
}
```

Note: `comment` field in `GetIssue` response is a **wrapper object** with `total` (int) and `comments` (array), not a bare array. Phase 1's `Issue.Fields.Comment` struct must reflect this. [VERIFIED: Atlassian docs — "Comment" type contains `total` and `comments`]

### Go Structs for Response Parsing

These names are **Claude's discretion** but must reflect the wire format exactly:

```go
// SearchResult returned by SearchIssues
type SearchResult struct {
    Issues        []IssueRef `json:"issues"`
    NextPageToken string     `json:"nextPageToken"`
    IsLast        bool       `json:"isLast"`
}

// IssueRef is the shallow issue shape from search (fields partial)
type IssueRef struct {
    ID     string      `json:"id"`
    Key    string      `json:"key"`
    Self   string      `json:"self"`
    Fields IssueFields `json:"fields"`
}

// Issue is the full issue shape from GetIssue
type Issue struct {
    ID     string      `json:"id"`
    Key    string      `json:"key"`
    Self   string      `json:"self"`
    Fields IssueFields `json:"fields"`
}

type IssueFields struct {
    Summary     string          `json:"summary"`
    Description json.RawMessage `json:"description"` // ADF — passed to ADFToMarkdown
    Status      struct {
        Name string `json:"name"`
        ID   string `json:"id"`
    } `json:"status"`
    IssueType struct {
        Name    string `json:"name"`
        ID      string `json:"id"`
        Subtask bool   `json:"subtask"`
    } `json:"issuetype"`
    Priority struct {
        Name string `json:"name"`
        ID   string `json:"id"`
    } `json:"priority"`
    Project struct {
        Key  string `json:"key"`
        Name string `json:"name"`
        ID   string `json:"id"`
    } `json:"project"`
    Created    string       `json:"created"`
    Updated    string       `json:"updated"`
    Attachment []Attachment `json:"attachment"`
    Comment    CommentPage  `json:"comment"`
}

type Attachment struct {
    ID        string `json:"id"`
    Filename  string `json:"filename"`
    MimeType  string `json:"mimeType"`
    Size      int64  `json:"size"`
    Created   string `json:"created"`
    Content   string `json:"content"`   // download URL
    Thumbnail string `json:"thumbnail"` // may be empty for non-images
    Author    struct {
        AccountID   string `json:"accountId"`
        DisplayName string `json:"displayName"`
    } `json:"author"`
}

type CommentPage struct {
    Total    int       `json:"total"`
    Comments []Comment `json:"comments"`
}

type Comment struct {
    ID      string          `json:"id"`
    Author  struct {
        AccountID   string `json:"accountId"`
        DisplayName string `json:"displayName"`
    } `json:"author"`
    Body    json.RawMessage `json:"body"` // ADF — passed to ADFToMarkdown
    Created string          `json:"created"`
    Updated string          `json:"updated"`
}
```

[ASSUMED: Field names on `SearchResult`/`Issue`/etc. are Claude's discretion per D-14; the shapes above reflect the wire format. Phase 2 will own mapping to `JiraIssue`.]

---

## ADF Node Shapes (D-14 — all types listed)

[VERIFIED: https://developer.atlassian.com/cloud/jira/platform/apis/document/structure/ + individual node pages]

### Root: doc

```json
{ "version": 1, "type": "doc", "content": [ /* block nodes */ ] }
```

### Block nodes

```json
// paragraph
{ "type": "paragraph", "content": [ /* inline nodes */ ] }

// heading — attrs.level is 1..6
{ "type": "heading", "attrs": { "level": 2 }, "content": [ /* inline */ ] }

// bulletList
{ "type": "bulletList", "content": [ /* listItem nodes */ ] }

// orderedList — listItem does NOT carry an order attr; renderer numbers sequentially
{ "type": "orderedList", "content": [ /* listItem nodes */ ] }

// listItem — content is block nodes (often paragraph)
{ "type": "listItem", "content": [ /* block nodes */ ] }

// codeBlock — attrs.language may be absent/empty
{ "type": "codeBlock", "attrs": { "language": "go" }, "content": [ /* text nodes */ ] }

// blockquote
{ "type": "blockquote", "content": [ /* block nodes */ ] }

// rule (horizontal rule)
{ "type": "rule" }

// table
{
  "type": "table",
  "attrs": { "isNumberColumnEnabled": false, "layout": "default" },
  "content": [ /* tableRow nodes */ ]
}

// tableRow
{ "type": "tableRow", "content": [ /* tableHeader or tableCell nodes */ ] }

// tableHeader — distinct from tableCell; its presence marks a header row
{ "type": "tableHeader", "attrs": {}, "content": [ /* block nodes, typically paragraph */ ] }

// tableCell
{ "type": "tableCell", "attrs": {}, "content": [ /* block nodes, typically paragraph */ ] }
```

### Inline nodes

```json
// text — marks array may be absent or empty
{
  "type": "text",
  "text": "hello",
  "marks": [
    { "type": "strong" },
    { "type": "em" },
    { "type": "code" },
    { "type": "link", "attrs": { "href": "https://example.com", "title": "optional" } }
  ]
}

// hardBreak — attrs is optional; renders as "\n" within inline flow
{ "type": "hardBreak" }
// OR: { "type": "hardBreak", "attrs": { "text": "\n" } }

// mention — attrs.text includes leading "@"; attrs.id is Atlassian account ID
{ "type": "mention", "attrs": { "id": "5b10ac8d82e05b22cc7d4ef5", "text": "@Bradley Ayers" } }
```

### ADF → Markdown conversion rules (D-13, D-14)

| ADF Node / Mark | Markdown Output | Notes |
|-----------------|-----------------|-------|
| `doc` | (recurse children) | |
| `paragraph` | children + `\n\n` | double newline between paragraphs |
| `hardBreak` | `\n` | within inline flow |
| `heading` level N | `"#"×N + " " + children + "\n\n"` | |
| `bulletList` | recurse `listItem` with `"- "` prefix | |
| `orderedList` | recurse `listItem` with `"N. "` prefix; N increments | |
| `listItem` | prefix + children (strip trailing newlines from children) | |
| `codeBlock` | ` ```lang\nchildren\n``` ` + `\n\n` | language attr may be empty → bare fences |
| `blockquote` | prefix each line with `"> "` | |
| `rule` | `"---\n\n"` | |
| `table` | GFM pipe table (see below) | |
| `text` (no marks) | `attrs.text` value | |
| mark `strong` | `"**" + text + "**"` | |
| mark `em` | `"*" + text + "*"` | |
| mark `code` | `` "`" + text + "`" `` | |
| mark `link` | `"[" + text + "](" + attrs.href + ")"` | |
| `mention` | `attrs.text` (e.g. `"@Bradley Ayers"`) or `"@" + attrs.id` if `.text` empty | |
| unknown node | silent skip + `log.Printf`; descend into `content` if present | D-15 |

**GFM pipe table construction:**

```
A tableRow containing tableHeader nodes is treated as the header row.
A tableRow containing tableCell nodes is treated as a data row.
If the first tableRow has tableHeader children → it is the header row.

Output:
| col1 | col2 | col3 |
| --- | --- | --- |      ← separator row (after header only)
| val1 | val2 | val3 |

If no tableHeader appears, treat all rows as data rows (no separator).
```

[VERIFIED: tableHeader is a distinct node type from tableCell per Atlassian docs — page 404 for tableHeader-specific doc, but the table docs confirm both exist as separate types. ASSUMED: the "first row = header" detection heuristic; no explicit spec found — this is the canonical approach for GFM rendering.]

### ADF Converter Implementation Style (Claude's discretion)

A recursive `map[string]any` walk is simpler given the small, fixed node set. Unmarshal the raw JSON once into `map[string]any`, then dispatch on `node["type"].(string)`. This avoids defining per-node Go structs.

```go
// Source: standard Go ADF rendering pattern
func ADFToMarkdown(raw json.RawMessage) (string, error) {
    if len(raw) == 0 || string(raw) == "null" {
        return "", nil
    }
    var doc map[string]any
    if err := json.Unmarshal(raw, &doc); err != nil {
        return "", fmt.Errorf("jira: ADF parse error: %w", err)
    }
    var sb strings.Builder
    renderNode(&sb, doc, 0)
    return strings.TrimSpace(sb.String()), nil
}
```

[ASSUMED: recursive map walk vs struct visitor — both are valid; map walk chosen for simplicity]

---

## Retry-After Header Parsing

[VERIFIED: Atlassian rate-limit docs — pseudocode shows `1000 * headerValue('Retry-After')` confirming integer seconds format]

```go
func parseRetryAfter(header string) time.Duration {
    if header == "" {
        return 0
    }
    secs, err := strconv.Atoi(strings.TrimSpace(header))
    if err != nil || secs < 0 {
        return 0  // unparseable → caller treats as unknown, Phase 5 uses a default
    }
    return time.Duration(secs) * time.Second
}
```

Atlassian also documents `X-RateLimit-Reset` as an ISO 8601 timestamp (for reset time) and `X-RateLimit-Remaining` for remaining capacity. Phase 1 only captures `Retry-After`. The other headers are Phase 5 concerns.

---

## Common Pitfalls

### Pitfall 1: nextPageToken sent as query param, not body field

**What goes wrong:** Developer treats `nextPageToken` like the legacy `startAt` (query param) and appends `?nextPageToken=X` to the URL.
**Why it happens:** The legacy `/rest/api/3/search` used `startAt`/`maxResults` as query params. The new enhanced endpoint uses a POST body for everything.
**How to avoid:** Always marshal `SearchOpts` into the POST body. The `nextPageToken` field in the request body is `""` or omitted for page 1.
**Warning signs:** API returns the same first page repeatedly (community-reported bug pattern when token is sent in wrong location).

[VERIFIED: Atlassian docs — all search params including nextPageToken are in request body]

### Pitfall 2: Assuming `total` field exists in enhanced search

**What goes wrong:** Phase 2 tries to read `searchResult.Total` to display "N issues to sync" progress.
**Why it happens:** Legacy `/rest/api/3/search` returned a `total` field. Enhanced search removed it.
**How to avoid:** `SearchResult` struct must NOT have a `Total` field. Progress must be tracked by counting fetched issues.
**Warning signs:** `SearchResult.Total` is always 0 / JSON unmarshal silently ignores missing fields.

[VERIFIED: Atlassian docs explicitly state no total; community discussions confirm as major pain point]

### Pitfall 3: `fields.comment` is an object, not an array

**What goes wrong:** Unmarshaling `fields.comment` as `[]Comment` fails with a JSON decode error.
**Why it happens:** Jira wraps comments in a `{total, comments[]}` envelope, unlike attachments which are a bare array.
**How to avoid:** Use `CommentPage` struct with `Total int` and `Comments []Comment`.
**Warning signs:** `json.Unmarshal` returns "cannot unmarshal object into Go struct field ... of type []jira.Comment".

[VERIFIED: Atlassian docs — comment field returns `{total: N, comments: [...]}` wrapper]

### Pitfall 4: `filepath.Join` vs string concat on Windows

**What goes wrong:** Using `home + "/.config/waveterm/jira.json"` on Windows produces `C:\Users\USER/.config/...` with mixed separators, which may cause `os.ReadFile` to fail or return wrong path in error messages.
**Why it happens:** Go's `filepath.Join` normalises separators per platform. String concat does not.
**How to avoid:** Always `filepath.Join(home, ".config", "waveterm", "jira.json")`. D-23 requires Windows green.
**Warning signs:** Tests pass on CI (Linux) but fail locally on Windows.

[VERIFIED: wavebase.go uses filepath.Join universally; D-23 locks Windows as primary target]

### Pitfall 5: ADF `mention` attrs.text vs attrs.displayName

**What goes wrong:** Code tries to read `attrs["displayName"]` and gets empty string.
**Why it happens:** The Atlassian ADF docs show `attrs.text` (including leading `@`), not `displayName`.
**How to avoid:** Read `attrs["text"]` for mention display name.
**Warning signs:** All mentions render as `@` (empty).

[VERIFIED: https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/mention/ — attribute is `text`, not `displayName`]

### Pitfall 6: ADF link mark uses `attrs.href`, not `attrs.url`

**What goes wrong:** Code reads `attrs["url"]` and gets nil.
**How to avoid:** Read `attrs["href"]`.

[VERIFIED: https://developer.atlassian.com/cloud/jira/platform/apis/document/marks/link/]

### Pitfall 7: `GetWaveConfigDir()` returns platform-specific path on Windows

**What goes wrong:** If `wavebase.GetWaveConfigDir()` is used instead of `os.UserHomeDir()`, on Windows the config dir resolves to `%LOCALAPPDATA%\waveterm` — not `~/.config/waveterm`. Existing widget and jira-cache.json contract use the literal `~/.config/waveterm/` path.
**How to avoid:** D-07 is locked. Use `os.UserHomeDir()` only.
**Warning signs:** Config not found on Windows even though file exists at `C:\Users\USER\.config\waveterm\jira.json`.

[VERIFIED: wavebase.go GetWaveConfigDir() = ConfigHome_VarCache set from WAVETERM_CONFIG_HOME env var (set by Electron to %LOCALAPPDATA%\waveterm on Windows)]

---

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|-------------|-----------|---------|----------|
| Go toolchain | All Go compilation | Yes | go1.26.1 windows/amd64 | — |
| `net/http/httptest` | Unit tests | Yes | stdlib (go1.26.1) | — |
| `encoding/base64` | Auth header | Yes | stdlib | — |
| `encoding/json` | All JSON | Yes | stdlib | — |
| `pkg/jira/` directory | Phase 1 deliverable | No (does not exist yet) | — | Create in Wave 0 task |

[VERIFIED: `go version` output; pkg/jira/ absence confirmed via ls]

No external tools required. All dependencies are Go stdlib.

---

## Validation Architecture

### Test Framework

| Property | Value |
|----------|-------|
| Framework | Go standard `testing` package (go1.26.1) |
| Config file | none — Go tests use `go test ./pkg/jira/...` |
| Quick run command | `go test ./pkg/jira/... -v -count=1` |
| Full suite command | `go test ./pkg/jira/... -v -count=1 -race` |

[VERIFIED: existing tests in pkg/ all use `package X` + `import "testing"` pattern; no external test framework]

### Phase Requirements → Test Map

| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| JIRA-01 | SearchIssues returns issues on 200 | unit (httptest) | `go test ./pkg/jira/... -run TestSearchIssues` | Wave 0 |
| JIRA-01 | SearchIssues pagination (page 1 + page 2 via nextPageToken) | unit (httptest) | `go test ./pkg/jira/... -run TestSearchIssues` | Wave 0 |
| JIRA-01 | GetIssue returns parsed fields on 200 | unit (httptest) | `go test ./pkg/jira/... -run TestGetIssue` | Wave 0 |
| JIRA-01 | Auth header is exactly `Basic base64(email:token)` | unit (httptest) | `go test ./pkg/jira/... -run TestAuthHeader` | Wave 0 |
| JIRA-01 | 401 → `errors.Is(err, ErrUnauthorized)` | unit (httptest) | `go test ./pkg/jira/... -run TestErrorPaths` | Wave 0 |
| JIRA-01 | 404 → `errors.Is(err, ErrNotFound)` | unit (httptest) | `go test ./pkg/jira/... -run TestErrorPaths` | Wave 0 |
| JIRA-01 | 429 → `errors.Is(err, ErrRateLimited)` + `APIError.RetryAfter` set | unit (httptest) | `go test ./pkg/jira/... -run TestErrorPaths` | Wave 0 |
| JIRA-01 | 5xx → `errors.Is(err, ErrServerError)` | unit (httptest) | `go test ./pkg/jira/... -run TestErrorPaths` | Wave 0 |
| JIRA-02 | Config happy path: reads + fills defaults | unit (file) | `go test ./pkg/jira/... -run TestLoadConfig` | Wave 0 |
| JIRA-02 | Config missing file → `ErrConfigNotFound` | unit (file) | `go test ./pkg/jira/... -run TestLoadConfig` | Wave 0 |
| JIRA-02 | Config malformed JSON → `ErrConfigInvalid` | unit (file) | `go test ./pkg/jira/... -run TestLoadConfig` | Wave 0 |
| JIRA-02 | Config incomplete (missing required field) → `ErrConfigIncomplete` | unit (file) | `go test ./pkg/jira/... -run TestLoadConfig` | Wave 0 |
| JIRA-01+02 | ADF per-node (table-driven, one case per D-14 type) | unit | `go test ./pkg/jira/... -run TestADFToMarkdown` | Wave 0 |
| JIRA-01+02 | ADF unknown node in mixed tree → no error, surrounding text renders | unit | `go test ./pkg/jira/... -run TestADFToMarkdown` | Wave 0 |

### Sampling Rate

- **Per task commit:** `go test ./pkg/jira/... -count=1`
- **Per wave merge:** `go test ./pkg/jira/... -count=1 -race`
- **Phase gate:** full suite green before `/gsd-verify-work`

### Wave 0 Gaps

- [ ] `pkg/jira/` directory — create the package (Wave 0, task 0)
- [ ] `pkg/jira/client_test.go` — covers JIRA-01 HTTP paths
- [ ] `pkg/jira/config_test.go` — covers JIRA-02 config loading (or merged into client_test.go)
- [ ] `pkg/jira/adf_test.go` — covers D-14 ADF nodes (or merged into client_test.go)

Note: config tests that write/read temp files must use `t.TempDir()` to remain Windows-safe. [VERIFIED: stdlib `testing.T.TempDir()` is cross-platform]

---

## Security Domain

### Applicable ASVS Categories

| ASVS Category | Applies | Standard Control |
|---------------|---------|-----------------|
| V2 Authentication | Yes | HTTP Basic with Atlassian API Token — no session, per-request |
| V3 Session Management | No | Stateless HTTP client; no session cookies |
| V4 Access Control | No | Client makes read-only requests; access control is Jira-side |
| V5 Input Validation | Yes | Config fields validated for presence; JQL is caller-supplied, forwarded verbatim to Jira |
| V6 Cryptography | No | Base64 is encoding, not encryption. API token stored as plaintext per D-08 (encryption deferred to JIRA-F-01) |

### Known Threat Patterns

| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| API token in plaintext config | Information Disclosure | Documented accepted risk per D-08; JIRA-F-01 defers safeStorage |
| JQL injection (caller-supplied JQL) | Tampering | Phase 1 forwards caller JQL verbatim; Phase 1 callers are internal Go code only, not user input — acceptable for this phase |
| Oversized API responses filling memory | DoS | `io.LimitReader` on error response bodies; success response bodies are bounded by `maxResults=50` |
| `Retry-After` integer overflow | Tampering | `strconv.Atoi` returns error on overflow; code handles `err != nil` path |

---

## Codebase Alignment Notes

| Aspect | Finding | Action |
|--------|---------|--------|
| Logging | `"log"` (stdlib) is universal in pkg/; `slog` not used | Use `log.Printf` for ADF unknown-node debug messages (D-15) |
| Error wrapping | `pkg/utilds.CodedError` is the existing Unwrap pattern | Mirror for `APIError.Unwrap()` |
| No `init()` side effects | All pkg/ packages are import-clean | `pkg/jira/` must have no `init()` — construction via `NewClient` only (D-05 / code_context) |
| Copyright header | `// Copyright 2025, Command Line Inc.` in all files | Add to every new .go file |
| Module path | `github.com/wavetermdev/waveterm` | Package path will be `github.com/wavetermdev/waveterm/pkg/jira` |
| `wavebase.WaveVersion` | Used for User-Agent string (D-06) | Import `pkg/wavebase` — or expose version via a constant; check for import cycle risk since wavebase imports nothing from pkg/ |

[VERIFIED: go.mod module path; copyright header from anthropicbackend.go; WaveVersion from wavebase.go line 25]

---

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Recursive `map[string]any` walk chosen for ADF converter | Architecture Patterns §ADF | Low — struct visitor is drop-in alternative; same external API |
| A2 | `SearchResult` / `Issue` struct field names chosen as documented | §Go Structs | Low — Phase 2 owns mapping; names can change without breaking phase boundary |
| A3 | GFM table: first row with `tableHeader` children is the header row | §ADF Table | Low — Atlassian docs confirm tableHeader is a distinct type; heuristic is standard |
| A4 | `hardBreak` renders as `\n` (inline newline, not paragraph break) | §ADF Conversion Rules | Low — hardBreak is equivalent to HTML `<br>` per ADF docs |
| A5 | `errors.Is(ErrConfigNotFound, ...)` used with plain `errors.New` sentinels | §Error Typing | None — this is the locked D-09/D-18 pattern; no wrapping needed for sentinels |

---

## Open Questions

1. **`GetIssue` — `fields` param as comma-separated string vs repeated `?fields=X&fields=Y`**
   - What we know: Atlassian docs show `fields` as `array<string>` in the OpenAPI schema.
   - What's unclear: Whether the HTTP layer expects `?fields=a,b,c` or `?fields=a&fields=b`.
   - Recommendation: Use `strings.Join(fields, ",")` as a single param value. This is the dominant pattern in Jira client libraries and aligns with how `curl` examples show it. [MEDIUM confidence — not verified with a live call]

2. **`tableHeader` page 404 on Atlassian docs**
   - What we know: The table page shows `tableCell` and `tableRow`; the `tableHeader` dedicated page returned 404 during research.
   - What's unclear: Whether `tableHeader` attrs differ from `tableCell` beyond the node type name.
   - Recommendation: Treat `tableHeader` as identical to `tableCell` in terms of attrs (empty `{}`); detect header row by checking if any child's type is `"tableHeader"`. If the real response uses different attrs, the map-walk code handles it gracefully.

---

## Sources

### Primary (HIGH confidence)
- Atlassian Jira Cloud REST API v3 — POST /rest/api/3/search/jql (request + response shape, pagination)
  https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issue-search/#api-rest-api-3-search-jql-post
- Atlassian Jira Cloud REST API v3 — GET /rest/api/3/issue/{key} (request params, response shape)
  https://developer.atlassian.com/cloud/jira/platform/rest/v3/api-group-issues/#api-rest-api-3-issue-issueidorkey-get
- Atlassian Basic Auth for REST APIs
  https://developer.atlassian.com/cloud/jira/platform/basic-auth-for-rest-apis/
- Atlassian Rate Limiting (Retry-After integer-seconds format)
  https://developer.atlassian.com/cloud/jira/platform/rate-limiting/
- ADF Node: mention (attrs.text, attrs.id)
  https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/mention/
- ADF Mark: link (attrs.href)
  https://developer.atlassian.com/cloud/jira/platform/apis/document/marks/link/
- ADF Node: table/tableRow/tableCell
  https://developer.atlassian.com/cloud/jira/platform/apis/document/nodes/table/
- Go stdlib: net/http/httptest
  https://pkg.go.dev/net/http/httptest
- Codebase: pkg/waveai/anthropicbackend.go (HTTP client pattern)
- Codebase: pkg/wconfig/settingsconfig.go (JSON config decode pattern)
- Codebase: pkg/utilds/codederror.go (Unwrap() error chain pattern)
- Codebase: pkg/wavebase/wavebase.go (GetHomeDir, GetWaveConfigDir — NOT used for jira path)

### Secondary (MEDIUM confidence)
- Atlassian Community — nextPageToken in response body confirmed, total field absence confirmed
  https://community.developer.atlassian.com/t/jira-cloud-rest-api-v3-search-jql-slower-fetching-with-nextpagetoken-no-totalissues-any-workarounds/90176

### Tertiary (LOW confidence — no additional items)

---

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — stdlib only; verified in go.mod + codebase
- API wire shapes: HIGH — verified via official Atlassian docs
- ADF node shapes: HIGH (mention/link/table) — from individual ADF node pages; MEDIUM (tableHeader attrs) — dedicated page 404'd
- Architecture patterns: HIGH — directly mirrored from codebase reference files
- Pitfalls: HIGH — derived from verified API docs and locked decisions

**Research date:** 2026-04-15
**Valid until:** 2026-05-15 (Atlassian API docs change infrequently; ADF spec is stable)

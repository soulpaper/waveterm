---
phase: 01-jira-http-client-config
plan: 04
type: execute
wave: 3
depends_on: [01-02]
files_modified:
  - pkg/jira/client.go
autonomous: true
requirements: [JIRA-01]

must_haves:
  truths:
    - "NewClient(cfg Config) *Client returns a client with a 30s-timeout http.Client (D-05)"
    - "NewClientWithHTTP(cfg Config, hc *http.Client) *Client exists as test seam (D-05)"
    - "SearchIssues POSTs to {baseUrl}/rest/api/3/search/jql with JSON body {jql, maxResults, nextPageToken, fields} (D-01, RESEARCH §Pattern 1)"
    - "GetIssue GETs {baseUrl}/rest/api/3/issue/{key}?fields=a,b,c (comma-joined) (D-02)"
    - "When opts.Fields is nil/empty, GetIssue omits the fields query param entirely"
    - "Every request carries Authorization: Basic base64(email:apiToken), Accept: application/json, User-Agent: waveterm-jira/<version> (D-06)"
    - "Non-2xx responses return *APIError with StatusCode/Endpoint/Method/Body set; Unwrap() buckets to ErrUnauthorized/ErrForbidden/ErrNotFound/ErrRateLimited/ErrServerError (D-19)"
    - "429 response populates APIError.RetryAfter from Retry-After header (D-20)"
    - "MaxResults=0 in SearchOpts defaults to 50 in the request body (D-03)"
    - "nextPageToken is in request BODY, NOT query string (D-01, RESEARCH Pitfall 1)"
    - "SearchResult does NOT expose a Total field (RESEARCH Pitfall 2)"
    - "All 6 client_test.go functions pass on Windows"
  artifacts:
    - path: "pkg/jira/client.go"
      provides: "Client, NewClient, NewClientWithHTTP, SearchIssues, GetIssue, SearchResult, IssueRef, Issue, IssueFields, Attachment, CommentPage, Comment"
      contains: "func NewClient"
  key_links:
    - from: "SearchIssues"
      to: "POST {baseUrl}/rest/api/3/search/jql"
      via: "http.NewRequestWithContext + json.Marshal request body"
      pattern: "/rest/api/3/search/jql"
    - from: "GetIssue"
      to: "GET {baseUrl}/rest/api/3/issue/{key}"
      via: "strings.Join(fields, \",\") as query param"
      pattern: "/rest/api/3/issue/"
    - from: "every request"
      to: "Basic base64 auth + Accept + User-Agent + Content-Type"
      via: "setCommonHeaders helper"
      pattern: "Authorization.*Basic"
    - from: "non-2xx response"
      to: "*APIError with Unwrap() → sentinel"
      via: "errors.go APIError, parseRetryAfter for 429"
      pattern: "APIError\\{"
---

<objective>
Implement `pkg/jira/client.go` — the HTTP client that turns every client_test.go stub GREEN. This is the JIRA-01 endpoint: any Waveterm subsystem imports `pkg/jira`, constructs a client from Config, and calls SearchIssues / GetIssue.

Purpose: Closes JIRA-01 (HTTP request to Jira Cloud, Basic auth). Phase 2's refresh orchestrator consumes SearchIssues in a pagination loop + calls GetIssue per issue.

Output: One file (`client.go`) containing the Client struct, constructors, request methods, and response-parsing structs per RESEARCH §Go Structs for Response Parsing.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-jira-http-client-config/01-CONTEXT.md
@.planning/phases/01-jira-http-client-config/01-RESEARCH.md
@.planning/phases/01-jira-http-client-config/01-02-SUMMARY.md
@pkg/waveai/anthropicbackend.go
@pkg/wavebase/wavebase.go

<interfaces>
<!-- Already-implemented dependencies (Plan 02). Client.go USES these — does not redefine. -->

From pkg/jira/config.go (Plan 02):
```go
type Config struct {
    BaseUrl  string  // e.g. "https://kakaovx.atlassian.net"
    CloudId  string
    Email    string
    ApiToken string
    Jql      string
    PageSize int
}
const DefaultPageSize = 50
```

From pkg/jira/errors.go (Plan 02):
```go
type APIError struct {
    StatusCode int
    Endpoint   string
    Method     string
    Body       string
    RetryAfter time.Duration
}
func (e *APIError) Error() string
func (e *APIError) Unwrap() error  // returns ErrUnauthorized/ErrForbidden/ErrNotFound/ErrRateLimited/ErrServerError/nil by status bucket
func parseRetryAfter(header string) time.Duration  // private — use via package
```

From pkg/wavebase/wavebase.go:
```go
var WaveVersion = "0.0.0"  // used for User-Agent string per D-06
```

From pkg/jira/client_test.go (the test contract — these symbols MUST exist with these exact names):
- `type Client struct { ... }`
- `NewClient(cfg Config) *Client`
- `NewClientWithHTTP(cfg Config, hc *http.Client) *Client`
- `type SearchOpts struct { JQL string; NextPageToken string; Fields []string; MaxResults int }`
- `type SearchResult struct { Issues []IssueRef; NextPageToken string; IsLast bool }`
- `type IssueRef struct { ID string; Key string; /* + Fields */ }`
- `type Issue struct { ID string; Key string; Fields IssueFields }` with `Fields.Summary` readable as string
- `type GetIssueOpts struct { Fields []string }`
- `func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error)`
- `func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error)`

Reference pattern from pkg/waveai/anthropicbackend.go (RESEARCH §Pattern 5):
```go
resp, err := c.hc.Do(req)
if err != nil { return nil, fmt.Errorf("...: %w", err) }
defer resp.Body.Close()
if resp.StatusCode < 200 || resp.StatusCode >= 300 {
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))  // D-18 body cap
    // build *APIError and return
}
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement pkg/jira/client.go (Client + SearchIssues + GetIssue + response structs)</name>
  <files>pkg/jira/client.go</files>
  <read_first>
    - pkg/waveai/anthropicbackend.go (entire file — http.Client construction, request building, error categorization reference per RESEARCH §Pattern 1 and §Pattern 5)
    - pkg/wavebase/wavebase.go (line containing WaveVersion definition)
    - pkg/jira/config.go (Config struct + DefaultPageSize constant — just implemented in Plan 02)
    - pkg/jira/errors.go (APIError struct + parseRetryAfter function — just implemented in Plan 02)
    - pkg/jira/client_test.go (the contract — 6 Test functions drive the exact method signatures and header requirements)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md D-01, D-02, D-04, D-05, D-06, D-19, D-20
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §Jira API Wire Format, §Go Structs for Response Parsing, §Pattern 1 HTTP Client Construction, §Pattern 2 Basic Auth Header, §Pattern 5 Non-2xx Response Handling, §Pitfalls 1/2/3
  </read_first>
  <behavior>
    - TestAuthHeader_ExactBasicBase64: Authorization header is EXACTLY `"Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:test-token"))`. Accept is `"application/json"`. User-Agent starts with `"waveterm-jira/"`.
    - TestSearchIssues_Pagination: First call with empty NextPageToken returns {IsLast:false, NextPageToken:"tok2", Issues:[{Key:"ITSM-1"}]}. Second call with NextPageToken:"tok2" returns {IsLast:true, Issues:[{Key:"ITSM-2"}]}. Method is POST. Path is `/rest/api/3/search/jql`.
    - TestSearchIssues_RequestBodyShape: Request body contains `"jql":"project = ITSM"`, `"nextPageToken":"abc123"`, `"fields":["summary","status"]`, `"maxResults":50` (default when opts.MaxResults=0). URL does NOT contain `?nextPageToken=`.
    - TestGetIssue_FieldsAsCSV: URL path is `/rest/api/3/issue/ITSM-3135`. Query `fields` is EXACTLY `summary,status,comment` (comma-joined, single param). Response parses Issue.Key="ITSM-3135", Issue.Fields.Summary="Test".
    - TestGetIssue_NilFieldsOmitsQueryParam: When opts.Fields is empty, the URL raw query does NOT contain `fields=`.
    - TestErrorPaths: For each of 401/403/404/429/500/503, the error returned satisfies errors.Is against the matching sentinel AND errors.As into *APIError with correct StatusCode. For 429, APIError.RetryAfter == 7*time.Second (header "7").
  </behavior>
  <action>
Create `pkg/jira/client.go`. Mirror `pkg/waveai/anthropicbackend.go`'s HTTP pattern but store the http.Client in the struct (we make many requests per refresh; no reason to reconstruct).

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

// defaultTimeout is the http.Client timeout for a single Jira request (D-05).
// Jira's P99 for /search is well under 5s; 30s is generous headroom without
// hanging user-visible flows.
const defaultTimeout = 30 * time.Second

// errorBodyLimit caps how much of a non-2xx response body we retain in
// APIError.Body (per D-18). 1 KB is enough for a stack trace or JSON error
// envelope without letting a hostile server balloon memory.
const errorBodyLimit = 1024

// Client is a stateless-ish Jira Cloud REST v3 client. It holds a Config
// snapshot captured at construction time AND a private http.Client. No
// global singleton, no init() side effects — per CONTEXT §Code Context.
type Client struct {
	cfg Config
	hc  *http.Client
}

// NewClient constructs a Client with a 30s-timeout http.Client. This is the
// normal production entry point. Callers that need a custom http.Client
// (tests, or future phases that inject a rate-limiting Transport) use
// NewClientWithHTTP.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: defaultTimeout},
	}
}

// NewClientWithHTTP is the test seam (D-05). client_test.go passes
// httptest.NewServer().Client() here so that the test server's self-signed
// certificate is accepted.
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client {
	return &Client{cfg: cfg, hc: hc}
}

// SearchOpts is the input to SearchIssues (D-04 options-struct style).
type SearchOpts struct {
	JQL           string   // required
	NextPageToken string   // "" for first page
	Fields        []string // nil/empty → server default; else sent as JSON array
	MaxResults    int      // 0 → DefaultPageSize (50 per D-03)
}

// GetIssueOpts is the input to GetIssue.
type GetIssueOpts struct {
	Fields []string // nil/empty → server default (fields query param omitted)
}

// SearchResult is the response from POST /rest/api/3/search/jql. Per
// RESEARCH Pitfall 2, Atlassian's enhanced search response has NO total
// count — progress tracking is caller's job via fetched-so-far counting.
type SearchResult struct {
	Issues        []IssueRef `json:"issues"`
	NextPageToken string     `json:"nextPageToken"`
	IsLast        bool       `json:"isLast"`
}

// IssueRef is the shallow issue shape returned by search. Fields populated
// depend on what the caller requested via SearchOpts.Fields.
type IssueRef struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

// Issue is the full issue shape returned by GetIssue.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

// IssueFields is a best-effort decode of the Jira "fields" object. Fields
// not requested (or not present on the issue type) decode as zero values.
// Description and Comment.Body are kept as json.RawMessage so the caller can
// feed them into ADFToMarkdown when they need markdown output.
type IssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
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

// Attachment is an element of IssueFields.Attachment. Content is the direct
// download URL; Phase 5 will GET it with auth to stream to disk.
type Attachment struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
	Created   string `json:"created"`
	Content   string `json:"content"`
	Thumbnail string `json:"thumbnail"`
	Author    struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
}

// CommentPage is the wrapper object Jira returns in fields.comment. Per
// RESEARCH Pitfall 3 the comment field is an OBJECT (with total + comments),
// NOT a bare array. Unmarshaling into []Comment would fail decode.
type CommentPage struct {
	Total    int       `json:"total"`
	Comments []Comment `json:"comments"`
}

// Comment is a single issue comment. Body is ADF (json.RawMessage) so
// callers can pass it to ADFToMarkdown.
type Comment struct {
	ID     string `json:"id"`
	Author struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Body    json.RawMessage `json:"body"`
	Created string          `json:"created"`
	Updated string          `json:"updated"`
}

// searchRequest is the JSON body for POST /rest/api/3/search/jql. Kept
// private; SearchOpts is the caller-facing shape.
type searchRequest struct {
	JQL           string   `json:"jql"`
	MaxResults    int      `json:"maxResults"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
	Fields        []string `json:"fields,omitempty"`
}

// SearchIssues calls POST /rest/api/3/search/jql with the caller's JQL and
// pagination cursor. Per D-01/D-03/RESEARCH Pitfall 1, nextPageToken is in
// the BODY (not a query string) and MaxResults defaults to 50 when zero.
func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = DefaultPageSize
	}
	reqBody := searchRequest{
		JQL:           opts.JQL,
		MaxResults:    maxResults,
		NextPageToken: opts.NextPageToken,
		Fields:        opts.Fields,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("jira: marshal search request: %w", err)
	}

	endpoint := c.cfg.BaseUrl + "/rest/api/3/search/jql"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("jira: build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setCommonHeaders(req)

	var result SearchResult
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetIssue calls GET /rest/api/3/issue/{key}. Per D-02 the fields param is
// a single comma-joined query value, and if opts.Fields is nil/empty we
// OMIT the param entirely (Jira returns its default field set).
func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error) {
	if key == "" {
		return nil, fmt.Errorf("jira: issue key is required")
	}
	// PathEscape the key — keys are ASCII (ITSM-3135) in practice but be
	// defensive against future custom project prefixes.
	endpoint := c.cfg.BaseUrl + "/rest/api/3/issue/" + url.PathEscape(key)
	if len(opts.Fields) > 0 {
		// Comma-joined single param per RESEARCH §GET Issue Request and
		// §Open Question 1 (dominant Jira client library convention).
		endpoint += "?fields=" + url.QueryEscape(strings.Join(opts.Fields, ","))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jira: build getissue request: %w", err)
	}
	c.setCommonHeaders(req)

	var issue Issue
	if err := c.doJSON(req, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// setCommonHeaders applies Authorization, Accept, and User-Agent to every
// request (D-06). Content-Type is set by the caller when there is a body.
func (c *Client) setCommonHeaders(req *http.Request) {
	// Basic auth: base64(email + ":" + apiToken) per Atlassian docs.
	raw := c.cfg.Email + ":" + c.cfg.ApiToken
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "waveterm-jira/"+wavebase.WaveVersion)
}

// doJSON executes req, categorizes non-2xx into *APIError, decodes the
// success body into out. Body is always closed.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("jira: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Endpoint:   req.URL.Path,
			Method:     req.Method,
			Body:       string(body),
		}
		if resp.StatusCode == 429 {
			apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jira: decode response: %w", err)
	}
	return nil
}
```

Key implementation constraints (verify during code-review):
- `http.NewRequestWithContext` NOT `http.NewRequest` — context propagation is mandatory (every test passes `context.Background()`)
- `bytes.NewReader` for body (not `strings.NewReader`) so multiple reads (e.g. from middleware) work
- `io.LimitReader(resp.Body, errorBodyLimit)` BEFORE `io.ReadAll` on the error path (RESEARCH §Anti-Patterns)
- `base64.StdEncoding` NOT `base64.URLEncoding` (RFC 7617 / RESEARCH §Anti-Patterns)
- `url.PathEscape(key)` for the issue key + `url.QueryEscape(strings.Join(...))` for the fields value (belt-and-braces)
- Do NOT add a `Total` field to SearchResult (RESEARCH Pitfall 2 — enhanced search has no total)
- Do NOT send nextPageToken as a query parameter (RESEARCH Pitfall 1 — body only)
- `fields` array uses `omitempty` so nil/empty Fields omits the JSON key entirely
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go test ./pkg/jira/... -count=1 -race 2>&amp;1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/client.go` returns exit 0
    - `grep -q "^func NewClient(cfg Config) \*Client" pkg/jira/client.go`
    - `grep -q "^func NewClientWithHTTP(cfg Config, hc \*http.Client) \*Client" pkg/jira/client.go`
    - `grep -q "^func (c \*Client) SearchIssues(ctx context.Context, opts SearchOpts) (\*SearchResult, error)" pkg/jira/client.go`
    - `grep -q "^func (c \*Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (\*Issue, error)" pkg/jira/client.go`
    - `grep -q '"/rest/api/3/search/jql"' pkg/jira/client.go`
    - `grep -q '"/rest/api/3/issue/"' pkg/jira/client.go`
    - `grep -q "base64.StdEncoding.EncodeToString" pkg/jira/client.go`  (D-06, RFC 7617)
    - `grep -vq "base64.URLEncoding" pkg/jira/client.go`  (wrong encoding)
    - `grep -q "waveterm-jira/" pkg/jira/client.go`  (D-06 User-Agent)
    - `grep -q "wavebase.WaveVersion" pkg/jira/client.go`
    - `grep -q "30 \* time.Second" pkg/jira/client.go`  (D-05)
    - `grep -q "io.LimitReader(resp.Body, errorBodyLimit)" pkg/jira/client.go`  (body cap)
    - `grep -q "http.NewRequestWithContext" pkg/jira/client.go`  (context propagation)
    - `grep -q 'parseRetryAfter(resp.Header.Get("Retry-After"))' pkg/jira/client.go`  (D-20)
    - `grep -q "DefaultPageSize" pkg/jira/client.go`  (D-03)
    - `grep -vq '"total"' pkg/jira/client.go`  (Pitfall 2 — no total field in SearchResult)
    - `grep -q "strings.Join(opts.Fields" pkg/jira/client.go`  (D-02 comma-joined CSV)
    - `grep -vq "fields=.*nextPageToken" pkg/jira/client.go`  (Pitfall 1 — token NOT in URL)
    - `grep -q "type SearchResult struct {" pkg/jira/client.go`
    - `grep -q "type IssueRef struct {" pkg/jira/client.go`
    - `grep -q "type Issue struct {" pkg/jira/client.go`
    - `grep -q "type IssueFields struct {" pkg/jira/client.go`
    - `grep -q "type Attachment struct {" pkg/jira/client.go`
    - `grep -q "type CommentPage struct {" pkg/jira/client.go`
    - `grep -q "type Comment struct {" pkg/jira/client.go`
    - `go vet ./pkg/jira/...` exits 0
    - `go build ./pkg/jira/...` exits 0
    - `go test ./pkg/jira/... -count=1 -race` exits 0 — ALL tests GREEN (config + adf + client, full pkg green)
    - `go test ./pkg/jira/... -run "TestAuthHeader_ExactBasicBase64" -count=1` exits 0
    - `go test ./pkg/jira/... -run "TestSearchIssues_Pagination" -count=1` exits 0
    - `go test ./pkg/jira/... -run "TestSearchIssues_RequestBodyShape" -count=1` exits 0
    - `go test ./pkg/jira/... -run "TestGetIssue_FieldsAsCSV" -count=1` exits 0
    - `go test ./pkg/jira/... -run "TestGetIssue_NilFieldsOmitsQueryParam" -count=1` exits 0
    - `go test ./pkg/jira/... -run "TestErrorPaths" -count=1` exits 0
  </acceptance_criteria>
  <done>client.go compiles; all 6 client_test.go functions GREEN on Windows; JIRA-01 fully implemented end-to-end (construct client → call SearchIssues/GetIssue → receive parsed response or typed error).</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| Jira Cloud → Go process | HTTP response bodies (potentially hostile from a compromised MITM or malformed server response) |
| Go process → Jira Cloud | Outgoing auth headers and JQL strings |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-04-01 | I (Information Disclosure) | Auth header leakage in logs | mitigate | Client.doJSON does NOT log request or response bodies. Error formatting (APIError.Error()) omits headers entirely. `grep 'log\.' pkg/jira/client.go` returns only zero matches (no logging in client.go). |
| T-01-04-02 | D (DoS) | Oversized error body filling memory | mitigate (per RESEARCH Security Domain) | `io.LimitReader(resp.Body, errorBodyLimit)` caps error bodies at 1 KB. Grep check: `grep 'io.LimitReader.*errorBodyLimit' pkg/jira/client.go` finds the cap. Success bodies are bounded by Jira's maxResults=50 per page (typical response <500KB). |
| T-01-04-03 | T (Tampering) | Retry-After integer overflow | mitigate (per RESEARCH Security Domain) | parseRetryAfter in errors.go handles `strconv.Atoi` error path and negative values. Client.go only wires the header into APIError; it does not compute delays. Phase 5 (future) consumes RetryAfter and must bound it (outside Phase 1 scope). |
| T-01-04-04 | T (Tampering) | JQL injection | accept (per RESEARCH Security Domain) | Phase 1 callers are internal Go code (Phase 2 refresh orchestrator). JQL strings originate from the user's own jira.json (not external input). A user who writes hostile JQL only affects their own Jira account — the threat model is self-harm, not cross-user tampering. JIRA-F-02 (React settings modal, future milestone) would add input validation at the UI boundary. |
| T-01-04-05 | S (Spoofing) | TLS to Jira Cloud | mitigate | http.Client uses Go's default TLS config (system cert pool, hostname verification ON). No `InsecureSkipVerify` anywhere. `grep 'InsecureSkipVerify' pkg/jira/` returns zero matches. httptest.NewServer in tests uses a self-signed cert, but tests explicitly pass `srv.Client()` which trusts that one cert — production never enters that code path. |
| T-01-04-06 | I (Information Disclosure) | API token in base64 auth header | accept | Basic auth over TLS is the Atlassian-recommended pattern for API tokens. Base64 is encoding not encryption (hence "accept"). Encryption at rest is deferred to JIRA-F-01 (safeStorage). In-transit confidentiality is provided by TLS. |
| T-01-04-07 | T (Tampering) | Malformed JSON response crashing client | mitigate | `json.NewDecoder(resp.Body).Decode(out)` returns typed error; we wrap it with `fmt.Errorf("jira: decode response: %w", err)`. No panic path. Unmarshal into IssueFields uses json.RawMessage for description/comment body so the ADF structure is opaque at this layer — tamper-resistant at parse time. |
</threat_model>

<verification>
After the task:
- `go vet ./pkg/jira/...` exits 0
- `go build ./pkg/jira/...` exits 0
- `go test ./pkg/jira/... -count=1 -race` exits 0 — ENTIRE pkg/jira test suite GREEN
- `go test ./... -count=1` (full workspace) exits 0 — no regressions elsewhere
- All 4 Phase 1 success criteria verifiable:
  - #1 SearchIssues returns keys + cursor → TestSearchIssues_Pagination, TestSearchIssues_RequestBodyShape GREEN
  - #2 GetIssue returns parsed description + attachments + comments → TestGetIssue_FieldsAsCSV GREEN
  - #3 LoadConfig reads jira.json → TestLoadConfig_* GREEN (from Plan 02)
  - #4 Unit tests cover 200/401/429/5xx, pass on Windows → TestErrorPaths + TestAuthHeader GREEN
</verification>

<success_criteria>
- pkg/jira is fully importable and testable end-to-end
- JIRA-01 requirement is closed (HTTP request with PAT auth)
- Test output on Windows shows 0 failures across the entire `pkg/jira/...` suite
- No use of http.DefaultClient, no hardcoded `/` separators, no base64.URLEncoding
- Response structs match RESEARCH §Go Structs for Response Parsing (no extra Total field)
</success_criteria>

<output>
After completion, create `.planning/phases/01-jira-http-client-config/01-04-SUMMARY.md` recording:
- Full test output (`go test ./pkg/jira/... -count=1 -race -v`)
- Mapping: each D-22 coverage item → test name → PASS
- Confirmation that Phase 1 ROADMAP success criteria 1–4 are all GREEN
- Note that Phase 2 can now import `pkg/jira` and call Client.SearchIssues / Client.GetIssue / ADFToMarkdown with the types documented here
</output>

---
phase: 01-jira-http-client-config
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - pkg/jira/doc.go
  - pkg/jira/client_test.go
  - pkg/jira/config_test.go
  - pkg/jira/adf_test.go
autonomous: true
requirements: [JIRA-01, JIRA-02]

must_haves:
  truths:
    - "pkg/jira/ directory exists and is importable (go build ./pkg/jira/... succeeds)"
    - "Stub test files exist with a failing TestPlaceholder in each (Nyquist: tests exist BEFORE implementation)"
    - "No production code yet — only package declaration + test scaffolds"
  artifacts:
    - path: "pkg/jira/doc.go"
      provides: "package jira declaration + copyright header"
      contains: "package jira"
    - path: "pkg/jira/client_test.go"
      provides: "test stubs for SearchIssues, GetIssue, auth header, 401/404/429/5xx"
      contains: "package jira"
    - path: "pkg/jira/config_test.go"
      provides: "test stubs for LoadConfig (happy, missing, malformed, incomplete, defaults)"
      contains: "package jira"
    - path: "pkg/jira/adf_test.go"
      provides: "test stubs for ADFToMarkdown (per-node + unknown-node)"
      contains: "package jira"
  key_links:
    - from: "all *_test.go files"
      to: "pkg/jira package"
      via: "package jira (white-box tests, NOT jira_test)"
      pattern: "^package jira$"
---

<objective>
Create the `pkg/jira/` Go package directory and write failing stub test files for every acceptance criterion locked in CONTEXT.md D-22 + VALIDATION.md §Wave 0.

Purpose: Nyquist compliance — tests exist before implementation so every subsequent task can verify against a concrete `go test` command. This plan's tests MUST FAIL (RED state) because production files don't exist yet; implementation plans (02, 03, 04) turn them GREEN.

Output: Four files in `pkg/jira/` — one package declaration file (`doc.go`) and three test scaffolds.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/STATE.md
@.planning/phases/01-jira-http-client-config/01-CONTEXT.md
@.planning/phases/01-jira-http-client-config/01-RESEARCH.md
@.planning/phases/01-jira-http-client-config/01-VALIDATION.md

<interfaces>
<!-- Contracts downstream plans will implement. Test stubs reference these names so later plans can't drift. -->

From pkg/jira (to be created in Plan 02):
```go
// config.go
type Config struct {
    BaseUrl  string `json:"baseUrl"`
    CloudId  string `json:"cloudId"`
    Email    string `json:"email"`
    ApiToken string `json:"apiToken"`
    Jql      string `json:"jql"`
    PageSize int    `json:"pageSize"`
}
func LoadConfig() (Config, error)
func LoadConfigFromPath(path string) (Config, error)  // test seam

// errors.go
var (
    ErrUnauthorized     = errors.New("jira: unauthorized")      // 401
    ErrForbidden        = errors.New("jira: forbidden")         // 403
    ErrNotFound         = errors.New("jira: not found")         // 404
    ErrRateLimited      = errors.New("jira: rate limited")      // 429
    ErrServerError      = errors.New("jira: server error")      // 5xx
    ErrConfigNotFound   = errors.New("jira: config not found")
    ErrConfigInvalid    = errors.New("jira: config invalid")
    ErrConfigIncomplete = errors.New("jira: config incomplete")
)
type APIError struct {
    StatusCode int
    Endpoint   string
    Method     string
    Body       string
    RetryAfter time.Duration
}
func (e *APIError) Error() string
func (e *APIError) Unwrap() error
```

From pkg/jira (to be created in Plan 03):
```go
// client.go
type Client struct { /* private */ }
func NewClient(cfg Config) *Client
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client

type SearchOpts struct {
    JQL           string
    NextPageToken string
    Fields        []string
    MaxResults    int
}
type SearchResult struct {
    Issues        []IssueRef `json:"issues"`
    NextPageToken string     `json:"nextPageToken"`
    IsLast        bool       `json:"isLast"`
}
func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error)

type GetIssueOpts struct { Fields []string }
func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error)
```

From pkg/jira (to be created in Plan 04):
```go
// adf.go
func ADFToMarkdown(raw json.RawMessage) (string, error)
```
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create pkg/jira/ package with doc.go</name>
  <files>pkg/jira/doc.go</files>
  <read_first>
    - pkg/waveai/anthropicbackend.go (copyright header format — line 1)
    - pkg/wavebase/wavebase.go (copyright header format — line 1)
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §Codebase Alignment Notes
  </read_first>
  <action>
Create `pkg/jira/doc.go` with EXACTLY this content (copyright header matches all other pkg/ files per RESEARCH §Codebase Alignment):

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package jira provides a Go client for Atlassian Jira Cloud REST API v3.
// It authenticates via HTTP Basic (email + API token), supports cursor-based
// search (POST /rest/api/3/search/jql), single-issue retrieval, and a minimal
// ADF → markdown converter for issue descriptions and comment bodies.
//
// The package is stdlib-only and has no init() side effects. Construct via
// NewClient(cfg) or NewClientWithHTTP(cfg, hc) for tests.
package jira
```

Do NOT add any other symbols to this file — it is package documentation only. All production code goes in config.go / errors.go / client.go / adf.go (created in later plans).
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go build ./pkg/jira/... 2>&amp;1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/doc.go` returns exit 0
    - `grep -q '^package jira$' pkg/jira/doc.go` returns exit 0
    - `grep -q '// Copyright 2025, Command Line Inc.' pkg/jira/doc.go` returns exit 0
    - `grep -q 'SPDX-License-Identifier: Apache-2.0' pkg/jira/doc.go` returns exit 0
    - `go build ./pkg/jira/...` exits 0 (empty package compiles)
  </acceptance_criteria>
  <done>Package pkg/jira exists, compiles, has copyright + SPDX header.</done>
</task>

<task type="auto">
  <name>Task 2: Create pkg/jira/config_test.go stubs for JIRA-02 coverage</name>
  <files>pkg/jira/config_test.go</files>
  <read_first>
    - pkg/jira/doc.go (just created — confirm package name)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md §Implementation Decisions D-07..D-12, D-22
    - .planning/phases/01-jira-http-client-config/01-VALIDATION.md §Wave 0 Requirements
  </read_first>
  <action>
Create `pkg/jira/config_test.go` with 5 test functions covering D-22 config cases. Tests MUST fail to compile because `LoadConfigFromPath`, `Config`, `ErrConfigNotFound`, `ErrConfigInvalid`, `ErrConfigIncomplete` don't exist yet — that's the Nyquist RED state.

Use `t.TempDir()` for all file operations (D-23 Windows-safety). Use `filepath.Join` — NEVER string-concat paths with `/`.

Required file content:

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeConfigFile is a helper used across config tests. It writes `contents`
// to a file named jira.json inside t.TempDir() and returns the full path.
func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "jira.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfig_Happy(t *testing.T) {
	path := writeConfigFile(t, `{
		"baseUrl":  "https://kakaovx.atlassian.net",
		"cloudId":  "280eeb13-4c6a-4dc3-aec5-c5f9385c7a7d",
		"email":    "spike@kakaovx.com",
		"apiToken": "ATATT-xxx",
		"jql":      "project = ITSM",
		"pageSize": 25
	}`)
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseUrl != "https://kakaovx.atlassian.net" {
		t.Errorf("BaseUrl: got %q", cfg.BaseUrl)
	}
	if cfg.Email != "spike@kakaovx.com" {
		t.Errorf("Email: got %q", cfg.Email)
	}
	if cfg.ApiToken != "ATATT-xxx" {
		t.Errorf("ApiToken: got %q", cfg.ApiToken)
	}
	if cfg.Jql != "project = ITSM" {
		t.Errorf("Jql: got %q", cfg.Jql)
	}
	if cfg.PageSize != 25 {
		t.Errorf("PageSize: got %d want 25", cfg.PageSize)
	}
}

func TestLoadConfig_DefaultsFill(t *testing.T) {
	// Omit jql and pageSize; loader must fill them per D-03.
	path := writeConfigFile(t, `{
		"baseUrl":  "https://kakaovx.atlassian.net",
		"email":    "spike@kakaovx.com",
		"apiToken": "ATATT-xxx"
	}`)
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Jql != "assignee = currentUser() ORDER BY updated DESC" {
		t.Errorf("Jql default: got %q", cfg.Jql)
	}
	if cfg.PageSize != 50 {
		t.Errorf("PageSize default: got %d want 50", cfg.PageSize)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	// Path points into a temp dir but the file is never created.
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("want ErrConfigNotFound, got %v", err)
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	path := writeConfigFile(t, `{this is not valid json`)
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("want ErrConfigInvalid, got %v", err)
	}
}

func TestLoadConfig_Incomplete(t *testing.T) {
	// Missing baseUrl, email, apiToken → must name all three in error message.
	path := writeConfigFile(t, `{"cloudId":"abc"}`)
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigIncomplete) {
		t.Fatalf("want ErrConfigIncomplete, got %v", err)
	}
	msg := err.Error()
	for _, field := range []string{"baseUrl", "email", "apiToken"} {
		if !stringsContains(msg, field) {
			t.Errorf("error %q does not mention missing field %q", msg, field)
		}
	}
}

// stringsContains is a local helper to avoid importing "strings" inside tests
// that already do enough with stdlib. Replace with strings.Contains if preferred
// during implementation.
func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

Note: `stringsContains` is a local helper so this test file has zero external deps beyond stdlib. The implementation in Plan 02 may replace it with `strings.Contains`.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go vet ./pkg/jira/... 2>&amp;1 | grep -q "undefined: LoadConfigFromPath\|undefined: Config\|undefined: ErrConfig" &amp;&amp; echo "expected RED state confirmed"</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/config_test.go` returns exit 0
    - `grep -c "^func Test" pkg/jira/config_test.go` outputs `5`
    - `grep -q "TestLoadConfig_Happy" pkg/jira/config_test.go`
    - `grep -q "TestLoadConfig_DefaultsFill" pkg/jira/config_test.go`
    - `grep -q "TestLoadConfig_MissingFile" pkg/jira/config_test.go`
    - `grep -q "TestLoadConfig_MalformedJSON" pkg/jira/config_test.go`
    - `grep -q "TestLoadConfig_Incomplete" pkg/jira/config_test.go`
    - `grep -q "t.TempDir()" pkg/jira/config_test.go` (Windows-safe per D-23)
    - `grep -q "filepath.Join" pkg/jira/config_test.go` (Windows-safe per D-23)
    - `grep -vq '".*/.config/waveterm.*"' pkg/jira/config_test.go` — NO hardcoded POSIX paths
    - `go test ./pkg/jira/... -count=1` exits NON-ZERO (compile failure expected — implementation comes in Plan 02; this is the Nyquist RED state)
  </acceptance_criteria>
  <done>Stub test file exists with all 5 D-22 config test cases; uses t.TempDir() and filepath.Join; currently fails to compile (expected).</done>
</task>

<task type="auto">
  <name>Task 3: Create pkg/jira/client_test.go and adf_test.go stubs</name>
  <files>pkg/jira/client_test.go, pkg/jira/adf_test.go</files>
  <read_first>
    - pkg/jira/doc.go (package name)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md D-04, D-06, D-18, D-19, D-20, D-22
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §Pattern 6 httptest Table-Driven Tests, §Jira API Wire Format, §ADF Node Shapes
  </read_first>
  <action>
Create TWO files. Both currently fail to compile (Nyquist RED) because Client/SearchIssues/GetIssue/ADFToMarkdown don't exist yet.

**File 1: `pkg/jira/client_test.go`** — stubs covering JIRA-01 coverage matrix (auth, search, getissue, 401/404/429/5xx). Use `httptest.NewServer` per D-21.

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a Client pointed at srv.URL using srv.Client().
// All HTTP tests funnel through this helper so auth/header assertions are
// consistent.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	cfg := Config{
		BaseUrl:  srv.URL,
		CloudId:  "test-cloud-id",
		Email:    "user@example.com",
		ApiToken: "test-token",
		Jql:      "assignee = currentUser()",
		PageSize: 50,
	}
	return NewClientWithHTTP(cfg, srv.Client())
}

func TestAuthHeader_ExactBasicBase64(t *testing.T) {
	// D-06: Authorization: Basic base64("user@example.com:test-token")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:test-token"))
	var gotAuth, gotAccept, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"issues":[],"isLast":true,"nextPageToken":""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != expected {
		t.Errorf("Authorization header: got %q want %q", gotAuth, expected)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept header: got %q", gotAccept)
	}
	if !strings.HasPrefix(gotUA, "waveterm-jira/") {
		t.Errorf("User-Agent: got %q want prefix waveterm-jira/", gotUA)
	}
}

func TestSearchIssues_Pagination(t *testing.T) {
	// First call: no token → server returns nextPageToken=tok2, isLast=false.
	// Second call: token=tok2 → server returns isLast=true.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if calls == 1 {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"ITSM-1"}],"isLast":false,"nextPageToken":"tok2"}`)
		} else {
			fmt.Fprint(w, `{"issues":[{"id":"2","key":"ITSM-2"}],"isLast":true,"nextPageToken":""}`)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	page1, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X"})
	if err != nil {
		t.Fatalf("page1 error: %v", err)
	}
	if page1.IsLast {
		t.Error("page1.IsLast should be false")
	}
	if page1.NextPageToken != "tok2" {
		t.Errorf("page1.NextPageToken: got %q want tok2", page1.NextPageToken)
	}
	if len(page1.Issues) != 1 || page1.Issues[0].Key != "ITSM-1" {
		t.Errorf("page1 issues wrong: %+v", page1.Issues)
	}

	page2, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X", NextPageToken: "tok2"})
	if err != nil {
		t.Fatalf("page2 error: %v", err)
	}
	if !page2.IsLast {
		t.Error("page2.IsLast should be true")
	}
	if len(page2.Issues) != 1 || page2.Issues[0].Key != "ITSM-2" {
		t.Errorf("page2 issues wrong: %+v", page2.Issues)
	}
}

func TestSearchIssues_RequestBodyShape(t *testing.T) {
	// D-01: nextPageToken is in the request BODY, not query string.
	// D-03: MaxResults=0 → defaults to 50.
	var bodyStr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		bodyStr = string(buf[:n])
		w.WriteHeader(200)
		fmt.Fprint(w, `{"issues":[],"isLast":true,"nextPageToken":""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.SearchIssues(context.Background(), SearchOpts{
		JQL:           "project = ITSM",
		NextPageToken: "abc123",
		Fields:        []string{"summary", "status"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`"jql"`, `"project = ITSM"`,
		`"nextPageToken"`, `"abc123"`,
		`"fields"`, `"summary"`, `"status"`,
		`"maxResults"`, `50`, // default applied
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("request body missing %q; full body: %s", want, bodyStr)
		}
	}
	// Must NOT send nextPageToken as a URL query param.
	if strings.Contains(bodyStr, "?nextPageToken=") {
		t.Error("nextPageToken must be in body, not URL")
	}
}

func TestGetIssue_FieldsAsCSV(t *testing.T) {
	// D-02: GET /rest/api/3/issue/{key}?fields=a,b,c (comma-joined)
	var gotQuery string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"10001","key":"ITSM-3135","fields":{"summary":"Test"}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	iss, err := c.GetIssue(context.Background(), "ITSM-3135", GetIssueOpts{
		Fields: []string{"summary", "status", "comment"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/rest/api/3/issue/ITSM-3135" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotQuery != "summary,status,comment" {
		t.Errorf("fields query: got %q want summary,status,comment", gotQuery)
	}
	if iss.Key != "ITSM-3135" {
		t.Errorf("Key: got %q", iss.Key)
	}
	if iss.Fields.Summary != "Test" {
		t.Errorf("Summary: got %q", iss.Fields.Summary)
	}
}

func TestGetIssue_NilFieldsOmitsQueryParam(t *testing.T) {
	// When opts.Fields is nil/empty, do NOT send ?fields= (let Jira return defaults).
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","key":"K","fields":{}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.GetIssue(context.Background(), "K", GetIssueOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(rawQuery, "fields=") {
		t.Errorf("expected no fields= param, got query %q", rawQuery)
	}
}

func TestErrorPaths(t *testing.T) {
	cases := []struct {
		name        string
		status      int
		body        string
		retryAfter  string
		wantSentinel error
		wantRetryAfter time.Duration
	}{
		{"401 unauthorized", 401, `{"errorMessages":["bad token"]}`, "", ErrUnauthorized, 0},
		{"403 forbidden", 403, `{"errorMessages":["forbidden"]}`, "", ErrForbidden, 0},
		{"404 not found", 404, `{"errorMessages":["no issue"]}`, "", ErrNotFound, 0},
		{"429 rate limited with Retry-After", 429, `{"errorMessages":["slow down"]}`, "7", ErrRateLimited, 7 * time.Second},
		{"500 server error", 500, `internal error`, "", ErrServerError, 0},
		{"503 server error", 503, `gateway busy`, "", ErrServerError, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := newTestClient(t, srv)
			_, err := c.GetIssue(context.Background(), "KEY-1", GetIssueOpts{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.wantSentinel) {
				t.Errorf("errors.Is failed: err=%v want sentinel=%v", err, tc.wantSentinel)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As(*APIError) failed for %v", err)
			}
			if apiErr.StatusCode != tc.status {
				t.Errorf("StatusCode: got %d want %d", apiErr.StatusCode, tc.status)
			}
			if apiErr.RetryAfter != tc.wantRetryAfter {
				t.Errorf("RetryAfter: got %v want %v", apiErr.RetryAfter, tc.wantRetryAfter)
			}
		})
	}
}
```

**File 2: `pkg/jira/adf_test.go`** — stubs covering D-14 ADF node set + D-15 unknown-node case.

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestADFToMarkdown_EmptyInput(t *testing.T) {
	// Empty raw JSON and literal null must return empty string, no error.
	for _, in := range []string{"", "null"} {
		out, err := ADFToMarkdown(json.RawMessage(in))
		if err != nil {
			t.Errorf("input %q: unexpected error %v", in, err)
		}
		if out != "" {
			t.Errorf("input %q: got %q want empty", in, out)
		}
	}
}

func TestADFToMarkdown_MalformedJSON(t *testing.T) {
	_, err := ADFToMarkdown(json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestADFToMarkdown_PerNodeType(t *testing.T) {
	// Table-driven per D-14. Each case supplies an ADF doc and a substring
	// that must appear in the markdown output. Full markdown assertions are
	// fleshed out in Plan 05 (TestFleshing); this stub only confirms the
	// converter runs and produces SOMETHING containing the expected marker.
	cases := []struct {
		name  string
		adf   string
		want  string
	}{
		{"paragraph", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`, "hello"},
		{"heading h2", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]}]}`, "## Title"},
		{"bulletList", `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]}]}]}`, "- A"},
		{"orderedList", `{"type":"doc","content":[{"type":"orderedList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]}]}`, "1. A"},
		{"codeBlock", `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"x := 1"}]}]}`, "```go"},
		{"blockquote", `{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"Q"}]}]}]}`, "> Q"},
		{"rule", `{"type":"doc","content":[{"type":"rule"}]}`, "---"},
		{"hardBreak", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}`, "a\nb"},
		{"mark strong", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]}]}]}`, "**bold**"},
		{"mark em", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"it","marks":[{"type":"em"}]}]}]}`, "*it*"},
		{"mark code", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"code"}]}]}]}`, "`x`"},
		{"mark link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"go","marks":[{"type":"link","attrs":{"href":"https://go.dev"}}]}]}]}`, "[go](https://go.dev)"},
		{"mention with text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"5b10","text":"@Bradley"}}]}]}`, "@Bradley"},
		{"mention fallback to id", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"5b10"}}]}]}`, "@5b10"},
		{"table with header row", `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"H1"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"H2"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"c1"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"c2"}]}]}]}]}]}`, "| H1 | H2 |"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ADFToMarkdown(json.RawMessage(tc.adf))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output missing %q; full output:\n%s", tc.want, out)
			}
		})
	}
}

func TestADFToMarkdown_UnknownNodeInMixedTree(t *testing.T) {
	// D-15: unknown node types silently skipped; surrounding text still renders.
	// A "panel" node wraps a paragraph with real text. Panel is NOT in D-14;
	// converter must descend into its content (or skip it entirely while
	// preserving siblings). The "after" text must appear in output.
	adf := `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"panel","attrs":{"panelType":"info"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"inside-panel"}]}
		]},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}`
	out, err := ADFToMarkdown(json.RawMessage(adf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "before") {
		t.Errorf("missing 'before' in output: %s", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("missing 'after' in output: %s", out)
	}
	// The converter is allowed either to render "inside-panel" (descend into
	// unknown's children) or to drop the entire unknown subtree; both are
	// spec-compliant per D-15. We do NOT assert on "inside-panel" here.
}
```

Both files should be `package jira` (white-box) so they can reference unexported types if needed.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; (go test ./pkg/jira/... -count=1 2>&amp;1 | grep -q "undefined\|Client\|SearchIssues\|ADFToMarkdown") &amp;&amp; echo "expected RED state confirmed"</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/client_test.go` returns exit 0
    - `test -f pkg/jira/adf_test.go` returns exit 0
    - `grep -c "^func Test" pkg/jira/client_test.go` outputs `6`  (TestAuthHeader_ExactBasicBase64, TestSearchIssues_Pagination, TestSearchIssues_RequestBodyShape, TestGetIssue_FieldsAsCSV, TestGetIssue_NilFieldsOmitsQueryParam, TestErrorPaths)
    - `grep -c "^func Test" pkg/jira/adf_test.go` outputs `4`  (TestADFToMarkdown_EmptyInput, TestADFToMarkdown_MalformedJSON, TestADFToMarkdown_PerNodeType, TestADFToMarkdown_UnknownNodeInMixedTree)
    - `grep -q "httptest.NewServer" pkg/jira/client_test.go`  (D-21)
    - `grep -q "NewClientWithHTTP" pkg/jira/client_test.go`  (D-05 test seam)
    - `grep -q "base64.StdEncoding" pkg/jira/client_test.go`  (D-06 exact auth shape)
    - `grep -q "errors.Is" pkg/jira/client_test.go`  (D-19 sentinel pattern)
    - `grep -q "errors.As" pkg/jira/client_test.go`  (D-19 struct pattern)
    - `grep -q "Retry-After" pkg/jira/client_test.go`  (D-20)
    - `grep -q "tableHeader" pkg/jira/adf_test.go`  (D-14)
    - `grep -q "mention" pkg/jira/adf_test.go`  (D-14)
    - `go test ./pkg/jira/... -count=1` exits NON-ZERO (Nyquist RED — implementations come in Plans 02/03/04)
  </acceptance_criteria>
  <done>All three test files exist with stubs for every D-22 coverage case; they fail to compile (expected RED state); ready for Plans 02/03/04 to implement against.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| N/A (scaffolding) | This plan only creates empty files and test stubs; no runtime behavior, no trust boundary crossed |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-00-01 | I (Information Disclosure) | test fixture strings (email + token) | accept | Test fixtures use obviously-fake values (`user@example.com`, `test-token`, `ATATT-xxx`); no real credentials committed. Grep check: `grep -E "kakaovx|ATATT-[A-Z0-9]{10,}" pkg/jira/*_test.go` must find only the fake `ATATT-xxx` string, never a real token. |
</threat_model>

<verification>
After completing all tasks:
- `ls pkg/jira/` shows `doc.go`, `client_test.go`, `config_test.go`, `adf_test.go`
- `go build ./pkg/jira/...` succeeds (package compiles even though tests don't)
- `go test ./pkg/jira/... -count=1` fails with undefined-symbol errors (expected Nyquist RED — every downstream task now has a failing test to turn GREEN)
- Every test name in D-22's coverage list maps to a stub in one of the three test files
</verification>

<success_criteria>
- pkg/jira/ directory exists and is importable
- Exactly 3 test files with Nyquist-compliant stubs covering all D-22 cases
- Zero production code (no config.go, errors.go, client.go, adf.go yet — those are Plans 02/03/04)
- Windows-safe: all temp paths via t.TempDir() + filepath.Join; no hardcoded `/` separators
- `go test ./pkg/jira/... -count=1` fails due to undefined symbols (expected RED)
</success_criteria>

<output>
After completion, create `.planning/phases/01-jira-http-client-config/01-01-SUMMARY.md` recording:
- Files created with line counts
- Confirmed RED state (paste go test error output)
- Test name → D-XX coverage mapping
</output>

---
phase: 01-jira-http-client-config
plan: 02
type: execute
wave: 2
depends_on: [01-01]
files_modified:
  - pkg/jira/config.go
  - pkg/jira/errors.go
autonomous: true
requirements: [JIRA-02, JIRA-01]

must_haves:
  truths:
    - "LoadConfig() reads ~/.config/waveterm/jira.json via os.UserHomeDir() (D-07)"
    - "LoadConfigFromPath(path) exists as the test seam and is what LoadConfig() calls"
    - "Missing file returns ErrConfigNotFound (D-09)"
    - "Malformed JSON returns error where errors.Is(err, ErrConfigInvalid) is true (D-10)"
    - "Missing required fields returns error where errors.Is(err, ErrConfigIncomplete) is true, with field names in message (D-11)"
    - "Defaults (Jql, PageSize=50) are filled silently on partial configs (D-03, D-11)"
    - "Loader re-reads on every call — no in-process caching (D-12)"
    - "APIError struct has StatusCode/Endpoint/Method/Body/RetryAfter; Unwrap() returns matching sentinel (D-18, D-19, D-20)"
    - "All 8 sentinel errors exported and errors.Is-compatible (D-18)"
    - "All 5 D-22 config test cases pass: TestLoadConfig_Happy, _DefaultsFill, _MissingFile, _MalformedJSON, _Incomplete"
  artifacts:
    - path: "pkg/jira/config.go"
      provides: "Config struct, LoadConfig(), LoadConfigFromPath(), ErrConfig* sentinels"
      contains: "func LoadConfig"
    - path: "pkg/jira/errors.go"
      provides: "APIError struct with Unwrap(); sentinel errors for HTTP status buckets"
      contains: "type APIError struct"
  key_links:
    - from: "LoadConfig()"
      to: "os.UserHomeDir() + filepath.Join(home, .config, waveterm, jira.json)"
      via: "D-07 literal path"
      pattern: "os\\.UserHomeDir"
    - from: "APIError.Unwrap()"
      to: "ErrUnauthorized/ErrForbidden/ErrNotFound/ErrRateLimited/ErrServerError"
      via: "switch on StatusCode"
      pattern: "case e.StatusCode"
---

<objective>
Implement the config loader and error types for `pkg/jira/`. These are the two independent foundation files that Plans 03 (client) and 04 (adf) depend on.

Purpose: Close JIRA-02 fully (config file semantics) and provide the error-typing surface JIRA-01 callers use (`errors.Is(err, ErrUnauthorized)` etc.) per D-09..D-12 and D-18..D-20.

Output: Two files (`config.go`, `errors.go`) that together turn `config_test.go` GREEN (5 tests) and make the error types referenced in `client_test.go` resolvable.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-jira-http-client-config/01-CONTEXT.md
@.planning/phases/01-jira-http-client-config/01-RESEARCH.md
@.planning/phases/01-jira-http-client-config/01-01-SUMMARY.md
@pkg/wconfig/settingsconfig.go
@pkg/utilds/codederror.go

<interfaces>
<!-- Existing patterns from the codebase that THIS plan mirrors. -->

From pkg/utilds/codederror.go (the Unwrap() reference pattern per RESEARCH §Pattern 4):
```go
// CodedError wraps an error with an HTTP-like status code. Its Unwrap()
// returns the inner error so errors.Is / errors.As work through the chain.
// APIError in this plan mirrors the same Unwrap() idea but returns a
// SENTINEL (not a wrapped cause) based on status bucket.
type CodedError struct { Code int; Err error }
func (e *CodedError) Error() string { ... }
func (e *CodedError) Unwrap() error { return e.Err }
```

From pkg/wconfig/settingsconfig.go (the config-load-with-defaults reference per RESEARCH §Pattern 3):
- Uses `os.ReadFile` + `json.Unmarshal` into a typed struct
- Defaults applied via conditional assignment after decode
- No background polling or in-process cache needed for this phase

Stub tests from Plan 01 (01-01) reference these exact symbols — if you rename them, the stubs fail:
- `Config` struct with fields: `BaseUrl, CloudId, Email, ApiToken, Jql, PageSize`
- `LoadConfigFromPath(path string) (Config, error)`
- `ErrConfigNotFound, ErrConfigInvalid, ErrConfigIncomplete`
- `ErrUnauthorized, ErrForbidden, ErrNotFound, ErrRateLimited, ErrServerError`
- `APIError` with fields `StatusCode, Endpoint, Method, Body, RetryAfter`
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement pkg/jira/errors.go with sentinels + APIError</name>
  <files>pkg/jira/errors.go</files>
  <read_first>
    - pkg/utilds/codederror.go (entire file — the Unwrap pattern this mirrors)
    - pkg/jira/client_test.go (lines containing `errors.Is`, `errors.As`, `APIError`, `RetryAfter` — the contract)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md D-18, D-19, D-20
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §Pattern 4 Error Typing with Unwrap Chain, §Retry-After Header Parsing
  </read_first>
  <behavior>
    - `errors.Is(&amp;APIError{StatusCode: 401}, ErrUnauthorized)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 403}, ErrForbidden)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 404}, ErrNotFound)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 429}, ErrRateLimited)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 500}, ErrServerError)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 503}, ErrServerError)` must return true
    - `errors.Is(&amp;APIError{StatusCode: 418}, ErrUnauthorized)` must return FALSE (teapot doesn't bucket)
    - `parseRetryAfter("7")` returns `7 * time.Second`
    - `parseRetryAfter("")` returns `0`
    - `parseRetryAfter("not-a-number")` returns `0`
    - `parseRetryAfter("-5")` returns `0` (negative guard)
    - `APIError{StatusCode: 404, Method: "GET", Endpoint: "/rest/api/3/issue/K-1"}.Error()` returns a string containing `"404"`, `"GET"`, `"/rest/api/3/issue/K-1"`
  </behavior>
  <action>
Create `pkg/jira/errors.go` with EXACTLY this structure (values locked per D-18/D-19/D-20):

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Sentinel errors — callers use errors.Is(err, ErrX) for class checks.
// Per D-18, these are plain errors.New sentinels (no wrapping, no codes).
var (
	// HTTP status bucket sentinels — returned via APIError.Unwrap().
	ErrUnauthorized = errors.New("jira: unauthorized")      // 401
	ErrForbidden    = errors.New("jira: forbidden")         // 403
	ErrNotFound     = errors.New("jira: not found")         // 404
	ErrRateLimited  = errors.New("jira: rate limited")      // 429
	ErrServerError  = errors.New("jira: server error")      // 5xx

	// Config loader sentinels.
	ErrConfigNotFound   = errors.New("jira: config not found")
	ErrConfigInvalid    = errors.New("jira: config invalid")
	ErrConfigIncomplete = errors.New("jira: config incomplete")
)

// APIError is returned for every non-2xx response from the Jira API. Callers
// use errors.Is(err, ErrUnauthorized) for class checks and errors.As(err, &apiErr)
// when they need the status code, endpoint, or response body.
//
// Per D-20, RetryAfter is populated only when StatusCode == 429 AND the server
// sent a parseable Retry-After header.
type APIError struct {
	StatusCode int
	Endpoint   string
	Method     string
	Body       string        // truncated to 1 KB by the client
	RetryAfter time.Duration // populated iff StatusCode == 429 and header parseable
}

// Error implements the error interface. Format is intentionally short and
// debug-oriented — callers that need to display a user-facing message should
// use errors.Is to dispatch on the sentinel.
func (e *APIError) Error() string {
	return fmt.Sprintf("jira: HTTP %d %s %s", e.StatusCode, e.Method, e.Endpoint)
}

// Unwrap returns the sentinel that matches the HTTP status bucket. This lets
// errors.Is(err, ErrUnauthorized) succeed without the caller needing to know
// about APIError at all.
//
// Unrecognized status codes (non-401/403/404/429/5xx, e.g. 418) return nil,
// meaning errors.Is(err, anySentinel) is false for them — callers must fall
// back to errors.As to inspect the struct.
func (e *APIError) Unwrap() error {
	switch {
	case e.StatusCode == 401:
		return ErrUnauthorized
	case e.StatusCode == 403:
		return ErrForbidden
	case e.StatusCode == 404:
		return ErrNotFound
	case e.StatusCode == 429:
		return ErrRateLimited
	case e.StatusCode >= 500 && e.StatusCode < 600:
		return ErrServerError
	default:
		return nil
	}
}

// parseRetryAfter parses an Atlassian Retry-After header value. Per
// https://developer.atlassian.com/cloud/jira/platform/rate-limiting/ the value
// is always an integer number of seconds. Returns 0 if the header is empty,
// unparseable, or negative — Phase 5's rate limiter is responsible for picking
// a default backoff when this returns 0.
func parseRetryAfter(header string) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	secs, err := strconv.Atoi(header)
	if err != nil || secs < 0 {
		return 0
	}
	return time.Duration(secs) * time.Second
}
```

Do NOT export `parseRetryAfter` — it is an internal helper used by client.go.
Do NOT add a `Cause` field or wrap underlying errors — per D-18 sentinels are plain, and APIError.Unwrap returns the bucket sentinel, not an inner error.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go vet ./pkg/jira/... &amp;&amp; go build ./pkg/jira/... 2>&amp;1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/errors.go` returns exit 0
    - `grep -q "^var ($" pkg/jira/errors.go` (block declaration)
    - `grep -q 'ErrUnauthorized = errors.New("jira: unauthorized")' pkg/jira/errors.go`
    - `grep -q 'ErrForbidden    = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrNotFound     = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrRateLimited  = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrServerError  = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrConfigNotFound   = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrConfigInvalid    = errors.New' pkg/jira/errors.go`
    - `grep -q 'ErrConfigIncomplete = errors.New' pkg/jira/errors.go`
    - `grep -q "type APIError struct {" pkg/jira/errors.go`
    - `grep -q "StatusCode int" pkg/jira/errors.go`
    - `grep -q "RetryAfter time.Duration" pkg/jira/errors.go`
    - `grep -q "func (e \*APIError) Unwrap() error" pkg/jira/errors.go`
    - `grep -q "func parseRetryAfter(header string) time.Duration" pkg/jira/errors.go`
    - `go vet ./pkg/jira/...` exits 0
    - `go build ./pkg/jira/...` exits 0
  </acceptance_criteria>
  <done>errors.go compiles; exports 8 sentinels + APIError struct; Unwrap() buckets HTTP statuses 401/403/404/429/5xx; parseRetryAfter handles empty/invalid/negative.</done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Implement pkg/jira/config.go (Config struct + loader with defaults)</name>
  <files>pkg/jira/config.go</files>
  <read_first>
    - pkg/wconfig/settingsconfig.go (entire config-load pattern — the reference per RESEARCH §Pattern 3)
    - pkg/wavebase/wavebase.go (CONFIRM: this plan does NOT use GetWaveConfigDir per D-07 / Pitfall 7)
    - pkg/jira/config_test.go (the contract — all 5 D-22 cases)
    - pkg/jira/errors.go (just created — for ErrConfigNotFound/ErrConfigInvalid/ErrConfigIncomplete)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md D-07, D-08, D-09, D-10, D-11, D-12
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §Pattern 3 Config Loader, §Pitfall 4 filepath.Join, §Pitfall 7 GetWaveConfigDir
  </read_first>
  <behavior>
    - `LoadConfigFromPath(validPath)` with full JSON returns populated Config, no error
    - `LoadConfigFromPath(validPath)` with partial JSON (missing jql + pageSize but required fields present) returns cfg with `cfg.Jql == "assignee = currentUser() ORDER BY updated DESC"` and `cfg.PageSize == 50`
    - `LoadConfigFromPath(nonExistentPath)` returns error where `errors.Is(err, ErrConfigNotFound)` is true
    - `LoadConfigFromPath(malformedJSONFile)` returns error where `errors.Is(err, ErrConfigInvalid)` is true; the underlying json.Unmarshal error is preserved via %w so `errors.Unwrap(errors.Unwrap(err))` eventually finds a json.SyntaxError-like cause
    - `LoadConfigFromPath(file with only cloudId)` returns error where `errors.Is(err, ErrConfigIncomplete)` is true; error message contains the substrings `baseUrl`, `email`, `apiToken`
    - `LoadConfig()` (no args) resolves `os.UserHomeDir() + filepath.Join(.config, waveterm, jira.json)` and delegates to `LoadConfigFromPath`
    - Calling `LoadConfig()` twice does NOT cache — each call re-reads the file (D-12)
  </behavior>
  <action>
Create `pkg/jira/config.go` with the Config struct and loader. Mirror `pkg/wconfig/settingsconfig.go`'s decode-then-fill pattern.

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is the on-disk representation of ~/.config/waveterm/jira.json.
// Per D-07 the path is literal — we do NOT route through
// wavebase.GetWaveConfigDir() because that resolves to a platform-specific
// directory (e.g. %LOCALAPPDATA%/waveterm on Windows) which is NOT the
// contract the existing Jira widget and jira-cache.json already use.
type Config struct {
	BaseUrl  string `json:"baseUrl"`  // required, e.g. "https://kakaovx.atlassian.net"
	CloudId  string `json:"cloudId"`  // required for cache output, e.g. "280eeb13-..."
	Email    string `json:"email"`    // required, Basic auth username
	ApiToken string `json:"apiToken"` // required, Basic auth password
	Jql      string `json:"jql"`      // optional, default = "assignee = currentUser() ORDER BY updated DESC"
	PageSize int    `json:"pageSize"` // optional, default = 50
}

// Default values applied by LoadConfig / LoadConfigFromPath when fields are
// missing or zero. Keep these as exported constants so Phase 2's refresh
// orchestrator can reference the same literals.
const (
	DefaultJQL      = "assignee = currentUser() ORDER BY updated DESC"
	DefaultPageSize = 50
)

// configFileName and configSubdir are the literal path components per D-07.
// They are private constants because LoadConfig is the only public entry
// point that assembles them.
const (
	configFileName = "jira.json"
	configSubdir   = ".config/waveterm" // joined via filepath.Join — safe on Windows
)

// LoadConfig reads ~/.config/waveterm/jira.json, fills defaults, validates
// required fields. Per D-12 this is called on every refresh (no in-process
// cache) so edits to jira.json take effect without a Waveterm restart.
//
// Errors:
//   - ErrConfigNotFound  : file does not exist
//   - ErrConfigInvalid   : file exists but JSON is malformed (wraps json error)
//   - ErrConfigIncomplete: required field (baseUrl/email/apiToken) missing
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("jira: cannot resolve home directory: %w", err)
	}
	// filepath.Join normalizes separators per platform (D-23 Windows-safe).
	path := filepath.Join(home, ".config", "waveterm", configFileName)
	return LoadConfigFromPath(path)
}

// LoadConfigFromPath is the test seam used by config_test.go. It performs all
// the actual work; LoadConfig is a thin wrapper that resolves the home path.
func LoadConfigFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return Config{}, ErrConfigNotFound
		}
		// Other read failures (permission denied, etc.) are treated as
		// "not found" from the caller's perspective — Phase 4's empty-state
		// UX handles both the same way.
		return Config{}, fmt.Errorf("%w: %v", ErrConfigNotFound, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Wrap the json error via %w so tooling (errors.As) can inspect it,
		// while errors.Is(err, ErrConfigInvalid) remains true via the %w chain.
		return Config{}, fmt.Errorf("%w: %w", ErrConfigInvalid, err)
	}

	// Fill defaults (D-03 / D-08). Silent — no warning, no error.
	if cfg.Jql == "" {
		cfg.Jql = DefaultJQL
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = DefaultPageSize
	}

	// Validate required fields (D-11). The error message NAMES each missing
	// field so the user knows exactly what to fix in jira.json.
	var missing []string
	if cfg.BaseUrl == "" {
		missing = append(missing, "baseUrl")
	}
	if cfg.Email == "" {
		missing = append(missing, "email")
	}
	if cfg.ApiToken == "" {
		missing = append(missing, "apiToken")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%w: missing required fields: %s",
			ErrConfigIncomplete, strings.Join(missing, ", "))
	}
	return cfg, nil
}
```

Key points to verify during implementation:
- Use `filepath.Join(home, ".config", "waveterm", configFileName)` — NEVER `home + "/.config/..."`
- Use `errors.Is(err, fs.ErrNotExist)` AND `os.IsNotExist(err)` (both — the former is modern, the latter is historical; covers all stdlib error shapes)
- Use `fmt.Errorf("%w: %w", sentinel, inner)` for the malformed-JSON case — Go 1.20+ supports multi-%w
- Do NOT add a package-level cache variable. Every call must re-read (D-12).
- Do NOT log the config content (contains API token).
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go test ./pkg/jira/... -run "TestLoadConfig" -count=1 -v 2>&amp;1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/config.go` returns exit 0
    - `grep -q "^type Config struct" pkg/jira/config.go`
    - `grep -q 'BaseUrl  string `json:"baseUrl"`' pkg/jira/config.go`
    - `grep -q 'CloudId  string `json:"cloudId"`' pkg/jira/config.go`
    - `grep -q 'Email    string `json:"email"`' pkg/jira/config.go`
    - `grep -q 'ApiToken string `json:"apiToken"`' pkg/jira/config.go`
    - `grep -q 'Jql      string `json:"jql"`' pkg/jira/config.go`
    - `grep -q 'PageSize int    `json:"pageSize"`' pkg/jira/config.go`
    - `grep -q 'DefaultJQL      = "assignee = currentUser() ORDER BY updated DESC"' pkg/jira/config.go`
    - `grep -q "DefaultPageSize = 50" pkg/jira/config.go`
    - `grep -q "^func LoadConfig() (Config, error)" pkg/jira/config.go`
    - `grep -q "^func LoadConfigFromPath(path string) (Config, error)" pkg/jira/config.go`
    - `grep -q "os.UserHomeDir()" pkg/jira/config.go`  (D-07)
    - `grep -q 'filepath.Join(home, ".config", "waveterm"' pkg/jira/config.go`  (D-07 + Windows-safe)
    - `grep -vq '"home.*/\\.config"' pkg/jira/config.go`  (no string-concat POSIX paths)
    - `grep -vq "GetWaveConfigDir" pkg/jira/config.go`  (D-07: must NOT use this)
    - `grep -q "fs.ErrNotExist" pkg/jira/config.go`
    - `grep -q "ErrConfigNotFound" pkg/jira/config.go`
    - `grep -q "ErrConfigInvalid" pkg/jira/config.go`
    - `grep -q "ErrConfigIncomplete" pkg/jira/config.go`
    - `go test ./pkg/jira/... -run "TestLoadConfig" -count=1` exits 0 (all 5 config tests GREEN)
    - `go test ./pkg/jira/... -run "TestLoadConfig" -count=1 -race` exits 0 (race-free; no cache = no shared state)
  </acceptance_criteria>
  <done>All 5 D-22 config tests pass: TestLoadConfig_Happy, TestLoadConfig_DefaultsFill, TestLoadConfig_MissingFile, TestLoadConfig_MalformedJSON, TestLoadConfig_Incomplete. Loader is stateless (re-reads every call).</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| filesystem → Go process | jira.json contents (contains API token) cross from disk into memory |
| Go process → HTTP response formatter | APIError.Body is user-visible (included in Error() indirectly via logging) |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-02-01 | I (Information Disclosure) | APIError.Error() output | mitigate | `Error()` returns `jira: HTTP <code> <method> <endpoint>` — deliberately does NOT include Body or auth headers. Body is accessible only via `errors.As`, for callers that explicitly opt in to reading it (e.g., debug logging). Grep check: `grep "e.Body\|e.ApiToken" pkg/jira/errors.go` must NOT appear inside `Error()`. |
| T-01-02-02 | I (Information Disclosure) | Config loader logging | mitigate | Config loader must NOT log file contents. `grep "log\.\|Printf.*cfg\.\|Printf.*ApiToken" pkg/jira/config.go` returns no matches (no logging of config data). |
| T-01-02-03 | T (Tampering) | Retry-After header | mitigate (per RESEARCH Security Domain) | `strconv.Atoi` returns error on overflow or non-integer; parseRetryAfter guards err != nil and secs < 0; max representable value (~2^31 s = 68 years) is bounded by int overflow which Atoi handles. No client-induced DoS. |
| T-01-02-04 | I (Information Disclosure) | jira.json API token in plaintext | accept (D-08) | User explicitly accepted this risk in CONTEXT.md D-08. JIRA-F-01 defers safeStorage encryption to a future milestone. Documented in README (Phase 4). |
| T-01-02-05 | I (Information Disclosure) | Config file permissions | mitigate | Test helper writes with mode 0o600 (owner read/write only). Production code does NOT create the file — user creates it manually per JIRA-02 — so we rely on the user's umask. No mitigation needed in Phase 1 code. |
</threat_model>

<verification>
After both tasks:
- `go build ./pkg/jira/...` exits 0
- `go vet ./pkg/jira/...` exits 0
- `go test ./pkg/jira/... -run "TestLoadConfig" -count=1 -race` exits 0 — all 5 config tests GREEN
- `pkg/jira/client_test.go` still fails to compile ONLY due to missing Client/SearchIssues/GetIssue (NOT due to missing sentinels or APIError) — confirms errors.go provides the full error surface
- `pkg/jira/adf_test.go` still fails to compile ONLY due to missing ADFToMarkdown — unchanged by this plan
</verification>

<success_criteria>
- `config.go` passes all 5 D-22 config test cases on Windows (t.TempDir + filepath.Join)
- `errors.go` exports 8 sentinels and APIError with Unwrap() returning the right bucket sentinel for 401/403/404/429/5xx, nil otherwise
- No `GetWaveConfigDir` reference in pkg/jira/ (D-07)
- No in-process config cache (D-12 — every call to LoadConfig re-reads)
- JIRA-02 is now fully implementable end-to-end (config file → Config struct → typed errors)
</success_criteria>

<output>
After completion, create `.planning/phases/01-jira-http-client-config/01-02-SUMMARY.md` recording:
- Test results (`go test ./pkg/jira/... -run TestLoadConfig -count=1 -race` output)
- Sentinel error names and their statuses
- Confirmation that Plan 03 (client.go) has a complete Config struct and APIError type to build on
</output>

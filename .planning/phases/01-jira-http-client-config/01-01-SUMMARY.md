---
phase: 01-jira-http-client-config
plan: 01
subsystem: pkg/jira
tags: [scaffold, tests, nyquist-red, go]
requires: []
provides:
  - package: pkg/jira
  - tests: [config, client, adf]
affects: []
tech_stack:
  added: []
  patterns: [httptest.NewServer, table-driven tests, t.TempDir + filepath.Join, errors.Is/As sentinel+struct pattern]
key_files:
  created:
    - pkg/jira/doc.go
    - pkg/jira/config_test.go
    - pkg/jira/client_test.go
    - pkg/jira/adf_test.go
  modified: []
decisions:
  - "Stubs reference undefined symbols on purpose — establishes Nyquist RED so Plans 01-02/03/04 have a concrete failing target to turn GREEN"
  - "White-box `package jira` (not `jira_test`) so future internal helpers remain testable without exporting them"
  - "Local `stringsContains` helper in config_test.go keeps the stub dep-free; implementation in Plan 01-02 may swap for strings.Contains"
metrics:
  duration: "~5 min (scaffold)"
  completed: "2026-04-15"
  test_functions: 15
  total_lines: 471
---

# Phase 01 Plan 01: Scaffold and Stubs Summary

Created the `pkg/jira/` Go package with `doc.go` and three Nyquist-compliant failing test files (`config_test.go`, `client_test.go`, `adf_test.go`) so Plans 01-02/03/04 have concrete failing targets to turn GREEN.

## Files Created

| File | Lines | Purpose |
| ---- | ----: | ------- |
| `pkg/jira/doc.go` | 11 | Package declaration + copyright/SPDX header (matches pkg/ convention); empty package compiles |
| `pkg/jira/config_test.go` | 117 | 5 test functions covering D-22 config coverage (Happy, DefaultsFill, MissingFile, MalformedJSON, Incomplete). Uses `t.TempDir()` + `filepath.Join` (D-23 Windows-safe). |
| `pkg/jira/client_test.go` | 247 | 6 tests covering JIRA-01 matrix (auth header, pagination, request body shape, GetIssue fields CSV, nil-fields omit, 401/403/404/429/5xx error paths with APIError + Retry-After). Uses `httptest.NewServer` (D-21). |
| `pkg/jira/adf_test.go` | 96 | 4 tests covering D-14 node set + D-15 unknown-node case (empty, malformed, per-node table-driven with 15 node cases, mixed unknown). |
| **Total** | **471** | |

## Commits

| Task | Commit | Message |
| ---- | ------ | ------- |
| 1 | `6c926b29` | `feat(01-01): add pkg/jira package with doc.go` |
| 2 | `da81efaa` | `test(01-01): add failing config loader test stubs (Nyquist RED)` |
| 3 | `bc5fbb8e` | `test(01-01): add failing client + ADF converter stubs (Nyquist RED)` |

## Confirmed RED State

`go build ./pkg/jira/...` → **succeeds** (only `doc.go` contributes production code, which is empty)
`go test ./pkg/jira/... -count=1` → **fails with undefined-symbol errors** (expected Nyquist RED):

```
# github.com/wavetermdev/waveterm/pkg/jira [github.com/wavetermdev/waveterm/pkg/jira.test]
pkg\jira\client_test.go:21:57: undefined: Client
pkg\jira\client_test.go:23:9:  undefined: Config
pkg\jira\client_test.go:31:9:  undefined: NewClientWithHTTP
pkg\jira\client_test.go:48:49: undefined: SearchOpts
pkg\jira\client_test.go:86:53: undefined: SearchOpts
pkg\jira\client_test.go:100:53: undefined: SearchOpts
pkg\jira\adf_test.go:15:15:    undefined: ADFToMarkdown
pkg\jira\adf_test.go:26:12:    undefined: ADFToMarkdown
pkg\jira\adf_test.go:60:16:    undefined: ADFToMarkdown
pkg\jira\adf_test.go:83:14:    undefined: ADFToMarkdown
pkg\jira\client_test.go:100:53: too many errors
FAIL    github.com/wavetermdev/waveterm/pkg/jira [build failed]
```

This is the intended state. Symbols will be introduced by:

- **Plan 01-02** — `Config`, `LoadConfigFromPath`, `ErrConfigNotFound/Invalid/Incomplete`, `APIError`, `ErrUnauthorized/Forbidden/NotFound/RateLimited/ServerError` (turns config_test.go + error-path portion of client_test.go GREEN)
- **Plan 01-03** — `ADFToMarkdown` (turns adf_test.go GREEN)
- **Plan 01-04** — `Client`, `NewClient`, `NewClientWithHTTP`, `SearchOpts`, `SearchResult`, `GetIssueOpts`, `Issue`, `SearchIssues`, `GetIssue` (turns remaining client_test.go tests GREEN)

## Test Name → D-XX Coverage Mapping

### Config (D-22 Config Cases)

| Test | D-22 Case | Context Decision |
| ---- | --------- | ---------------- |
| `TestLoadConfig_Happy` | Happy path | D-07 path, D-08 fields |
| `TestLoadConfig_DefaultsFill` | Partial (defaults fill) | D-03, D-08, D-11 |
| `TestLoadConfig_MissingFile` | Missing file | D-09 `ErrConfigNotFound` |
| `TestLoadConfig_MalformedJSON` | Malformed JSON | D-10 `ErrConfigInvalid` |
| `TestLoadConfig_Incomplete` | Required missing | D-11 `ErrConfigIncomplete` names fields |

### Client (D-22 HTTP Coverage)

| Test | D-22 Case | Context Decision |
| ---- | --------- | ---------------- |
| `TestAuthHeader_ExactBasicBase64` | Authorization header correctness | D-06 Basic `email:token` |
| `TestSearchIssues_Pagination` | SearchIssues 200 + pagination | D-01 POST /rest/api/3/search/jql, cursor via nextPageToken |
| `TestSearchIssues_RequestBodyShape` | (body shape gate) | D-01 token in body, D-03 MaxResults default 50 |
| `TestGetIssue_FieldsAsCSV` | GetIssue 200 with fields | D-02 GET /rest/api/3/issue/{key}?fields=... |
| `TestGetIssue_NilFieldsOmitsQueryParam` | (nil fields gate) | D-02 caller-provided fields |
| `TestErrorPaths` (6 subtests) | 401/403/404/429/5xx paths | D-18/D-19 sentinel+struct, D-20 Retry-After |

### ADF (D-14 Node Set + D-15 Unknown Handling)

| Test | Coverage | Context Decision |
| ---- | -------- | ---------------- |
| `TestADFToMarkdown_EmptyInput` | Empty + `null` input → empty string | D-16 entry point contract |
| `TestADFToMarkdown_MalformedJSON` | Parse failure surfaces error | D-16 only JSON parse errors return err |
| `TestADFToMarkdown_PerNodeType` | 15 subtests: paragraph, heading h2, bulletList, orderedList, codeBlock, blockquote, rule, hardBreak, marks (strong/em/code/link), mention (with text + id-fallback), table with header row | D-14 supported node set, D-13 markdown output |
| `TestADFToMarkdown_UnknownNodeInMixedTree` | Unknown `panel` node; surrounding text preserved | D-15 silent skip, partial content preferred |

## Deviations from Plan

None — plan executed exactly as written.

## Threat Model

### T-01-00-01 (Information Disclosure — test fixture strings)

**Disposition:** accept (per plan)
**Check:** `grep -E "kakaovx|ATATT-[A-Z0-9]{10,}" pkg/jira/*_test.go`
**Result:** Only `ATATT-xxx` (fake placeholder) and the kakaovx base URL + spike@kakaovx.com email appear. The email + base URL are explicit test fixtures prescribed by the plan (and are public anyway — the API token is what matters). No real ATATT- token (would match `ATATT-[A-Z0-9]{10,}`) is present. Threat mitigated.

No new threat surface introduced — this plan only creates Go test files that run against `httptest.NewServer`; no network I/O, no file system writes outside `t.TempDir()`, no init() side effects.

## Verification Checklist (from plan `<verification>` + `<success_criteria>`)

- [x] `ls pkg/jira/` shows `doc.go`, `client_test.go`, `config_test.go`, `adf_test.go`
- [x] `go build ./pkg/jira/...` succeeds
- [x] `go test ./pkg/jira/... -count=1` fails with undefined-symbol errors (Nyquist RED)
- [x] Every D-22 coverage case maps to a stub (see mapping table above)
- [x] Zero production code beyond `doc.go` (no config.go / errors.go / client.go / adf.go)
- [x] Windows-safe: all temp paths via `t.TempDir()` + `filepath.Join`; no hardcoded `/` separators
- [x] T-01-00-01 threat mitigation verified (no real credentials)

## Self-Check: PASSED

- FOUND: pkg/jira/doc.go
- FOUND: pkg/jira/config_test.go
- FOUND: pkg/jira/client_test.go
- FOUND: pkg/jira/adf_test.go
- FOUND commit: 6c926b29
- FOUND commit: da81efaa
- FOUND commit: bc5fbb8e

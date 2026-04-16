---
phase: 02-cache-orchestration
plan: 01
subsystem: jira-cache
tags: [jira, go, tdd, nyquist-red, httptest, cache-schema]

requires:
  - phase: 01-jira-http-client-config
    provides: LoadConfig, NewClient, NewClientWithHTTP, SearchIssues, GetIssue, ADFToMarkdown, *APIError
provides:
  - Extended IssueFields.Status with nested StatusCategory.Key (D-CACHE-08 fix)
  - Myself struct + Client.GetMyself method (D-FLOW-03)
  - JiraCache / JiraCacheIssue / JiraCacheAttachment / JiraCacheComment cache-shape types
  - Nyquist RED scaffolding — 9 TestRefresh_* functions + 6 testdata fixtures
  - Golden-file comparison pattern with <<REPLACED-AT-TEST>> + fetchedAt normalization
affects: [02-02-refresh-orchestration-green, 03-wsh-rpc, 04-widget, 05-attachments]

tech-stack:
  added: []
  patterns:
    - "Cache-shape vs wire-shape type separation (RESEARCH Anti-Patterns)"
    - "Nyquist RED: compile-failing tests precede implementation"
    - "httptest.NewServer with path-multiplexed handler covering /myself + /search/jql + /issue/{key}"
    - "Golden-file comparison with regex-normalized time fields and URL placeholder substitution"
    - "HTTP client injection via RefreshOpts.HTTPClient (Recommendation 1 adopted)"

key-files:
  created:
    - pkg/jira/cache_types.go
    - pkg/jira/refresh_test.go
    - pkg/jira/testdata/cache.golden.json
    - pkg/jira/testdata/cache-with-localpath.seed.json
    - pkg/jira/testdata/issue-itsm1.json
    - pkg/jira/testdata/issue-comment-cap.json
    - pkg/jira/testdata/issue-comment-truncation.json
    - pkg/jira/testdata/issue-null-description.json
  modified:
    - pkg/jira/client.go

key-decisions:
  - "Adopted RESEARCH Recommendation 1: inject *http.Client through RefreshOpts.HTTPClient; Refresh uses NewClientWithHTTP when non-nil"
  - "Additive-only client.go edits — zero Phase 1 behavior or test changes"
  - "Cache-shape types (JiraCache*) kept in cache_types.go, distinct from wire-shape types in client.go"
  - "Merged truncation coverage into issue-comment-cap.json (c06 carries the 2500-char body) AND kept issue-comment-truncation.json as a standalone single-issue fixture"
  - "Test 6 builds statusCategory fixtures inline as Go string literals — one disk fixture per mapping would be 5 near-duplicate files"

patterns-established:
  - "Nyquist RED: test file + fixtures shipped before the production code; compile failure (`undefined: Refresh`) is the intended signal"
  - "setFakeHome(t) helper that sets both HOME and USERPROFILE to t.TempDir() so os.UserHomeDir() is sandboxed on POSIX and Windows"
  - "fetchedAtRe regex-based timestamp normalization before byte-identical golden-file compare"

requirements-completed: [JIRA-06, JIRA-07]

duration: ~90min (across two sessions)
completed: 2026-04-15
---

# Phase 02 Plan 01: Client Extensions and TDD RED Summary

**Extended Phase 1 client with Status.StatusCategory + GetMyself, added cache-shape type family in cache_types.go, and authored 9 failing TestRefresh_* tests backed by 6 testdata fixtures — the complete Nyquist RED scaffolding for Plan 02's refresh orchestrator.**

## Performance

- **Duration:** ~90 min (two sessions; prior session wrote Tasks 1+2 before quota interruption)
- **Completed:** 2026-04-15
- **Tasks:** 3
- **Files modified:** 9 (1 edited, 8 created)

## Accomplishments

- **Task 1** (prior session, commit `99e6838c`): Extended `IssueFields.Status` with nested `StatusCategory.Key`, added `Myself` struct, added `Client.GetMyself(ctx)` — all additive, no Phase 1 test touched.
- **Task 2** (prior session, commit `98d7dd16`): Created `pkg/jira/cache_types.go` with the widget-authoritative on-disk schema — `JiraCache`, `JiraCacheIssue`, `JiraCacheAttachment`, `JiraCacheComment`, including `Truncated bool` with `json:",omitempty"` for the D-TEST-01 golden-file guarantee.
- **Task 3** (this session, commit `31587cad`): Wrote 9 `TestRefresh_*` functions and 6 testdata fixtures that fail at compile time with `undefined: Refresh / RefreshOpts / RefreshReport` — the Nyquist RED signal Plan 02-02 will turn GREEN.

## Task Commits

1. **Task 1: Extend client.go with Status.StatusCategory + Myself + GetMyself** — `99e6838c` (feat) — *prior session*
2. **Task 2: Create cache_types.go with the widget-authoritative on-disk schema** — `98d7dd16` (feat) — *prior session*
3. **Task 3: Create refresh_test.go + fixture JSONs + golden file (Nyquist RED)** — `31587cad` (test) — *this session*

## Files Created/Modified

- `pkg/jira/client.go` — +20 LoC additive (StatusCategory struct extension, Myself type, GetMyself method). No removed or edited Phase 1 lines.
- `pkg/jira/cache_types.go` — 82 LoC. Pure type definitions; no methods, no constructors. Deliberate separation from wire-shape types in client.go.
- `pkg/jira/refresh_test.go` — ~560 LoC. 9 TestRefresh_* functions + 4 shared helpers (setFakeHome, readFixture, newRefreshTestServer, baseConfig) + regex normalizer.
- `pkg/jira/testdata/issue-itsm1.json` — canonical single-issue fixture (RESEARCH Example 2).
- `pkg/jira/testdata/issue-comment-cap.json` — 15 comments; c06 carries a 2500-char body so after the last-10 cap c06 is the first kept (Pitfall 2 + D-CACHE-06).
- `pkg/jira/testdata/issue-comment-truncation.json` — single-issue fixture isolating one 2500-char comment; kept alongside cap fixture for diagnostic clarity.
- `pkg/jira/testdata/issue-null-description.json` — null description, no comments, done-category — covers D-CACHE-04.
- `pkg/jira/testdata/cache-with-localpath.seed.json` — pre-existing cache with `localPath="C:/Users/dev/downloaded/att1-screenshot.png"` for D-FLOW-04 preserve test.
- `pkg/jira/testdata/cache.golden.json` — byte-exact expected output for TestRefresh_GoldenFile, with `<<REPLACED-AT-TEST>>` URL placeholder and `"fetchedAt": "NORMALIZED"` literal.

## Test Matrix (Task 3)

| Test                                      | D-decision(s) covered                      | Req    |
|-------------------------------------------|--------------------------------------------|--------|
| `TestRefresh_GoldenFile`                  | D-TEST-01, D-CACHE-02/03/04/05/06/07/08    | JIRA-07 |
| `TestRefresh_PreserveLocalPath`           | D-TEST-02, D-FLOW-04                       | JIRA-07 |
| `TestRefresh_CommentCapAndTruncation`     | D-CACHE-06, D-CACHE-07, D-TEST-03, Pitfall 2 | JIRA-06 |
| `TestRefresh_LastCommentAt`               | D-CACHE-07                                 | JIRA-06 |
| `TestRefresh_NullDescription`             | D-CACHE-04                                 | JIRA-07 |
| `TestRefresh_StatusCategoryMapping`       | D-CACHE-08, Pitfall 4                      | JIRA-07 |
| `TestRefresh_ErrorClassification`         | D-ERR-01, D-ERR-02, D-ERR-03               | JIRA-07 |
| `TestRefresh_ProgressCallback`            | D-PROG-01, Pitfall 6                       | JIRA-07 |
| `TestRefresh_AttachmentWebUrlPassthrough` | D-CACHE-05, RESEARCH A3                    | JIRA-07 |

## Decisions Made

- **HTTP injection via RefreshOpts.HTTPClient** — adopted RESEARCH Recommendation 1 over the package-level-var alternative. Minimal surface change, test-friendly, and Phase 5 will similarly want to inject a rate-limiting Transport.
- **Kept `issue-comment-truncation.json` as a separate file** even though the plan permits merging it into `issue-comment-cap.json`. Rationale: diagnostic clarity when a truncation bug ships — the single-comment fixture isolates the truncation rule without the 15-comment cap logic interfering. Both fixtures are tiny (<4KB combined).
- **Test 6 uses inline Go string literals for statusCategory fixtures** instead of five near-duplicate JSON files under testdata/. The variants differ only in one nested field; a table-driven test with `fmt.Sprintf` is faster to read and maintain.

## Deviations from Plan

### Documentation-level: Phase 1 tests unrunnable while RED holds

- **Found during:** Task 3 verification.
- **Issue:** Plan success_criteria bullet 5 expects `go test ./pkg/jira/ -run 'TestLoadConfig|TestADF|TestAuth|TestSearch|TestGetIssue|TestErrorPaths' -count=1` to **still PASS** after RED is established. Since `refresh_test.go` is in the same `package jira` test binary, Go compiles all test files together; the `undefined: Refresh` compile failure prevents *any* filtered test from executing. This is a logical contradiction inside the plan itself — Step D explicitly states compile failure is intentional, yet Criterion 5 asks for passing tests.
- **Decision:** Follow the plan's DESIGN intent (Step D, Nyquist RED) rather than the impossible criterion. Phase 1 tests were last verified passing on the pre-Task-3 state (commit `98d7dd16`, confirmed in this session before refresh_test.go was added). Plan 02-02 will restore test-binary compilability, at which point the Phase 1 filter regains runnability.
- **Classification:** Rule 4 territory (plan defect), but the resolution is trivially mechanical (document and continue) so not escalated as a checkpoint.
- **Files modified:** none (documentation-only deviation).
- **Committed in:** recorded here only.

### No auto-fixes otherwise

Tasks 1 and 2 executed per plan (prior session). Task 3 executed per plan with the three planner-discretion decisions listed above; all are within the plan's permitted discretion surface.

---

**Total deviations:** 1 documentation-level (plan-internal contradiction, unresolvable without modifying the plan).
**Impact on plan:** Zero. Plan 02-02 is unblocked — it receives exactly the scaffolding it expects.

## Issues Encountered

- **Python default unavailable; used `py` launcher.** The Windows environment has `python` / `python3` shims that return exit 49 under non-interactive invocation; falling back to `py` (Python 3.14.2) succeeded. Affected fixture generation for `issue-comment-cap.json` (2500-char body) and the null-description fixture.
- **Fixture generation wrote to the wrong working directory.** First invocation resolved `testdata/` against the main repo (`/f/Waveterm/waveterm/pkg/jira/testdata/`) rather than the worktree (`/f/Waveterm/waveterm/.claude/worktrees/agent-a5a9a27a/pkg/jira/testdata/`). Resolved by `cp` + cleanup; no lost work.

## User Setup Required

None.

## Next Phase Readiness

- Plan 02-02 can begin immediately. It implements `pkg/jira/refresh.go` (Refresh, RefreshOpts, RefreshReport). Success criterion: `go test ./pkg/jira -run TestRefresh -count=1` passes AND `go test ./pkg/jira/ -run 'TestAuthHeader|TestSearchIssues|TestGetIssue|TestLoadConfig|TestADF' -count=1` regains passing state.
- Every D-decision (D-CACHE, D-FLOW, D-PROG, D-ERR) is asserted by at least one test — Plan 02-02 can develop against the test suite without re-reading CONTEXT/RESEARCH except for cross-reference.

---
*Phase: 02-cache-orchestration*
*Plan: 01 (client-extensions-and-tdd-red)*
*Completed: 2026-04-15*

## Self-Check: PASSED

Verification performed at summary time:

- `pkg/jira/client.go` exists, contains `StatusCategory struct` (1 match) and `func (c *Client) GetMyself` (1 match).
- `pkg/jira/cache_types.go` exists (82 lines).
- `pkg/jira/refresh_test.go` exists, declares 9 `TestRefresh_*` functions; contains zero `%+v` occurrences.
- All 6 testdata fixtures exist under `pkg/jira/testdata/`.
- `go build ./pkg/jira/...` — PASSES (production build clean).
- `go test ./pkg/jira/ -run TestRefresh -count=1` — compile fails with `undefined: Refresh`, `undefined: RefreshOpts`, `undefined: RefreshReport` (intended RED signal).
- Commit `99e6838c` (Task 1) — FOUND in git log.
- Commit `98d7dd16` (Task 2) — FOUND in git log.
- Commit `31587cad` (Task 3) — FOUND in git log.

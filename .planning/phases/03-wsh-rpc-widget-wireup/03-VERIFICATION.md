---
phase: 03-wsh-rpc-widget-wireup
verified: 2026-04-15T00:00:00Z
status: human_needed
score: 15/15 must-haves verified
overrides_applied: 0
human_verification:
  - test: "End-to-end widget refresh (ROADMAP Success #2)"
    expected: "Clicking the ☁️ (cloud-arrow-down) button in the Jira Tasks widget triggers the backend RPC; spinner shows; within ~3s the issue list refreshes and a '<N> 이슈 · <X>s' summary appears briefly then fades after ~5s; tooltip reads 'Jira에서 새로고침' (no 'Claude에게 요청' suffix)."
    why_human: "Requires running Electron dev app + valid ~/.config/waveterm/jira.json; visual/UX behavior (spinner, summary fade-out, tooltip) cannot be verified by grep or tests."
  - test: "CLI path E2E (ROADMAP Success #1)"
    expected: "`wsh jira refresh` invoked from a terminal with valid ~/.config/waveterm/jira.json exits 0 and prints a single line 'Fetched N issues (X attachments, Y comments) in Z.Zs → <cache path>'. The cache file mtime is updated."
    why_human: "Requires a running wavesrv + live Jira credentials. Unit tests cover the cobra plumbing and pure helpers only — live RPC path is D-TEST-02 deferred."
  - test: "No Claude subprocess spawned (ROADMAP Success #3)"
    expected: "Before clicking ☁️, note the list of terminal blocks in the workspace. Click ☁️. Verify no new terminal block is created and no `claude` process spawns (check Task Manager or `ps`)."
    why_human: "Process-spawn observation requires a running app. Static grep shows no `createBlock`/`ControllerInputCommand` inside requestJiraRefresh body, which is a strong proxy but not proof under runtime."
  - test: "Error UX exact-string match (ROADMAP Success #4)"
    expected: "With ~/.config/waveterm/jira.json renamed, clicking ☁️ surfaces '설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요.' verbatim in errorAtom banner. With apiToken replaced by 'BADTOKEN', clicking ☁️ surfaces '인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)' verbatim."
    why_human: "Exact-string widget banner rendering against live Jira 401 response. Unit tests verify the Go-side mapping; the frontend renders err.message verbatim via {error} JSX but actual DOM output needs manual eyes."
---

# Phase 3: wsh RPC + Widget Wire-up Verification Report

**Phase Goal:** Expose the refresh operation as a wsh command and wire the widget's ☁️ button to call it directly instead of spawning a Claude terminal.
**Verified:** 2026-04-15
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | WshRpcInterface has JiraRefreshCommand method declared | VERIFIED | `pkg/wshrpc/wshrpctypes.go:87` — `JiraRefreshCommand(ctx context.Context, data CommandJiraRefreshData) (CommandJiraRefreshRtnData, error)` |
| 2 | WshServer.JiraRefreshCommand handler calls jira.LoadConfig + jira.Refresh and returns a populated CommandJiraRefreshRtnData on success | VERIFIED | `pkg/wshrpc/wshserver/wshserver-jira.go:55-76` — calls `jiraLoadConfig()` then `jiraRefresh(ctx, jira.RefreshOpts{Config: cfg})`; returns all 6 fields (IssueCount, AttachmentCount, CommentCount, ElapsedMs, CachePath, FetchedAt=RFC3339); handler unit test success subtest passes |
| 3 | Handler maps jira.ErrUnauthorized to the exact Korean PAT message | VERIFIED | `wshserver-jira.go:90-91` — `errors.Is(err, jira.ErrUnauthorized)` → `"인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)"`; test `TestJiraRefreshCommand/unauthorized` passes |
| 4 | Handler maps jira.ErrConfigNotFound to the exact Korean setup message | VERIFIED | `wshserver-jira.go:88-89` — `errors.Is(err, jira.ErrConfigNotFound), errors.Is(err, jira.ErrConfigIncomplete)` → `"설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."`; test `TestJiraRefreshCommand/config_not_found` + `/config_incomplete` pass |
| 5 | Handler never logs APIError.Body or Config.ApiToken (T-01-02) | VERIFIED | No `log.` call anywhere in `wshserver-jira.go`; `sanitizeErrMessage` regex `[A-Za-z0-9_=+/\-]{20,}` redacts token-shaped substrings; test plants `SECRET_TOKEN_12345` and asserts it never appears in any returned error/value |
| 6 | Regenerated wshclientapi.ts exposes RpcApi.JiraRefreshCommand | VERIFIED | `frontend/app/store/wshclientapi.ts:514` — `JiraRefreshCommand(client: WshClient, data: CommandJiraRefreshData, opts?: RpcOpts): Promise<CommandJiraRefreshRtnData>` |
| 7 | Regenerated gotypes.d.ts exposes CommandJiraRefreshData and CommandJiraRefreshRtnData | VERIFIED | `frontend/types/gotypes.d.ts:422` — `type CommandJiraRefreshData = object;`; lines 425-432 — `CommandJiraRefreshRtnData` with 6 fields (issuecount, attachmentcount, commentcount, elapsedms, cachepath, fetchedat) matching Go json tags |
| 8 | `wsh jira --help` prints usage listing `refresh` subcommand | VERIFIED | `TestJiraCmdHelp` passes; `jiraCmd.AddCommand(jiraRefreshCmd)` at `wshcmd-jira.go:48`; refresh subcommand's `Short` = "Fetch latest Jira issues into the cache" |
| 9 | `wsh jira refresh --help` prints flag help including `--json` and `--timeout` | VERIFIED | `TestJiraRefreshHelp` passes; flag registration at `wshcmd-jira.go:49-50` |
| 10 | `wsh jira refresh` invokes RpcApi JiraRefreshCommand and on success prints human-readable summary / JSON | VERIFIED | `wshcmd-jira.go:70` — `wshclient.JiraRefreshCommand(RpcClient, wshrpc.CommandJiraRefreshData{}, opts)`; summary formatted via `formatRefreshSummary` (lines 112-117); `TestFormatRefreshSummary` passes |
| 11 | Exit codes 0/1/2/3 per D-ERR-04; stderr for errors, stdout for success; sendActivity("jira-refresh", …) called | VERIFIED | `exitCodeForError` at `wshcmd-jira.go:94-107` — nil→0, `"인증 실패"` prefix→1, `"설정 파일이 없습니다"` prefix→2, default→3; `fmt.Fprintln(os.Stderr, err.Error())` at line 72; `sendActivity("jira-refresh", rtnErr == nil && WshExitCode == 0)` at line 58 in defer; `TestJiraRefreshExitCodeMapping` passes; `WshExitCode = exitCodeForError(err)` follows repo convention (wshcmd-getvar.go pattern) |
| 12 | Clicking ☁️ calls RpcApi.JiraRefreshCommand — NOT createBlock/ControllerInputCommand with claude "…" | VERIFIED | `jiratasks.tsx:383-417` body contains `RpcApi.JiraRefreshCommand(TabRpcClient, {})` at line 392; no `createBlock`/`ControllerInputCommand` inside requestJiraRefresh; grep confirms `createBlock`/`ControllerInputCommand` only in analyze flow (lines 591, 626, 647, 651, 660) |
| 13 | loadingAtom wraps RPC call; success → loadFromCache + refreshProgressAtom summary with 5s clear; error → errorAtom raw message | VERIFIED | `jiratasks.tsx:389-416` — sets loadingAtom true, clears errorAtom, awaits RPC, on success calls `loadFromCache()` and sets `refreshProgressAtom` to `"{N} 이슈 · {X}s"`; setTimeout 5000ms clears with stale-value guard; catch stores `err.message` in errorAtom; finally clears loadingAtom |
| 14 | Widget tooltip no longer says "Claude에게 요청"; no claude subprocess path remains in refresh | VERIFIED | `jiratasks.tsx:370` — `title: "Jira에서 새로고침"` (no Claude suffix); `grep "Claude에게 요청" jiratasks.tsx` → 0 matches; auto-refresh select tooltip also updated (Rule-1 auto-fix) |
| 15 | refreshProgressAtom declared + rendered | VERIFIED | Declared at line 232; rendered at line 1088-1093 via `useAtomValue(model.refreshProgressAtom)` in conditional `<div className="jira-refresh-summary">` |

**Score:** 15/15 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `pkg/wshrpc/wshrpctypes.go` | JiraRefreshCommand interface + Command*Data types | VERIFIED | Interface at line 87; types at 543-560 |
| `pkg/wshrpc/wshserver/wshserver-jira.go` | Handler + mapJiraError | VERIFIED | 142 LOC; handler + mapJiraError + isNetworkError + sanitizeErrMessage all present |
| `pkg/wshrpc/wshserver/wshserver-jira_test.go` | TestJiraRefreshCommand covering success + error taxonomy | VERIFIED | 7 subtests pass (success, config_not_found, config_incomplete, unauthorized, rate_limited, network, generic); SECRET_TOKEN_12345 leak check included |
| `cmd/wsh/cmd/wshcmd-jira.go` | jira parent + refresh subcommand wired to RpcApi | VERIFIED | Registered via `rootCmd.AddCommand(jiraCmd)` + `jiraCmd.AddCommand(jiraRefreshCmd)`; flags `--json`, `--timeout` declared |
| `cmd/wsh/cmd/wshcmd-jira_test.go` | help/exit-code/format tests | VERIFIED | 5 tests pass: TestJiraCmdHelp, TestJiraRefreshHelp, TestJiraRefreshExitCodeMapping, TestJiraRefreshExitCodeNoTokenLeak, TestFormatRefreshSummary |
| `pkg/wshrpc/wshclient/wshclient.go` | generated client binding | VERIFIED | Line 511-513 — `JiraRefreshCommand(w, data, opts)` via `sendRpcRequestCallHelper` with route "jirarefresh" |
| `frontend/app/store/wshclientapi.ts` | generated RpcApi.JiraRefreshCommand | VERIFIED | Line 514 |
| `frontend/types/gotypes.d.ts` | generated TS types | VERIFIED | Lines 421-432; 6 fields match Go tags |
| `frontend/app/view/jiratasks/jiratasks.tsx` | RPC-driven requestJiraRefresh + refreshProgressAtom + updated tooltip | VERIFIED | atom line 232; tooltip line 370; RPC call line 392; render line 1088-1093 |

### Key Link Verification

| From | To | Via | Status | Details |
|------|----|----|--------|---------|
| `wshserver-jira.go` | `pkg/jira.Refresh` | direct call after LoadConfig | WIRED | `jiraRefresh(ctx, jira.RefreshOpts{Config: cfg})` at line 64 |
| `wshserver-jira.go` | `pkg/jira` error sentinels | `errors.Is` on ErrUnauthorized/ErrConfigNotFound/ErrConfigIncomplete/ErrRateLimited | WIRED | `mapJiraError` lines 86-99 dispatches on all four sentinels plus isNetworkError + default |
| `wshclientapi.ts` | `wshrpctypes.go` | cmd/generatets output | WIRED | Line 514 present; types regenerated in gotypes.d.ts |
| `wshcmd-jira.go` | `wshclient.JiraRefreshCommand` | direct call | WIRED | Line 70 |
| `wshcmd-jira.go` | `rootCmd` in wshcmd-root.go | `rootCmd.AddCommand(jiraCmd)` in init() | WIRED | Line 47 |
| `jiratasks.tsx` requestJiraRefresh | `RpcApi.JiraRefreshCommand` | `import { RpcApi }` | WIRED | Line 392 — `await RpcApi.JiraRefreshCommand(TabRpcClient, {})` |
| `jiratasks.tsx` requestJiraRefresh | `this.loadFromCache` | `await this.loadFromCache()` after RPC success | WIRED | Line 394 |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
|----------|---------------|--------|--------------------|--------|
| widget refreshProgressAtom render | `refreshProgress` | `refreshProgressAtom` set to `"${N} 이슈 · ${X}s"` from RPC return values | Yes — derived from CommandJiraRefreshRtnData fields (issuecount, elapsedms) | FLOWING |
| widget issuesAtom (via loadFromCache) | issuesAtom | `loadFromCache()` reads jira-cache.json file | Yes — real cache file, unchanged by Phase 3 | FLOWING |
| widget errorAtom | error | set from `err.message` in catch; RPC error strings come from handler's `mapJiraError` | Yes — Korean human-actionable strings | FLOWING |
| handler return | CommandJiraRefreshRtnData | `jiraRefresh(ctx, RefreshOpts{Config: cfg})` → RefreshReport | Yes — real pkg/jira.Refresh (Phase 2) | FLOWING |
| CLI summary output | `formatRefreshSummary(rtn)` | RPC return data | Yes — direct passthrough from server | FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| Module builds clean | `go build ./...` | exit 0 (no output) | PASS |
| vet clean across phase surface | `go vet ./pkg/wshrpc/... ./pkg/jira/... ./cmd/wsh/...` | exit 0 | PASS |
| Unit tests pass | `go test ./pkg/jira/ ./pkg/wshrpc/wshserver/ ./cmd/wsh/cmd/ -count=1` | ok (3 packages) | PASS |
| TypeScript typecheck | `npx tsc --noEmit` | exit 0 (no output) | PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
|-------------|-------------|-------------|--------|----------|
| JIRA-03 | 03-01, 03-03 | Widget ☁️ button triggers Waveterm backend refresh; no AI CLI involved; progress surfaced | SATISFIED (automated) / NEEDS HUMAN (E2E UX) | RPC handler + widget rewrite verified statically and by Go unit tests. E2E click-the-button observation is in human verification. |
| JIRA-08 | 03-01, 03-02 | `wsh jira refresh` from any terminal triggers same flow as widget | SATISFIED (automated) / NEEDS HUMAN (live refresh) | CLI subcommand, exit codes, flag plumbing, route selection all verified. Live refresh against running wavesrv is human verification. |

Both phase-declared requirements (JIRA-03, JIRA-08) appear in plan frontmatter across 03-01, 03-02, 03-03. No orphaned requirements — REQUIREMENTS.md maps Phase 3 exclusively to JIRA-03 and JIRA-08.

### Anti-Patterns Found

None blocking.

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| `jiratasks.tsx` | 591, 626, 647, 651, 660 | `createBlock` / `ControllerInputCommand` | Info | Intentional — these are the analyze-issue flow (separate feature, not refresh). REVIEW IN-04 confirms this is preserved by design. |
| `wshserver-jira.go` | 124 | `strings.Contains(msg, "dial tcp")` network heuristic | Info | Defense-in-depth fallback; Go stdlib errors don't currently localize. Documented in REVIEW IN-02; out of scope for fix. |

Scanned phase-modified files for: TODO/FIXME/placeholder comments, empty returns, hardcoded empty props, console.log-only implementations — none found in the phase edit surface. `return null` / `=> {}` occurrences in jiratasks.tsx are pre-existing and outside this phase's edit scope.

### Human Verification Required

See frontmatter `human_verification:` block. The four ROADMAP Phase 3 Success Criteria that require live runtime:

1. **CLI path E2E (Success #1):** `wsh jira refresh` exits 0 and writes cache with valid jira.json + live Jira.
2. **Widget RPC trigger + spinner + summary fade (Success #2):** Click ☁️, observe spinner, issue list update, summary badge.
3. **No Claude subprocess (Success #3):** Confirm no new terminal block or `claude` process on ☁️ click (grep already confirms code is absent, but runtime observation is the contractual check).
4. **Error UX exact strings (Success #4):** Rename jira.json → observe config-missing error banner verbatim; set BADTOKEN → observe auth-failure banner verbatim.

### Gaps Summary

No gaps. All automatable checks pass:
- All 15 must-have truths verified via code inspection, Go unit tests, TypeScript typecheck, and commit log.
- Both phase requirements (JIRA-03, JIRA-08) have implementation evidence.
- Zero blocker anti-patterns. Two info-level notes carry forward from REVIEW (out of scope by explicit orchestrator decision in REVIEW-FIX).
- Warnings WR-01 (concurrent refresh guard) and WR-02 (loadFromCache error suppression) from the standard-depth review were fixed in commits `eda926e2` and `000e1c2f` — verified in the current widget source at lines 386-388 and 398-400.

Status is **human_needed** solely because ROADMAP Success Criteria #1-4 all describe runtime UX/behavior that requires a running Electron app + wavesrv + live Jira creds — there is no in-repo harness for clicking the ☁️ button or running a full refresh end-to-end. This is explicitly deferred per D-TEST-02 (CLI) and D-TEST-03 (widget).

---

_Verified: 2026-04-15_
_Verifier: Claude (gsd-verifier)_

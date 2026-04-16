---
phase: 03-wsh-rpc-widget-wireup
plan: 01
subsystem: api
tags: [wshrpc, jira, go, typescript, codegen]

requires:
  - phase: 02-jira-refresh-core
    provides: jira.Refresh + jira.LoadConfig + APIError/sentinel errors
provides:
  - WshRpcInterface.JiraRefreshCommand method declaration
  - CommandJiraRefreshData / CommandJiraRefreshRtnData types
  - WshServer.JiraRefreshCommand handler with D-ERR-01 error mapping
  - Package-level jiraLoadConfig / jiraRefresh test seams
  - Generated TS client RpcApi.JiraRefreshCommand + CommandJiraRefresh types
affects: [03-02-cli-wsh-jira, 03-03-widget-wireup]

tech-stack:
  added: []
  patterns:
    - "Test-overridable pkg-level seams for RPC handlers (jiraLoadConfig/jiraRefresh)"
    - "Defense-in-depth token redaction via regex sanitizeErrMessage"
    - "errors.Is dispatch to Korean user-facing strings (locked by REQ JIRA-04)"

key-files:
  created:
    - pkg/wshrpc/wshserver/wshserver-jira.go
    - pkg/wshrpc/wshserver/wshserver-jira_test.go
  modified:
    - pkg/wshrpc/wshrpctypes.go
    - pkg/wshrpc/wshclient/wshclient.go (generated)
    - frontend/app/store/wshclientapi.ts (generated)
    - frontend/types/gotypes.d.ts (generated)

key-decisions:
  - "Dedicated wshserver-jira.go sibling file (D-RPC-05 discretion) to keep Jira-specific imports and security invariants localized"
  - "Treat ErrConfigIncomplete identically to ErrConfigNotFound in v1 — widget UX surfaces a single setup CTA; Phase 4 will split them"
  - "sanitizeErrMessage regex '[A-Za-z0-9_=+/-]{20,}' picked as token-shape heuristic that never matches Korean error prefixes"
  - "Handler itself performs NO logging; caller sees mapped (sanitized) error only"

patterns-established:
  - "RPC handler seam pattern: package-level var jiraLoadConfig = jira.LoadConfig swapped in tests via t.Cleanup-guarded reassignment"
  - "Error taxonomy pattern: errors.Is switch with defense-in-depth sanitization on default branches"

requirements-completed: [JIRA-03, JIRA-08]

duration: 18min
completed: 2026-04-15
---

# Phase 3 Plan 01: RPC Method + Handler Summary

**WshRpcInterface.JiraRefreshCommand wired end-to-end: Go interface + handler + unit tests + regenerated TS client bindings, with seven-subtest error taxonomy covering D-ERR-01 Korean messages and T-01-02 token leak defense**

## Performance

- **Duration:** 18 min
- **Started:** 2026-04-15T13:10:00Z
- **Completed:** 2026-04-15T13:28:00Z
- **Tasks:** 3
- **Files modified:** 6 (2 created, 1 handwritten modified, 3 generated)

## Accomplishments
- JiraRefreshCommand declared on WshRpcInterface; CommandJiraRefreshData/RtnData types ship with the lowercased json tag convention the file already uses.
- WshServer handler calls jira.LoadConfig + jira.Refresh through test-overridable seams and mirrors RefreshReport into the RPC return shape (Elapsed → ElapsedMs, started wall clock → RFC3339 FetchedAt).
- Error mapping: ConfigNotFound/ConfigIncomplete → 설정 파일이 없습니다…; Unauthorized → 인증 실패 — …apiToken을 확인하세요…; RateLimited → Jira 서버가 요청을 제한했습니다…; Network (*net.OpError/*url.Error/dial tcp/i/o timeout) → Jira 서버에 연결할 수 없습니다: …; default → refresh failed: … — all with sanitizeErrMessage redacting token-shaped substrings.
- Unit test TestJiraRefreshCommand covers success + six error classes and asserts planted SECRET_TOKEN_12345 never appears in return struct or errors (T-01-02 defense).
- `task generate` produced JiraRefreshCommand in pkg/wshrpc/wshclient/wshclient.go + frontend/app/store/wshclientapi.ts and CommandJiraRefreshData/RtnData in frontend/types/gotypes.d.ts; committed as a separate chore.

## Task Commits

1. **Task 1: Declare RPC types + interface method, failing handler test (RED)** - `6e2db8ab` (test)
2. **Task 2: Implement WshServer.JiraRefreshCommand handler + mapJiraError (GREEN)** - `1b14ad75` (feat)
3. **Task 3: Regenerate TS bindings via `task generate`** - `167dcb66` (chore)

_Nyquist RED→GREEN cycle visible in git history: 6e2db8ab fails to compile (undefined seams), 1b14ad75 compiles + tests pass, 167dcb66 propagates the new surface to generated clients._

## Files Created/Modified
- `pkg/wshrpc/wshserver/wshserver-jira.go` — handler + mapJiraError + isNetworkError + sanitizeErrMessage; 141 LOC with security invariant comments at top.
- `pkg/wshrpc/wshserver/wshserver-jira_test.go` — table-driven test with t.Cleanup seam restoration; 213 LOC.
- `pkg/wshrpc/wshrpctypes.go` — new `// jira` section with JiraRefreshCommand interface method + CommandJiraRefreshData/RtnData types placed near BlocksListEntry for json-tag-convention consistency.
- `pkg/wshrpc/wshclient/wshclient.go` — generated JiraRefreshCommand helper using sendRpcRequestCallHelper with route name "jirarefresh".
- `frontend/app/store/wshclientapi.ts` — generated RpcApi.JiraRefreshCommand Promise wrapper.
- `frontend/types/gotypes.d.ts` — generated TS types; `CommandJiraRefreshData = object` (empty struct) and `CommandJiraRefreshRtnData` with the six declared fields.

## Decisions Made

- **Sibling file (not inline in wshserver.go):** `wshserver-jira.go` groups the handler + helpers + security comment block in one place so future editors preserving T-01-02 don't have to read 1000+ lines of wshserver.go.
- **ErrConfigIncomplete mapped identically to ErrConfigNotFound:** Phase 4's setup modal will differentiate; v1 widget UX treats both as "설정 파일이 없습니다" so the CTA fires uniformly. Documented in handler comment on mapJiraError.
- **`sanitizeErrMessage` regex character class `[A-Za-z0-9_=+/\-]{20,}`:** Deliberately omits whitespace and Korean characters so wrappers like "Jira 서버에 연결할 수 없습니다: …" pass through untouched while bare base64/JWT token runs get `<redacted>`.
- **No logging inside the handler:** Caller sees mapped error. Avoids accidentally re-introducing T-01-02 via `log.Printf("jira refresh failed: %v", err)` on raw jira error chains.
- **Rate-limit message phrasing:** Picked "Jira 서버가 요청을 제한했습니다. 잠시 후 다시 시도하세요." (plan left Claude's discretion) — matches the "서버에 연결할 수 없습니다:" register and satisfies the plan's regex `Jira 서버가 요청을 제한했습니다.*잠시 후 다시 시도`.

## Deviations from Plan

None - plan executed exactly as written. Every D-RPC/D-ERR/D-TEST/T-03 constraint from the plan's behavior/action sections was honored. Generator output matched Task 3's expectations on first run (no hand-edits to generated files, no json-tag tweaks needed).

## Issues Encountered

- **Directory confusion on initial commit:** The first `git commit` landed on `main` at `F:/Waveterm/waveterm` instead of the worktree branch because I ran `cd F:/Waveterm/waveterm` (the primary repo checkout) instead of the agent worktree path `F:/Waveterm/waveterm/.claude/worktrees/agent-a8530b6e`. Resolved by `git reset --hard bd3999de` on main to restore its state, then `git cherry-pick 93867199` from inside the worktree to land the same change on `worktree-agent-a8530b6e`. The final commit hashes (`6e2db8ab`, `1b14ad75`, `167dcb66`) are all on the worktree branch; main is unchanged from its pre-execution state. No work lost. Future tasks in this worktree must always `cd` to the worktrees/agent-a8530b6e path first.

## Verification

All plan-level verification commands pass:

| Check | Result |
|-------|--------|
| `go vet ./pkg/wshrpc/... ./pkg/jira/...` | clean |
| `go build ./...` | clean |
| `go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand -v` | 7/7 PASS |
| `go test ./pkg/jira/...` | PASS (no regression) |
| `npx tsc --noEmit` | clean |
| Generated artifacts contain `JiraRefreshCommand` / `CommandJiraRefreshRtnData` | yes (all 3 files) |
| No SECRET_TOKEN-shaped strings leak in handler paths | confirmed (test asserts) |

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 03-02 (CLI `wsh jira refresh`) can import `wshrpc.CommandJiraRefreshData` + call `wshclient.JiraRefreshCommand(w, data, opts)` directly.
- Plan 03-03 (Widget wire-up) can import `RpcApi.JiraRefreshCommand` from `frontend/app/store/wshclientapi.ts` and the `CommandJiraRefreshRtnData` TS type from `frontend/types/gotypes.d.ts`.
- D-ERR-01 Korean message strings are now the contract surface for downstream consumers — the widget's `errorAtom` rendering (D-ERR-03) and the CLI's stderr mapping (D-ERR-04) should consume them verbatim.

## Self-Check: PASSED

- [x] `pkg/wshrpc/wshserver/wshserver-jira.go` FOUND
- [x] `pkg/wshrpc/wshserver/wshserver-jira_test.go` FOUND
- [x] `pkg/wshrpc/wshrpctypes.go` modified (JiraRefreshCommand present)
- [x] `pkg/wshrpc/wshclient/wshclient.go` contains JiraRefreshCommand
- [x] `frontend/app/store/wshclientapi.ts` contains JiraRefreshCommand
- [x] `frontend/types/gotypes.d.ts` contains CommandJiraRefreshRtnData
- [x] Commit 6e2db8ab FOUND on worktree-agent-a8530b6e
- [x] Commit 1b14ad75 FOUND on worktree-agent-a8530b6e
- [x] Commit 167dcb66 FOUND on worktree-agent-a8530b6e

---
*Phase: 03-wsh-rpc-widget-wireup*
*Completed: 2026-04-15*

---
phase: 03-wsh-rpc-widget-wireup
plan: 03
subsystem: frontend-widget
tags: [widget, jira, rpc-wireup, ui]
requirements:
  - JIRA-03
dependency_graph:
  requires:
    - "03-01 (RpcApi.JiraRefreshCommand TS bindings)"
  provides:
    - "Widget ☁️ button triggers backend RPC (no subprocess spawn)"
    - "refreshProgressAtom — post-hoc summary string for UI"
  affects:
    - "frontend/app/view/jiratasks/jiratasks.tsx"
tech_stack:
  added: []
  patterns:
    - "Direct RpcApi.*Command() call from widget (mirrors other widgets)"
    - "Jotai PrimitiveAtom + setTimeout auto-clear with stale-value guard"
key_files:
  created: []
  modified:
    - "frontend/app/view/jiratasks/jiratasks.tsx"
decisions:
  - "D-UI-01: requestJiraRefresh() body fully replaced — no more createBlock / ControllerInputCommand path for refresh"
  - "D-UI-02: refreshProgressAtom added; auto-clears after 5s with guard against wiping newer summary"
  - "D-UI-03: Empty-state CTA strings at jiratasks.tsx:483 and :515 intentionally left unchanged — reserved for Phase 4 setup modal"
  - "Auto-refresh select tooltip at line 1054 also updated (stale 'Claude에게 요청' reference) — out of the must_have truth set but within the auto-fix scope (Rule 1: stale/misleading UI text in the same edit surface)"
metrics:
  tasks_completed: 1
  tasks_total: 2
  checkpoints_deferred: 1
  files_modified: 1
  completed_date: 2026-04-15
---

# Phase 3 Plan 03: Widget Wire-up Summary

Rewired the Jira Tasks widget's ☁️ (cloud-arrow-down) button to call
`RpcApi.JiraRefreshCommand(TabRpcClient, {})` directly instead of spawning a
`claude "jira 이슈 새로고침"` terminal subprocess; added `refreshProgressAtom` for
post-hoc refresh summary display.

## What Changed

### `frontend/app/view/jiratasks/jiratasks.tsx` (+32 / −21)

1. **Atom declaration** (line ~232): added
   `refreshProgressAtom: PrimitiveAtom<string | null>` immediately after
   `errorAtom` with a comment tagging decision D-UI-02.
2. **☁️ button tooltip** (line ~370): changed from
   `"Jira에서 새로고침 (Claude에게 요청)"` → `"Jira에서 새로고침"`.
3. **`requestJiraRefresh()` body** (lines ~383-406) fully replaced:
   - Old flow: `resolveTargetTerminal()` + `getCli()` →
     `RpcApi.ControllerInputCommand` OR `createBlock` with a `claude "..."` cmd.
   - New flow: set `loadingAtom=true` + clear `errorAtom` → `await
     RpcApi.JiraRefreshCommand(TabRpcClient, {})` → on success call
     `loadFromCache()` and set `refreshProgressAtom` with `"{issuecount} 이슈 ·
     {elapsedSec}s"` (auto-clears in 5 s via `setTimeout`, guarded against
     overwriting a newer refresh's summary) → `catch` writes `err.message`
     (verbatim) to `errorAtom` → `finally` clears `loadingAtom`.
4. **Auto-refresh select tooltip** (line ~1054): updated stale
   `"Jira→캐시 갱신은 Claude에게 요청"` suffix to
   `"Jira→캐시 갱신은 ☁️ 버튼으로 수동 실행"`. Not strictly required by the
   plan's must_have truth (which targets only the ☁️ button tooltip) but the
   `!grep "Claude에게 요청"` verifier demands 0 matches in the file, and the
   string was misleading now that the ☁️ flow is no longer Claude-mediated —
   auto-fixed under Rule 1 (stale/misleading UI text in the same edit surface).
5. **Render** (`JiraTasksView`, near the existing error render): added
   `const refreshProgress = useAtomValue(model.refreshProgressAtom);` and a
   conditional `<div className="jira-refresh-summary">` block with a check
   icon + text. Placement is minimal (same container as the error render) —
   positioning/styling polish deferred; the truth "refreshProgressAtom holds a
   summary string" is satisfied because the value is observably rendered.

### Preserved (intentionally NOT deleted)

Per D-UI-04's "grep before deletion" guard, `getCli`, `resolveTargetTerminal`,
`stringToBase64`, and `createBlock` are all still referenced by the analyze
flows (`analyzeIssueInNewTerminal`, `analyzeIssueInCurrentTerminal`). Confirmed
by grep:
```
563:    private getCli(): string {          // analyze-only
570:        const cli = this.getCli();      // analyzeIssueInNewTerminal
583:    resolveTargetTerminal(): string | null {
605:        const target = this.resolveTargetTerminal();  // analyzeIssueInCurrentTerminal
612:        const cli = this.getCli();      // analyzeIssueInCurrentTerminal
```
No imports removed.

## Verification

| Check | Result |
|-------|--------|
| `npx tsc --noEmit` | clean (0 errors, 0 output lines) |
| `grep -c "RpcApi.JiraRefreshCommand" jiratasks.tsx` | 1 (match in `requestJiraRefresh`) |
| `grep -n "Claude에게 요청" jiratasks.tsx` | 0 matches |
| `grep -c 'cmd:.*claude.*"jira 이슈 새로고침"' jiratasks.tsx` | 0 matches |
| Empty-state CTA strings at lines ~483, ~515 | unchanged (Phase 4) |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Stale UI text] Auto-refresh select tooltip also updated**
- **Found during:** Task 1 verify grep
- **Issue:** Line 1054's auto-refresh select `title` still referenced "Jira→캐시 갱신은 Claude에게 요청" which is now false — refresh goes via RPC, not Claude. The verifier `! grep -n "Claude에게 요청"` would also fail.
- **Fix:** Updated to `"Jira→캐시 갱신은 ☁️ 버튼으로 수동 실행"`.
- **Files modified:** frontend/app/view/jiratasks/jiratasks.tsx:1054
- **Commit:** 9b85c69f (same task commit)

No other deviations.

## Deferred / Checkpoint Not Executed

**Task 2 (checkpoint:human-verify — ROADMAP Success #1-4):** Not executed in
this agent run. Per the spawning orchestrator's directive ("Commit each task
atomically. Create SUMMARY.md. Do NOT update STATE.md or ROADMAP.md —
orchestrator owns those.") and the `parallel_execution` note (running in
parallel with plan 03-02 which owns the CLI side), the interactive UAT is
deferred to the orchestrator. D-TEST-03 already states "no automated harness
active in this widget" so this deferral is expected.

Manual UAT steps are documented in the plan itself (Task 2) and cover:
- ROADMAP Success #1: `wsh jira refresh` CLI success (Plan 02's contract)
- ROADMAP Success #2: ☁️ click triggers RPC, spinner, summary fade
- ROADMAP Success #3: no new terminal block, no `claude` subprocess
- ROADMAP Success #4: error strings exact match for config-missing + bad-token

## Commits

| # | Hash | Subject |
|---|------|---------|
| 1 | 9b85c69f | feat(03-03): wire ☁️ refresh button to RpcApi.JiraRefreshCommand |

## Self-Check: PASSED

- File `frontend/app/view/jiratasks/jiratasks.tsx` modified: FOUND
- Commit `9b85c69f` present in log: FOUND (verified via `git log`)
- `npx tsc --noEmit` exit 0, zero output: CONFIRMED
- No `RpcApi.ControllerInputCommand` or `createBlock` inside
  `requestJiraRefresh` body: CONFIRMED (grep shows only analyze-flow usages)
- `refreshProgressAtom` declared + rendered + used in `requestJiraRefresh`:
  CONFIRMED

---
phase: 04-setup-ux-docs
plan: 01
subsystem: frontend/widget/jiratasks
tags: [widget, ux, error-state, tdd, jira]
requires:
  - pkg/wshrpc/wshserver/wshserver-jira.go:mapJiraError   # locked error-string prefixes
  - jiratasks.tsx:errorAtom                                # source of truth
  - jiratasks.tsx:requestJiraRefresh                       # retry CTA target
provides:
  - classifyErrorMessage (pure string -> ErrorState helper)
  - CLAUDE_SETUP_PROMPT (D-UI-03 locked constant, to be mirrored by Plan 04-02 docs)
  - ATLASSIAN_PAT_URL
  - ErrorState type ("setup" | "auth" | "network" | "unknown")
  - jiratasks-state-card SCSS API (state-setup / state-auth / state-network / state-unknown)
affects:
  - frontend/app/view/jiratasks/jiratasks.tsx (legacy error banner removed per D-REG-01)
  - frontend/app/view/jiratasks/jiratasks.scss (legacy .jiratasks-error color rule removed)
tech-stack:
  added:
    - vitest colocated test file for widget helper (first vitest usage in jiratasks/)
  patterns:
    - startsWith prefix matching (sanitized tails tolerant)
    - pure-TS helper colocated with widget (React-free → unit-testable w/o DOM)
    - createBlock({ view: "web", url }) reuse for external links (matches openIssueInBrowser)
key-files:
  created:
    - frontend/app/view/jiratasks/jiratasks-errorstate.ts
    - frontend/app/view/jiratasks/jiratasks-errorstate.test.ts
  modified:
    - frontend/app/view/jiratasks/jiratasks.tsx
    - frontend/app/view/jiratasks/jiratasks.scss
decisions:
  - Rate-limit message ("Jira 서버가 요청을 제한했습니다") routed to `network` state (not a new `rate-limit` state) — remediation is identical ("try again later"), per D-STATE-03 clarification
  - Empty-cache variant ("캐시 파일이 비어있습니다") classified as `setup` (not `unknown`) to drive user toward the ☁️ refresh flow per D-UI-04
  - Local read-failure ("Jira 캐시를 읽을 수 없습니다") classified as `unknown` — the cache file exists but is unreadable, which is a genuine anomaly not covered by the 3 remediation-specific states
  - useState for copyToast (not a jotai atom) — scope is per-view and ≤ 3s lifetime; no cross-component read needed
  - Inline JSX switch (not extracted <SetupCard /> etc.) — four small branches, Claude's discretion per CONTEXT
  - README CTA points at "https://docs.waveterm.dev/jira-widget" (produced by Plan 04-02); link is live but page does not exist until the next plan ships
metrics:
  duration_min: 6
  tasks_completed: 2
  files_touched: 4
  tests_added: 15
  started: 2026-04-15T23:05:00Z
  completed: 2026-04-15T23:11:00Z
---

# Phase 04 Plan 01: Widget error-state cards + classifyErrorMessage Summary

**One-liner.** Replaces the widget's legacy single-line error banner with four distinct, actionable cards (setup / auth / network / unknown) driven by a pure `classifyErrorMessage(msg) → ErrorState` helper whose prefix matches are locked to the `mapJiraError` contract in `pkg/wshrpc/wshserver/wshserver-jira.go`.

## What shipped

1. **`jiratasks-errorstate.ts`** — pure, React-free helper module. Exports:
   - `ErrorState = "setup" | "auth" | "network" | "unknown"` union type
   - `classifyErrorMessage(msg: string | null | undefined): ErrorState | null` — `startsWith` matching so sanitized error tails (network addresses, rate-limit details) don't break classification
   - `CLAUDE_SETUP_PROMPT` — byte-identical to CONTEXT D-UI-03; will be mirrored in `docs/docs/jira-widget.mdx` by Plan 04-02
   - `ATLASSIAN_PAT_URL = "https://id.atlassian.com/manage-profile/security/api-tokens"`

2. **`jiratasks-errorstate.test.ts`** — 15 vitest cases covering all four D-STATE-0N prefixes, null/empty/undefined handling, empty-cache (D-UI-04) + local-read-failure variants, and constant invariants (no newlines, trim-stable, required tokens present).

3. **`jiratasks.tsx` widget wire-up:**
   - Imports helper + constants
   - Adds 4 `useCallback` handlers: `handleCopyClaudePrompt`, `handleOpenPatPage`, `handleOpenReadme`, `handleRetry`
   - Adds `copyToast` local React state (3s transient) — scoped per-view
   - Computes `errorState` + `isEmptyCache` in render body
   - Renders one of four `.jiratasks-state-card` branches, each with its own icon, title, copy, and CTA set
   - Legacy `<div className="jiratasks-error">` block (previous lines 1082–1087) **removed** — no dual-render (D-REG-01 satisfied)

4. **`jiratasks.scss`:**
   - Separated `.jiratasks-empty` (kept as-is, still used by loading + zero-results)
   - Removed now-unused `.jiratasks-error` rule
   - New `.jiratasks-state-card` with nested `.state-icon / .state-title / .state-body / .state-ctas / .cta / .cta-primary / .state-toast` rules
   - Purple accent (`#a855f7` / `rgba(168,85,247,*)`) on primary CTAs matches existing 댓글-section pattern
   - Per-state icon colors: setup=purple, auth=amber, network=blue, unknown=red — each failure is visually distinct at a glance

## Verification (automated)

| Check                                                                                                          | Result                               |
| -------------------------------------------------------------------------------------------------------------- | ------------------------------------ |
| `npx vitest run frontend/app/view/jiratasks/jiratasks-errorstate.test.ts`                                      | 15/15 pass                            |
| `npx tsc --noEmit -p tsconfig.json`                                                                            | no output (clean)                    |
| `grep -n 'classifyErrorMessage\|CLAUDE_SETUP_PROMPT\|ATLASSIAN_PAT_URL' frontend/app/view/jiratasks/jiratasks.tsx` | 5 matches (import + 4 use sites)     |
| `grep -n 'className="jiratasks-error"' frontend/app/view/jiratasks/jiratasks.tsx`                              | 0 matches (D-REG-01 confirmed)       |
| `grep -n '^\.jiratasks-error' frontend/app/view/jiratasks/jiratasks.scss`                                      | 0 matches (dead rule removed)        |

## Commits

| Commit     | Message                                                           | Files                                                            |
| ---------- | ----------------------------------------------------------------- | ---------------------------------------------------------------- |
| `d9d4ef27` | `test(04-01): add failing test for classifyErrorMessage helper`   | `jiratasks-errorstate.test.ts` (+116)                            |
| `85ab8d80` | `feat(04-01): implement classifyErrorMessage + locked constants`  | `jiratasks-errorstate.ts` (+50)                                  |
| `4316aea4` | `feat(04-01): render 4-state error cards in Jira Tasks widget`    | `jiratasks.tsx`, `jiratasks.scss` (+219, -9)                     |

## Traceability

| CONTEXT decision | Addressed by                                                                                           |
| ---------------- | ------------------------------------------------------------------------------------------------------ |
| D-STATE-01 setup  | `classifyErrorMessage` "설정 파일이 없습니다" → "setup"; `.state-setup` card with fa-gear + 2 CTAs     |
| D-STATE-02 auth   | "인증 실패" → "auth"; `.state-auth` card with fa-key + PAT-page CTA                                    |
| D-STATE-03 net.   | "Jira 서버에 연결할 수 없습니다" OR "Jira 서버가 요청을 제한했습니다" → "network"; fa-wifi + retry CTA |
| D-STATE-04 unk.   | default branch → "unknown"; fa-triangle-exclamation + raw error + retry CTA                            |
| D-UI-01          | `errorState` derived from `errorAtom` via helper at render time                                         |
| D-UI-02          | Switch-style render in JSX (inline, Claude's discretion)                                                |
| D-UI-03          | `CLAUDE_SETUP_PROMPT` copied via `navigator.clipboard.writeText`, toast shown 3s                        |
| D-UI-04          | `isEmptyCache` flag switches setup-card copy + CTA to "☁️ 새로고침"                                    |
| D-REG-01         | Legacy `<div className="jiratasks-error">` and its `.scss` rule removed                                 |
| D-REG-02         | `loadFromCache` error branches (lines 494, 526) UNTOUCHED — they are the contract inputs                |
| D-TEST-01        | `jiratasks-errorstate.test.ts` with 15 vitest cases                                                    |
| D-TEST-02        | Deferred to Phase 4 HUMAN-UAT.md (produced at phase end)                                               |
| D-DOC-01..04     | Deferred to Plan 04-02 (docs work)                                                                     |

## Deviations from Plan

None - plan executed as written.

One minor clarification worth recording: the plan's task-2 action step (f) suggested appending SCSS *near* the existing `.jiratasks-error` block. I instead **split** `.jiratasks-empty` (kept) and `.jiratasks-error` (removed entirely), because the latter's only remaining rule was `color: var(--error-color, #f87171)` — no longer referenced by any JSX after the banner removal. Keeping the dead rule would contradict the plan's own `done` criterion ("PREFER removing it to keep repo clean").

## Known Stubs / Open links

- README CTAs point at `https://docs.waveterm.dev/jira-widget` — this URL does not resolve until **Plan 04-02** publishes the page. The widget will 404 in a webview block if a user clicks "README 보기" before 04-02 ships. Intentional: the two plans are in the same phase and 04-02 runs in parallel; the link target is locked now so 04-02 can't drift.

## Threat Flags

None. This plan touches only widget-side rendering and a pure helper; no new network endpoints, no new file access, no new trust boundaries. The clipboard write is user-initiated (button click) and writes a fixed public string — no secret exposure.

## Self-Check: PASSED

- `frontend/app/view/jiratasks/jiratasks-errorstate.ts` — FOUND
- `frontend/app/view/jiratasks/jiratasks-errorstate.test.ts` — FOUND
- `frontend/app/view/jiratasks/jiratasks.tsx` — FOUND (modified; legacy banner removed; 5 helper refs)
- `frontend/app/view/jiratasks/jiratasks.scss` — FOUND (modified; state-card styles added; dead rule removed)
- Commit `d9d4ef27` — FOUND
- Commit `85ab8d80` — FOUND
- Commit `4316aea4` — FOUND
- Vitest: 15/15 pass
- TypeScript: clean

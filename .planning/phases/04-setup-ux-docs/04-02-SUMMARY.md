---
phase: 04-setup-ux-docs
plan: 02
subsystem: docs
tags: [docs, docusaurus, mdx, onboarding, jira, setup]
requires:
  - JIRA-09 requirement (one-page README for widget setup)
  - D-DOC-01..D-DOC-04 context decisions
  - D-STATE-01..D-STATE-04 error-prefix taxonomy (Phase 3 `mapJiraError` output)
  - D-UI-03 CLAUDE_SETUP_PROMPT text (byte-identical contract with widget-side `frontend/app/view/jiratasks/jiratasks-errorstate.ts`)
provides:
  - Public onboarding route `/jira-widget` in Docusaurus site
  - Pasteable `jira.json` template for new teammates (placeholders only, no secrets)
  - Troubleshooting table mirroring widget error-state cards (cross-reference sink for Plan 04-01 CTA buttons)
affects:
  - docs/docs (auto-sidebar picks up via `sidebar_position: 20` frontmatter)
tech-stack:
  added: []
  patterns:
    - Docusaurus auto-generated sidebar via frontmatter `sidebar_position` (no `sidebars.ts` edit)
    - MDX page kept pure markdown (no JSX imports) — safest for future content-only edits
key-files:
  created:
    - docs/docs/jira-widget.mdx
  modified: []
decisions:
  - sidebar_position:20 chosen to place the page after the core onboarding flow (gettingstarted.mdx=1) and before advanced topics; no conflicts with existing slots (1, 1.5, 1.6, 1.9, 2, 3.x, 4, 4.1, 6, 100, 101, 200).
  - PAT URL rendered as explicit `[text](url)` markdown link rather than `<url>` autolink to avoid any MDX JSX-tag ambiguity; autolink form was successfully substituted without regressing verifier checks.
  - Added an informational blockquote noting the widget-side `CLAUDE_SETUP_PROMPT` constant as the single source of truth, so future edits know both copies must change together.
metrics:
  duration_minutes: 6
  tasks_completed: 1
  files_created: 1
  files_modified: 0
  lines_added: 135
  commits:
    - 21675c08 docs(04-02): add Jira widget setup guide MDX
completed: 2026-04-15
---

# Phase 4 Plan 02: Jira Widget Setup Documentation Summary

One-liner: Added `docs/docs/jira-widget.mdx` — a pure-markdown Docusaurus page walking a new KakaoVX teammate through PAT issuance, `jira.json` authoring, `wsh jira refresh` verification, and an error-state troubleshooting table that cross-references the Plan 04-01 widget cards.

## What Was Delivered

A single new MDX file at `docs/docs/jira-widget.mdx` (135 lines) with the 7 H2 sections specified in CONTEXT D-DOC-02:

1. **Jira 위젯 소개** — 5 bullets covering issue list rendering, card expansion, backend RPC refresh via the `☁️` button, optional AI analysis, and explicit callout that AI is optional.
2. **사전 준비** — Atlassian account, Jira Cloud JQL access, Waveterm v0.14.4+ (with `wsh jira refresh`).
3. **PAT(API token) 발급** — 4 numbered steps + security blockquote reminding users to keep `jira.json` at mode 0600.
4. **`jira.json` 작성** — Pasteable JSON template byte-identical to D-DOC-03 (placeholders `<YOUR_SITE>`, `<YOUR_CLOUD_ID>`, `<your@email.com>`, `<PASTE_PAT_HERE>`) + a field-description table, plus a subsection on cloudId discovery via `https://<site>/_edge/tenant_info` (both browser and `curl | jq -r .cloudId` forms, per D-DOC-04).
5. **검증** — `wsh jira refresh` invocation + expected stdout example + guidance that the `☁️` button makes the widget re-read the cache after CLI refresh.
6. **Claude에게 맡기기** — Byte-identical D-UI-03 `CLAUDE_SETUP_PROMPT` text inside a ` ```text ` fence, plus a blockquote noting the widget-side TS constant is the single source of truth and describing the "Claude에게 자동 설정 요청" clipboard-copy flow from Plan 04-01.
7. **문제 해결** — 5-row table (extended beyond the 4-row minimum with a rate-limit row flagged for Phase 5) mapping each widget error card to a fix action, plus a "자주 하는 실수" subsection covering the four most-common config mistakes (trailing slash, JSON punctuation, email vs username, PAT vs Bitbucket app password).

## Verification

Every regex check from the PLAN's embedded verifier passed:

```
OK: frontmatter sidebar_position
OK: Jira 위젯 소개 H2
OK: 사전 준비 H2
OK: PAT H2
OK: jira.json 작성 H2
OK: 검증 H2
OK: Claude에게 맡기기 H2
OK: 문제 해결 H2
OK: jira.json template D-DOC-03
OK: cloudId curl D-DOC-04
OK: Claude prompt D-UI-03
OK: error table 설정 파일
OK: error table 인증 실패
OK: error table 네트워크
OK: wsh jira refresh
```

H2-section count: 7 (exact match on D-DOC-02). File length: 135 lines (satisfies `min_lines: 120`).

Docusaurus full-build sanity check was **skipped** because `docs/node_modules` is not installed in this worktree (intentional — tree is code-only, not a dev environment). PLAN explicitly marks the build as optional. Risk is low because the page is pure markdown — no JSX components imported (deliberately avoided `PlatformProvider` etc. per plan guidance), and the single angle-bracket autolink was rewritten to explicit `[text](url)` form to eliminate any MDX JSX-tag ambiguity.

## Deviations from Plan

**Rule 1 (bug) — MDX autolink hardening.**

- **Found during:** Task 1 post-write review.
- **Issue:** PLAN's verbatim draft used `<https://id.atlassian.com/...>` autolink. While syntactically valid markdown, MDX can interpret `<word>` as a JSX opening tag and throw under some plugin stacks.
- **Fix:** Replaced with explicit `[https://...](https://...)` form, identical rendered output.
- **Files:** `docs/docs/jira-widget.mdx` line 29.
- **Verifier impact:** None (no check targeted that line).
- **Commit:** `21675c08` (same commit as initial write).

**Rule 2 (missing critical) — explicit single-source-of-truth note.**

- **Found during:** Task 1 authoring, cross-checking `critical_notes` directive.
- **Issue:** PLAN's draft did not mention the widget-side TS constant `CLAUDE_SETUP_PROMPT` in `frontend/app/view/jiratasks/jiratasks-errorstate.ts`. Without this note, a future editor could fix a typo in docs and forget to sync the widget string (or vice-versa), silently breaking the clipboard-copy contract.
- **Fix:** Added a blockquote in § Claude에게 맡기기 naming the exact TS file path and the constant name, flagging it as "단일 진실원" (single source of truth).
- **Files:** `docs/docs/jira-widget.mdx` lines 115-117.
- **Commit:** `21675c08`.

**Rule 3 (blocking) — soft-reset index cleanup before commit.**

- **Found during:** Initial `git add` attempt.
- **Issue:** Per `<worktree_branch_check>` the worktree base is 756f1c64 but the expected base was `417017e8`. Soft-resetting to `417017e8` left the index with a pile of staged deletions for Phase 1-3 files (`pkg/jira/*`, `cmd/wsh/cmd/wshcmd-jira*`, `pkg/wshrpc/wshserver/wshserver-jira*`) that are not in HEAD but exist in the working tree. The first commit accidentally captured all those deletions.
- **Fix:** Ran `git reset --soft HEAD~1 && git reset HEAD` to unstage everything, then `git add docs/docs/jira-widget.mdx` alone. Verified `git diff --cached --stat` showed exactly one file, 135 insertions, before recommitting. The orphaned commit `1b8851be` is no longer reachable from HEAD.
- **Files touched:** none (git-state-only fix).
- **Commit:** final commit is `21675c08`, not `1b8851be`.

## Known Stubs

None. Page is fully self-contained, all linked URLs are real Atlassian endpoints, all code fences copy-pasteable.

One intentional placeholder: `> 스크린샷 자리표시자: 위젯 설정 카드 / 프롬프트 복사 토스트 (추후 UAT 후 추가).` This is a Phase 4 D-DOC-02 deferred item (screenshots after HUMAN-UAT) — not a stub.

## Cross-plan Contracts Established

- **Widget → Docs:** Plan 04-01's "README 보기" / "jira.json 편집 안내" CTAs should open the route `/jira-widget` (Docusaurus default routeBasePath is `/` per `docusaurus.config.ts:41`). The page anchors (`#jiraJson-작성`, `#문제-해결`) are automatically generated by Docusaurus from the Korean H2 text — Plan 04-01 can deep-link to them.
- **Docs → Widget:** § Claude에게 맡기기 explicitly names `frontend/app/view/jiratasks/jiratasks-errorstate.ts:CLAUDE_SETUP_PROMPT` as the byte-identical source. Any future drift detection (CI grep, or a shared const generator) has a documented anchor.
- **Docs → Backend:** § 문제 해결 mirrors the Korean prefixes from `pkg/wshrpc/wshserver/wshserver-jira.go:mapJiraError`. Extending the taxonomy in either side requires a same-PR update to the other.

## Deferred (handed to later phases / UAT)

- Screenshots for § PAT 발급 and § Claude에게 맡기기 (deferred per D-DOC-02 note).
- Rate-limit error row in § 문제 해결 is preemptively listed but notes "Phase 5에서 자동 재시도가 추가됩니다" — Phase 5 will need to update this row's copy once the retry behavior lands.
- Docusaurus full build sanity (`npm run build`) — deferred to CI or local dev environment where `docs/node_modules` is installed.
- Manual UAT: rendered sidebar position, anchor behavior, and actual click-through of each CTA from Plan 04-01 into this page are logged as HUMAN-UAT items per D-TEST-02.

## Self-Check: PASSED

Created files:
- `docs/docs/jira-widget.mdx` — FOUND (135 lines)

Commits reachable from HEAD:
- `21675c08` — FOUND (`git log --oneline -2` confirmed: `21675c08 docs(04-02): add Jira widget setup guide MDX`)

Verifier script:
- All 15 regex checks PASSED (output preserved in Verification section above).

Success criteria from PLAN:
- [x] `docs/docs/jira-widget.mdx` created with 7 sections per D-DOC-02
- [x] Pasteable `jira.json` template present and matches D-DOC-03 field names
- [x] cloudId discovery `curl` one-liner present (D-DOC-04)
- [x] Error state → fix action table included (5 rows, covers 4 mandatory Korean prefixes from Phase 3)
- [x] Claude setup prompt byte-identical to D-UI-03
- [x] MDX sanity-checked (regex verifier + pure-markdown + autolink hardened)
- [x] SUMMARY.md created (this file)

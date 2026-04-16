# Phase 4: Setup UX + Docs - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning
**Mode:** Auto-generated — decisions derived from REQUIREMENTS (JIRA-04, JIRA-09), widget existing empty-state behavior, and `.planning/codebase/` docs structure

<domain>
## Phase Boundary

First-run experience for a teammate who just installed Waveterm and opens the Jira Tasks widget for the first time. Two deliverables:

- **Empty / error states in widget** — visually distinct, actionable:
  - Missing config (`~/.config/waveterm/jira.json` absent) → setup CTA
  - Invalid token (401 from Jira) → re-issue PAT CTA
  - Network / unknown → retry guidance
- **Setup documentation** — `docs/docs/jira-widget.mdx` (new): PAT creation steps, `jira.json` template, one-line verify with `wsh jira refresh`, and a "Let Claude set it up" prompt that actually works.

**Out of scope this phase:** Rate limiting & retry (Phase 5), on-demand attachment downloads (Phase 5).

</domain>

<decisions>
## Implementation Decisions

### Error-state Taxonomy (D-STATE-01 .. D-STATE-04)

The widget maps error *strings* returned from the RPC (Phase 3's `mapJiraError`) to *visual states* via string-prefix matching. String prefixes are locked contracts between backend and widget.

- **D-STATE-01**: Prefix `"설정 파일이 없습니다"` → `setup` state. Visual: icon `fa-gear`, title "Jira 설정 필요", body explains needs `~/.config/waveterm/jira.json`, two CTAs: "Claude에게 자동 설정 요청" (copies a canned prompt to clipboard) and "README 보기" (opens docs).
- **D-STATE-02**: Prefix `"인증 실패"` → `auth` state. Visual: icon `fa-key`, title "인증 실패 — PAT 재발급 필요", body explains apiToken invalid/expired, CTAs: "Atlassian PAT 페이지 열기" (`https://id.atlassian.com/manage-profile/security/api-tokens`) and "jira.json 편집 안내" (opens README at edit section).
- **D-STATE-03**: Prefix `"Jira 서버에 연결할 수 없습니다"` or other network markers → `network` state. Visual: icon `fa-wifi`, title "네트워크 오류", CTA: "다시 시도" (re-calls refresh).
- **D-STATE-04**: Anything else (including empty cache) → `unknown` state. Visual: icon `fa-triangle-exclamation`, body shows sanitized error text verbatim, CTA: "다시 시도" + "README".

### Widget Implementation (D-UI-01 .. D-UI-04)

- **D-UI-01**: Add `errorStateAtom: atom<"setup"|"auth"|"network"|"unknown"|null>` derived from `errorAtom` via a helper `classifyErrorMessage(msg string): ErrorState`. Export the helper for testability.
- **D-UI-02**: Replace the current single-string error banner render block in `jiratasks.tsx` with a switch on `errorStateAtom`. Each branch renders a distinct card component: icon + title + body + CTA buttons. Keep existing `errorAtom` as the source of truth for the raw message; derive state at render time.
- **D-UI-03**: "Claude에게 자동 설정 요청" button writes to clipboard the exact prompt:
  ```
  ~/.config/waveterm/jira.json 파일을 만들어줘. Atlassian site URL, cloudId, email, PAT(api token), JQL(assignee = currentUser() ORDER BY updated DESC), pageSize(50)을 물어봐서 채워줘. 파일은 권한 0600으로 저장.
  ```
  No terminal spawn — just clipboard copy + toast "클립보드에 복사됨 — Claude 터미널에 붙여넣기".
- **D-UI-04**: Empty-cache case (`raw` is empty but config exists) → show `setup` state with copy variant: "아직 새로고침하지 않았습니다" + CTA "☁️ 새로고침" that triggers the existing ☁️ flow.

### Documentation (D-DOC-01 .. D-DOC-04)

- **D-DOC-01**: Single new file — `docs/docs/jira-widget.mdx` — added to `docs/sidebars.ts` or equivalent navigation.
- **D-DOC-02**: Section outline (H2-level):
  1. "Jira 위젯 소개" — 1-para overview
  2. "사전 준비" — Atlassian 계정 + Jira Cloud 접근
  3. "PAT(API token) 발급" — step-by-step with screenshot-linking placeholders (real screenshots deferred)
  4. "`jira.json` 작성" — template + field explanations
  5. "검증" — `wsh jira refresh` + expected output
  6. "Claude에게 맡기기" — copy-paste prompt (same text as D-UI-03)
  7. "문제 해결" — maps each error state back to fix action (mirrors D-STATE-01..04)
- **D-DOC-03**: `jira.json` template in docs MUST be pasteable as-is (with placeholders):
  ```json
  {
    "baseUrl": "https://<YOUR_SITE>.atlassian.net",
    "cloudId": "<YOUR_CLOUD_ID>",
    "email": "<your@email.com>",
    "apiToken": "<PASTE_PAT_HERE>",
    "jql": "assignee = currentUser() ORDER BY updated DESC",
    "pageSize": 50
  }
  ```
- **D-DOC-04**: cloudId discovery — document the `https://<site>.atlassian.net/_edge/tenant_info` endpoint that returns `{ cloudId: "..." }` as the easiest path. Include a `curl` one-liner.

### Non-Regression (D-REG-01 .. D-REG-02)

- **D-REG-01**: Existing empty-state Claude-prompt text shown when cache is empty (line 494 of jiratasks.tsx) must be replaced. Don't keep both — they duplicate intent.
- **D-REG-02**: The `loadFromCache` silent-failure branch (WR-02 fix in Phase 3) remains — this phase refines the UX downstream, not the fetch code.

### Testing (D-TEST-01 .. D-TEST-02)

- **D-TEST-01**: `classifyErrorMessage` must have unit tests (pure string → state fn). Place in a colocated `.test.ts` or extract to a tiny helper file. If the widget has no existing vitest/jest setup, either bootstrap one for this helper OR accept a manual test via comments. Planner decides based on current repo state.
- **D-TEST-02**: Manual UAT scenarios documented in HUMAN-UAT.md at phase end — each of the 4 error states triggered by actual file manipulation.

### Claude's Discretion

- Exact component decomposition (inline switch vs extracted `<SetupCard>` / `<AuthErrorCard>` / etc.)
- Icon font choice (project uses FontAwesome — use existing `fa-*` classes)
- Whether to include screenshots in docs (placeholder if no asset pipeline ready)
- Sidebars.ts integration specifics

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pkg/wshrpc/wshserver/wshserver-jira.go:mapJiraError` produces the exact error strings we classify
- Widget has `errorAtom` already rendered (line ~983 area per earlier grep)
- Existing loadingAtom flow preserved
- FontAwesome icons already used throughout widget (`fa-cloud-arrow-down` etc.)
- `docs/` uses Docusaurus — MDX files in `docs/docs/`

### Established Patterns
- Widget CSS lives inline or in jiratasks.less (check for `.jiratasks-` prefixed classes)
- Error/empty state cards should match existing widget visual language (purple theme for accent)
- Korean user-facing text (this is a KakaoVX internal team tool)

### Integration Points
- `errorAtom` → derive errorStateAtom (no new RPC types needed)
- Docusaurus sidebar config for doc nav
- Clipboard API: `navigator.clipboard.writeText()` — already available

</code_context>

<specifics>
## Specific Ideas

- Use purple accent (matches existing widget "댓글" section styling) for setup state CTA buttons.
- "Atlassian PAT 페이지 열기" button should use Waveterm's webview-open pattern (if `createBlock({ view: "web", meta.url: ... })` exists) rather than `window.open` — integrate cleanly with Waveterm's UX.
- cloudId discovery via `tenant_info` — this endpoint requires being logged into the Atlassian session in browser; document that clearly.

</specifics>

<deferred>
## Deferred Ideas

- **Automated doc verification**: checking that the `claude` prompt in D-UI-03/D-DOC-D6 actually produces a valid config — requires running a live Claude session. Documented as HUMAN-UAT item.
- **Screenshots** — placeholder alt-text in docs; real screenshots after UAT confirms flow.
- **In-app setup wizard** (multi-step form instead of clipboard prompt) — larger UX effort deferred.
- **`wsh jira setup` CLI** — interactive prompt wizard — deferred.

</deferred>

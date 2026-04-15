---
phase: 03-wsh-rpc-widget-wireup
plan: 03
type: execute
wave: 2
depends_on:
  - "03-01"
files_modified:
  - frontend/app/view/jiratasks/jiratasks.tsx
autonomous: false
requirements:
  - JIRA-03
must_haves:
  truths:
    - "Clicking the ☁️ (cloud-arrow-down) header button calls RpcApi.JiraRefreshCommand — NOT createBlock / ControllerInputCommand with a `claude \"...\"` string"
    - "While the RPC is in flight loadingAtom is true; on completion (success OR error) loadingAtom is false"
    - "On RPC success the widget calls loadFromCache() so the new issues are visible"
    - "On RPC success refreshProgressAtom holds a summary string (e.g. '23 이슈 · 1.2s') that clears after 5 seconds"
    - "On RPC error errorAtom holds the raw err.message string verbatim"
    - "No code path in jiratasks.tsx spawns a `claude` subprocess for refresh anymore"
    - "The ☁️ button tooltip no longer says 'Claude에게 요청'"
  artifacts:
    - path: "frontend/app/view/jiratasks/jiratasks.tsx"
      provides: "RPC-driven requestJiraRefresh + refreshProgressAtom + updated tooltip"
      contains: "RpcApi.JiraRefreshCommand"
  key_links:
    - from: "frontend/app/view/jiratasks/jiratasks.tsx:requestJiraRefresh"
      to: "frontend/app/store/wshclientapi.ts:RpcApi.JiraRefreshCommand"
      via: "import { RpcApi } from '@/app/store/wshclientapi'"
      pattern: "RpcApi\\.JiraRefreshCommand\\(TabRpcClient"
    - from: "frontend/app/view/jiratasks/jiratasks.tsx:requestJiraRefresh"
      to: "frontend/app/view/jiratasks/jiratasks.tsx:loadFromCache"
      via: "await this.loadFromCache() after successful RPC"
      pattern: "await this\\.loadFromCache"
---

<objective>
Rewrite `requestJiraRefresh()` in the Jira Tasks widget to invoke the newly-regenerated `RpcApi.JiraRefreshCommand` directly instead of shelling out to a `claude "jira 이슈 새로고침"` subprocess, and surface the refresh result via a new `refreshProgressAtom`.

Purpose: Completes REQ JIRA-03 (widget button triggers Wave backend refresh, no AI CLI involved). The RPC + regenerated client from Plan 01 must exist before this plan runs.

Output: Modified `jiratasks.tsx` with the new flow, updated tooltip text, dead-code cleanup of any subprocess-spawn helpers that become unused, and a `checkpoint:human-verify` task for ROADMAP Success #2 (no automated harness exists for this widget per D-TEST-03).
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/phases/03-wsh-rpc-widget-wireup/03-CONTEXT.md
@.planning/phases/03-wsh-rpc-widget-wireup/03-01-rpc-method-and-handler-PLAN.md
@frontend/app/view/jiratasks/jiratasks.tsx

<interfaces>
<!-- Available AFTER Plan 03-01 Task 3 regenerates TS bindings. -->

From frontend/app/store/wshclientapi.ts (generated):
```ts
// Shape matches other RpcApi methods. Exact signature may be:
RpcApi.JiraRefreshCommand(
    client: typeof TabRpcClient,
    data: CommandJiraRefreshData,
    opts?: RpcOpts
): Promise<CommandJiraRefreshRtnData>
```

From frontend/types/gotypes.d.ts (generated):
```ts
type CommandJiraRefreshData = {};
type CommandJiraRefreshRtnData = {
    issuecount: number;
    attachmentcount: number;
    commentcount: number;
    elapsedms: number;
    cachepath: string;
    fetchedat: string;
};
```

Existing widget atoms (jiratasks.tsx:229-230):
```ts
loadingAtom: PrimitiveAtom<boolean> = atom(false);
errorAtom:   PrimitiveAtom<string | null> = atom<string | null>(null);
```

Existing helper to reuse after RPC success (jiratasks.tsx:469):
```ts
async loadFromCache(): Promise<void>  // reads cache file and populates issuesAtom
```

Subprocess-spawn helpers to evaluate for deletion (D-UI-04):
- `getCli()` at line 559 — ALSO used by analyzeIssueInNewTerminal (line 566) and analyzeIssueInCurrentTerminal (line 608). DO NOT delete; still needed for analyze flow.
- `resolveTargetTerminal()` at line 579 — ALSO used by analyzeIssueInCurrentTerminal (line 601). DO NOT delete; still needed.
- `stringToBase64` import — used by ControllerInputCommand at line 611 (analyze flow). DO NOT delete.
- `createBlock` import — still used by analyzeIssueInNewTerminal (line 576). DO NOT delete.
- **Conclusion:** After removing the subprocess-spawn code from `requestJiraRefresh` specifically, ALL of those helpers remain in use by the analyze flows. D-UI-04's "grep before deletion" guard applies: confirm each is still referenced elsewhere before attempting removal. In this phase, expect no imports/helpers to be deleted — only the body of `requestJiraRefresh` changes.
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1: Add refreshProgressAtom + rewrite requestJiraRefresh + update tooltip</name>
  <files>frontend/app/view/jiratasks/jiratasks.tsx</files>
  <action>
    1. **Add atom declaration** near the other atoms around line 229-243:
       ```ts
       refreshProgressAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
       ```
       Place it right after `errorAtom` so the group stays cohesive.
    2. **Update the ☁️ button tooltip** at line 368:
       - Before: `title: "Jira에서 새로고침 (Claude에게 요청)",`
       - After:  `title: "Jira에서 새로고침",`
       The "(Claude에게 요청)" suffix is no longer accurate. Keep the icon name `cloud-arrow-down` unchanged.
    3. **Rewrite `requestJiraRefresh()`** (currently lines 381-402). New body per D-UI-01:
       ```ts
       async requestJiraRefresh(): Promise<void> {
           globalStore.set(this.loadingAtom, true);
           globalStore.set(this.errorAtom, null);
           try {
               const rtn = await RpcApi.JiraRefreshCommand(TabRpcClient, {});
               // Success: reload cache so issuesAtom reflects new data, then surface a summary.
               await this.loadFromCache();
               const elapsedSec = (rtn.elapsedms / 1000).toFixed(1);
               const summary = `${rtn.issuecount} 이슈 · ${elapsedSec}s`;
               globalStore.set(this.refreshProgressAtom, summary);
               // Auto-clear after 5s per D-UI-02.
               setTimeout(() => {
                   // Only clear if the current value is still this summary — avoids wiping a newer refresh's summary.
                   if (globalStore.get(this.refreshProgressAtom) === summary) {
                       globalStore.set(this.refreshProgressAtom, null);
                   }
               }, 5000);
           } catch (err: unknown) {
               const msg = err instanceof Error ? err.message : String(err);
               globalStore.set(this.errorAtom, msg);
           } finally {
               globalStore.set(this.loadingAtom, false);
           }
       }
       ```
       Note: `loadFromCache()` sets `loadingAtom=true` at its own entry (line 470) and `loadingAtom=false` in its finally (line 513). That's compatible — our outer try/finally will also set false at the end, but the sequence is still `true → (cache true) → false → false` which is fine. Leave the double-setting as-is to keep the change minimal; the UI only sees the final `false` state.
    4. **Render the progress summary** (D-UI-02). Find the header render block around line 900-930. If there is no existing location for this text, add a small `<span className="jira-refresh-summary">...</span>` adjacent to (not replacing) the error rendering. Exact placement is Claude's discretion — match the existing Tailwind / SCSS class conventions in jiratasks.scss. Use `useAtomValue(model.refreshProgressAtom)` in the component. If the render would require non-trivial layout work, the MINIMUM acceptable change is to add the atom read + conditional render with a class like `"jira-refresh-summary"` and no positioning tweaks (can be refined in Phase 4 polish). Do NOT skip this step — the truth "refreshProgressAtom holds a summary string" requires the value to be observable, and rendering it is the observable path.
    5. **Do NOT delete** `getCli`, `resolveTargetTerminal`, `stringToBase64` import, or `createBlock` import — each is still referenced by the analyze flow per the `<interfaces>` block above. D-UI-04 says "grep usages before deletion"; the grep confirms they are still used.
    6. **Run typecheck**: `npx tsc --noEmit` must pass. This validates that the regenerated RpcApi.JiraRefreshCommand signature matches our call site (e.g. that passing `{}` as `CommandJiraRefreshData` is accepted — if the generator emits a stricter type, adjust the call site accordingly; the generated TS is the contract).
    7. **Search for leftover references** to the old behavior:
       ```bash
       grep -n "Claude에게 요청\\|cli.*jira.*새로고침\\|claude \"jira" frontend/app/view/jiratasks/jiratasks.tsx
       ```
       Should return 0 matches (empty-state CTA text at line 479 and 511 is about "Claude에게 'jira 이슈 새로고침'을 요청하세요" — that's the fallback CTA per D-UI-03 and MUST remain until Phase 4 replaces it with a setup modal. So those two lines are EXPECTED matches; confirm they are still the only matches and no new callers of the subprocess path remain).
    8. Reference the decisions in code comments where behavior is non-obvious:
       - Above refreshProgressAtom declaration: `// D-UI-02: post-hoc refresh summary, auto-clears after 5s.`
       - Above setTimeout callback: `// Guard: don't clear a newer refresh's summary.`
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; npx tsc --noEmit 2>&amp;1 | tail -20 &amp;&amp; grep -c "RpcApi.JiraRefreshCommand" frontend/app/view/jiratasks/jiratasks.tsx &amp;&amp; (! grep -n "Claude에게 요청" frontend/app/view/jiratasks/jiratasks.tsx) &amp;&amp; (! grep -n 'cmd:.*claude.*"jira 이슈 새로고침"' frontend/app/view/jiratasks/jiratasks.tsx)</automated>
  </verify>
  <done>
    - `npx tsc --noEmit` passes.
    - `requestJiraRefresh` body contains `RpcApi.JiraRefreshCommand(TabRpcClient, {})` and NOT `ControllerInputCommand` / `createBlock` / `cli` variables.
    - `refreshProgressAtom` declared and rendered somewhere in the header area.
    - Tooltip no longer mentions Claude.
    - Empty-state CTA Korean strings at lines ~479/511 are UNCHANGED (reserved for Phase 4).
  </done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Task 2: Manual UAT — click ☁️, confirm RPC-driven refresh (ROADMAP Success #2, #3)</name>
  <what-built>
    Plan 01 added the backend RPC + handler. Plan 02 added `wsh jira refresh`. This plan rewired the widget's ☁️ button to call the RPC directly instead of spawning a `claude "..."` subprocess. `refreshProgressAtom` surfaces a post-hoc summary.
  </what-built>
  <how-to-verify>
    Prerequisite: valid `~/.config/waveterm/jira.json` with a working PAT (already present from Phase 1/2 UAT). If not, set it up first.

    **Automated build** (run from F:/Waveterm/waveterm):
    1. `task generate` — ensure TS bindings are current.
    2. `task build:backend` — builds wavesrv + wsh.
    3. `task electron:dev` — starts the dev app.

    **ROADMAP Success #1 (CLI path — owned by Plan 02 but re-verify here E2E):**
    4. Open any terminal block in Waveterm, run `wsh jira refresh`. EXPECTED:
       - Exits 0.
       - Prints one line like `Fetched N issues (X attachments, Y comments) in Z.Zs → /path/to/jira-cache.json`.
       - `~/.config/waveterm/jira-cache.json` mtime updated.

    **ROADMAP Success #2 (widget path — THIS plan):**
    5. Open (or focus) the Jira Tasks widget.
    6. Click the ☁️ (cloud-arrow-down) button in the header. EXPECTED:
       - Spinner appears immediately (loadingAtom true).
       - Within ~3s, spinner disappears and issue list updates with fresh data.
       - A small summary line like `23 이슈 · 1.2s` appears briefly and fades after ~5s.
       - Hovering the ☁️ button shows the tooltip `Jira에서 새로고침` (no "Claude에게 요청" suffix).

    **ROADMAP Success #3 (no Claude subprocess):**
    7. Before clicking ☁️, note the list of terminal blocks in the workspace.
    8. Click ☁️ again.
    9. EXPECTED: NO new terminal block is created. NO `claude` process spawns (verify with Task Manager / `ps` if paranoid).

    **ROADMAP Success #4 (error UX):**
    10. Rename `~/.config/waveterm/jira.json` to `jira.json.bak` temporarily.
    11. Click ☁️. EXPECTED: errorAtom shows exact string `설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요.` (the Plan 01 D-ERR-01 mapping).
    12. Rename back to `jira.json`.
    13. Edit `jira.json` → change `apiToken` to `"BADTOKEN"`.
    14. Click ☁️. EXPECTED: errorAtom shows exact string starting with `인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)`.
    15. Restore the real apiToken.

    If any of steps 4-15 fails, describe which step and what you observed. The planner will open a gap-closure plan.
  </how-to-verify>
  <resume-signal>Type "approved" if all 4 ROADMAP success criteria pass, or describe the specific step number that failed + observed behavior.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| widget → RPC | Widget sends empty CommandJiraRefreshData. No user-controlled input crosses in v1. |
| RPC error → errorAtom | err.message is shown verbatim in the widget UI. Message comes from Plan 01 mapJiraError (already sanitized). |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-03-09 | Information Disclosure | errorAtom rendered in DOM | mitigate | Error strings come from Plan 01 mapJiraError, which sanitizes token-like substrings. Widget does no additional rendering escape beyond React's default text-node escaping (safe). |
| T-03-10 | Tampering | refreshProgressAtom string | accept | Derived entirely from server-returned ints. No user input. React escapes text content. |
| T-03-11 | Cross-Site Scripting | rendering err.message | mitigate | Render via JSX text child (`{errorMsg}`), NEVER `dangerouslySetInnerHTML`. Inspect the existing error render site before editing — if it uses dangerouslySetInnerHTML today (unlikely), fix it as part of this plan. |
</threat_model>

<verification>
1. `npx tsc --noEmit` passes.
2. Static: `grep "ControllerInputCommand\\|createBlock" jiratasks.tsx` shows ONLY analyze-flow usages (lines ~576, 611), NOT inside requestJiraRefresh.
3. Static: `grep "RpcApi.JiraRefreshCommand" jiratasks.tsx` → at least one match.
4. Manual UAT (Task 2 checkpoint) covers ROADMAP Success #1-4.
</verification>

<success_criteria>
- Widget ☁️ button triggers the backend RPC (no subprocess).
- Refresh completes in-process, loadFromCache reloads the issue list, refreshProgressAtom shows a summary.
- Errors surface to errorAtom with the exact Korean strings from D-ERR-01.
- ROADMAP Success #1-4 for Phase 3 all verified via the checkpoint.
</success_criteria>

<output>
After completion, create `.planning/phases/03-wsh-rpc-widget-wireup/03-03-SUMMARY.md` summarizing: lines changed in jiratasks.tsx, where refreshProgressAtom was rendered, checkpoint UAT outcome (which of Success #1-4 were verified by the human), and any deferred polish.
</output>

# Phase 3: wsh RPC + Widget Wire-up - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning
**Mode:** Auto-generated — decisions derived from existing wshrpc patterns, widget code, and ROADMAP goal

<domain>
## Phase Boundary

Expose Phase 2's `jira.Refresh()` as a wsh RPC command and wire the Jira widget's ☁️ button to invoke it directly instead of spawning a `claude "..."` subprocess.

- **Backend:** New RPC method in `WshRpcInterface` + handler that calls `jira.LoadConfig()` → `jira.NewClient()` → `jira.Refresh()`.
- **CLI:** `wsh jira refresh` subcommand using the cobra pattern from `wshcmd-ai.go`.
- **Frontend:** Replace `requestJiraRefresh()` body in `frontend/app/view/jiratasks/jiratasks.tsx`. Add `refreshProgressAtom`. Keep existing `loadingAtom` + `errorAtom`.
- **TS regeneration:** Run `task generate` to regenerate `frontend/types/gotypes.d.ts` + RPC client bindings after editing `wshrpctypes.go`.

**Out of scope this phase:** Setup/first-run UX (Phase 4), on-demand downloads (Phase 5), rate limiting/retry (Phase 5).

</domain>

<decisions>
## Implementation Decisions

### RPC Surface (D-RPC-01 .. D-RPC-05)

- **D-RPC-01**: Method name — `JiraRefreshCommand(ctx context.Context, data CommandJiraRefreshData) (CommandJiraRefreshRtnData, error)`. Follows existing `*Command` convention in `WshRpcInterface`.
- **D-RPC-02**: Request shape — `CommandJiraRefreshData` empty struct for now (refresh uses config from disk, no args). Future: optional `Force bool`, `JqlOverride string` — deferred.
- **D-RPC-03**: Response shape — `CommandJiraRefreshRtnData { IssueCount int; AttachmentCount int; CommentCount int; ElapsedMs int64; CachePath string; FetchedAt string }`. Mirrors `jira.RefreshReport` but JSON-serializable (`time.Duration` → ms).
- **D-RPC-04**: Progress — **synchronous RPC for v1** (no streaming). The widget sets `loadingAtom=true` before the call, clears after. Rationale: avoid stream complexity + keeps widget code minimal. Progress info surfaces post-hoc via the return value; mid-call progress = simple spinner only. Streaming upgrade path deferred to a follow-up phase if UX requires it.
- **D-RPC-05**: Handler location — `pkg/wshrpc/wshserver/wshserver.go` (or a new `wshserver-jira.go` sibling file if the planner prefers). Handler imports `pkg/jira`.

### Error Classification (D-ERR-01 .. D-ERR-04)

- **D-ERR-01**: Error types returned by handler must be user-actionable strings that widget can show verbatim. Wrap `jira.ErrUnauthorized` → `"인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)"`. Wrap `jira.ErrConfigNotFound` → `"설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."`. Wrap network errors → `"Jira 서버에 연결할 수 없습니다: {err}"`. Other errors → `fmt.Errorf("refresh failed: %v", err)` (no body/token leakage).
- **D-ERR-02**: Handler must NOT log the auth token or the raw APIError.Body (T-01-02 threat guard already in place — don't introduce new leaks).
- **D-ERR-03**: Widget `errorAtom` is set to the raw error.Error() string from the RPC; no additional processing.
- **D-ERR-04**: `wsh jira refresh` CLI maps errors to exit codes: 0=success, 1=auth (mapped from `ErrUnauthorized`), 2=config missing, 3=other. Printed to stderr.

### CLI Surface (D-CLI-01 .. D-CLI-03)

- **D-CLI-01**: Command structure — `wsh jira refresh`. `jira` is a parent cobra.Command with `refresh` as the only subcommand this phase. Phase 5 will add `jira download`.
- **D-CLI-02**: Output format — human-readable stdout on success: `Fetched 23 issues (4 attachments, 107 comments) in 1.2s → ~/.config/waveterm/jira-cache.json`. Use `--json` flag to emit the `CommandJiraRefreshRtnData` as JSON for scripting.
- **D-CLI-03**: Single file — `cmd/wsh/cmd/wshcmd-jira.go`. Register `jiraCmd` on `rootCmd`; register `jiraRefreshCmd` as subcommand of `jiraCmd`.

### Widget Changes (D-UI-01 .. D-UI-04)

- **D-UI-01**: `requestJiraRefresh()` replaced — delete `resolveTargetTerminal`/`getCli`/subprocess-spawn logic. New body:
  1. `globalStore.set(this.loadingAtom, true)` + clear `errorAtom`
  2. `try { result = await RpcApi.JiraRefreshCommand(TabRpcClient, {}) } catch (err) { errorAtom = err.message }`
  3. On success, call `await this.loadFromCache()` to reload.
  4. `finally { loadingAtom = false }`
- **D-UI-02**: Add `refreshProgressAtom: atom<string|null>` for post-hoc summary display (e.g., "23 이슈 · 1.2s"). Not mid-call progress. Fades after 5s via `setTimeout`.
- **D-UI-03**: Empty state update — if `errorAtom` mentions "설정 파일이 없습니다" or "인증 실패", keep the existing Claude-prompt CTA as fallback (Phase 4 will replace with a proper setup modal).
- **D-UI-04**: Remove unused imports after delete: `getCli`, `resolveTargetTerminal`, subprocess-spawn helpers if only used here. Grep usages before deletion.

### TS Regeneration (D-TS-01 .. D-TS-02)

- **D-TS-01**: After editing `wshrpctypes.go`, run `task generate` (or the appropriate Taskfile target that produces `frontend/types/gotypes.d.ts` + `wshclientapi.ts`). Include the regenerated files in the commit.
- **D-TS-02**: If `task generate` is not available in the Windows dev environment, the planner MUST identify the underlying command the task runs (inspect `Taskfile.yml`) and run it directly. Fall back: the Go type generator is typically `pkg/wshrpc/wshrpctypes_builder.go` or a `go run` one-liner — check the repo root Taskfile.

### Testing (D-TEST-01 .. D-TEST-03)

- **D-TEST-01**: Backend — unit test for `JiraRefreshCommand` handler in `pkg/wshrpc/wshserver/` that mocks `jira.Refresh()` via dependency injection, asserts error-class mapping (ErrUnauthorized → human message prefix).
- **D-TEST-02**: CLI — integration test runs `wsh jira refresh --help` and asserts usage output. Full refresh path requires a live Jira OR fixture server — deferred to manual UAT.
- **D-TEST-03**: Widget — no new unit tests required (TS has no test harness active in this widget). Manual UAT: click ☁️ → see spinner → see refreshed issues. ROADMAP Success #2 is this test.

### Claude's Discretion

- Whether to split server handler into its own file (`wshserver-jira.go`) or inline in existing file
- Exact cobra flag names (`--json` vs `-j`)
- Widget progress string formatting nuance
- Whether to add a `--timeout` flag to `wsh jira refresh` (default 60s reasonable)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `pkg/jira/refresh.go` — `Refresh(ctx, RefreshOpts) (*RefreshReport, error)` ready to call
- `pkg/jira/config.go` — `LoadConfig()` for default path
- `pkg/jira/client.go` — `NewClient(cfg)` or `NewClientWithHTTP(cfg, http.DefaultClient)`
- `pkg/wshrpc/wshrpctypes.go:37` — `WshRpcInterface` with examples like `BadgeCreateCommand`, `BlocksListCommand`
- `cmd/wsh/cmd/wshcmd-ai.go` + `wshcmd-badge.go` — cobra.Command templates
- Widget `loadFromCache()` at `jiratasks.tsx:470` — call after successful refresh
- Widget `errorAtom` + `loadingAtom` already exist and are rendered

### Established Patterns
- RPC method naming: `{Noun}{Verb}Command` (e.g., `BadgeCreateCommand`). We use `JiraRefreshCommand`.
- Handler wiring: register in `pkg/wshrpc/wshserver/wshserver.go` (WshServer struct method)
- Command pattern: `PreRunE: preRunSetupRpcClient` for RPC-using wsh commands
- Telemetry: `sendActivity("jira-refresh", rtnErr == nil)` on CLI exit (optional; inspect `wshcmd-ai.go` pattern)

### Integration Points
- `WshRpcInterface` method addition → requires `task generate`
- `TabRpcClient` in frontend (`frontend/app/store/wshrpc.ts` or similar) — RpcApi client
- `cmd/wsh/cmd/` — cobra subcommand registration

</code_context>

<specifics>
## Specific Ideas

- Error message Korean text is locked by REQUIREMENTS JIRA-04: "인증 실패 — PAT 재발급 필요". The widget already expects this exact phrasing — reuse.
- The widget currently calls `loadFromCache()` on mount via `errorAtom` empty-state CTA. Keep that flow intact; `requestJiraRefresh()` just adds a new happy path.
- Unused helper cleanup: after D-UI-04, grep `jiratasks.tsx` for `getCli`, `resolveTargetTerminal`, `TabRpcClient`-based subprocess calls to find dead code.

</specifics>

<deferred>
## Deferred Ideas

- **Streaming progress** — mid-refresh `(current/total)` progress via `chan RespOrErrorUnion[...]`. Deferred to evaluate post-UAT. v1 is sync with post-hoc summary.
- **Force flag / JQL override via RPC** — Phase 5 or later.
- **Setup modal** in widget (replaces Claude-prompt CTA) — Phase 4 (JIRA-04).
- **Rate limiting + retry** inside handler — Phase 5 (JIRA-10).
- **Background auto-refresh** — already has `refreshIntervalAtom`; wiring to use the new RPC instead of re-reading cache is a nice-to-have deferred unless needed for UAT.

</deferred>

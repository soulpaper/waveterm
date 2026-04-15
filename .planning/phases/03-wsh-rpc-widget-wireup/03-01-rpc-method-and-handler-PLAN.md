---
phase: 03-wsh-rpc-widget-wireup
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - pkg/wshrpc/wshrpctypes.go
  - pkg/wshrpc/wshserver/wshserver-jira.go
  - pkg/wshrpc/wshserver/wshserver-jira_test.go
  - pkg/wshrpc/wshclient/wshclient.go
  - frontend/app/store/wshclientapi.ts
  - frontend/types/gotypes.d.ts
autonomous: true
requirements:
  - JIRA-03
  - JIRA-08
must_haves:
  truths:
    - "WshRpcInterface has JiraRefreshCommand method declared"
    - "WshServer.JiraRefreshCommand handler calls jira.LoadConfig + jira.Refresh and returns a populated CommandJiraRefreshRtnData on success"
    - "Handler maps jira.ErrUnauthorized to '인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)'"
    - "Handler maps jira.ErrConfigNotFound to '설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요.'"
    - "Handler never logs APIError.Body or Config.ApiToken (T-01-02)"
    - "Regenerated wshclientapi.ts exposes RpcApi.JiraRefreshCommand"
    - "Regenerated gotypes.d.ts exposes CommandJiraRefreshData and CommandJiraRefreshRtnData"
  artifacts:
    - path: "pkg/wshrpc/wshrpctypes.go"
      provides: "JiraRefreshCommand interface method + request/response types"
      contains: "JiraRefreshCommand"
    - path: "pkg/wshrpc/wshserver/wshserver-jira.go"
      provides: "WshServer.JiraRefreshCommand handler + error-mapping helper"
      contains: "func (ws *WshServer) JiraRefreshCommand"
    - path: "pkg/wshrpc/wshserver/wshserver-jira_test.go"
      provides: "unit tests for error-class mapping (ErrUnauthorized/ErrConfigNotFound/ErrConfigIncomplete/generic)"
      contains: "TestJiraRefreshCommand"
    - path: "frontend/app/store/wshclientapi.ts"
      provides: "generated RpcApi.JiraRefreshCommand client binding"
      contains: "JiraRefreshCommand"
    - path: "frontend/types/gotypes.d.ts"
      provides: "generated TS types for request/response"
      contains: "CommandJiraRefreshRtnData"
  key_links:
    - from: "pkg/wshrpc/wshserver/wshserver-jira.go"
      to: "pkg/jira.Refresh"
      via: "direct package call after jira.LoadConfig"
      pattern: "jira\\.Refresh\\("
    - from: "pkg/wshrpc/wshserver/wshserver-jira.go"
      to: "pkg/jira error sentinels"
      via: "errors.Is dispatch on ErrUnauthorized / ErrConfigNotFound / ErrConfigIncomplete"
      pattern: "errors\\.Is\\(err, jira\\.Err"
    - from: "frontend/app/store/wshclientapi.ts"
      to: "pkg/wshrpc/wshrpctypes.go"
      via: "cmd/generatets output"
      pattern: "JiraRefreshCommand"
---

<objective>
Add the `JiraRefreshCommand` RPC to the Wave backend and regenerate the TypeScript bindings so downstream plans (CLI and widget) can consume the typed client.

Purpose: Implements REQ JIRA-03 (widget refresh via Wave backend) and JIRA-08 (same flow via `wsh`) at the RPC layer. Handler is the single entry point both consumers will call.

Output: Go interface method + data types, server-side handler with error-class mapping (Korean user-facing strings per D-ERR-01), unit test covering the mapping, and regenerated TS bindings committed alongside the Go changes.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/PROJECT.md
@.planning/ROADMAP.md
@.planning/phases/03-wsh-rpc-widget-wireup/03-CONTEXT.md
@pkg/jira/refresh.go
@pkg/jira/config.go
@pkg/jira/errors.go
@pkg/wshrpc/wshrpctypes.go
@pkg/wshrpc/wshserver/wshserver.go
@Taskfile.yml

<interfaces>
<!-- Contracts the executor needs. Extracted from codebase. -->

From pkg/jira/refresh.go:
```go
type RefreshOpts struct {
    Config     Config
    HTTPClient *http.Client
    OnProgress func(stage string, current, total int)
}

type RefreshReport struct {
    IssueCount      int
    AttachmentCount int
    CommentCount    int
    Elapsed         time.Duration
    CachePath       string
}

func Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error)
```

From pkg/jira/config.go:
```go
type Config struct {
    BaseUrl, CloudId, Email, ApiToken, Jql string
    PageSize                                int
}
func LoadConfig() (Config, error)
```

From pkg/jira/errors.go:
```go
var (
    ErrUnauthorized    = errors.New("jira: unauthorized")     // wrapped by APIError for 401
    ErrForbidden       = errors.New("jira: forbidden")
    ErrNotFound        = errors.New("jira: not found")
    ErrRateLimited     = errors.New("jira: rate limited")
    ErrServerError     = errors.New("jira: server error")
    ErrConfigNotFound  = errors.New("jira: config not found")
    ErrConfigInvalid   = errors.New("jira: config invalid")
    ErrConfigIncomplete = errors.New("jira: config incomplete")
)
// APIError has Body string — DO NOT log or surface.
```

From pkg/wshrpc/wshrpctypes.go (existing patterns):
```go
// Method naming: {Noun}{Verb}Command. Register in WshRpcInterface.
// Request shape convention:  CommandXxxData
// Response shape convention: CommandXxxRtnData
// After edit: run `task generate` (invokes cmd/generatets + cmd/generatego).
```

From Taskfile.yml:
```yaml
generate:
    cmds:
        - go run cmd/generatets/main-generatets.go
        - go run cmd/generatego/main-generatego.go
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Declare RPC types + interface method, write failing handler test (Nyquist RED)</name>
  <files>pkg/wshrpc/wshrpctypes.go, pkg/wshrpc/wshserver/wshserver-jira_test.go</files>
  <behavior>
    - `WshRpcInterface` grows one method: `JiraRefreshCommand(ctx context.Context, data CommandJiraRefreshData) (CommandJiraRefreshRtnData, error)` (per D-RPC-01).
    - New exported types alongside existing `Command*Data` patterns (near BlocksListRequest/BlocksListEntry at line ~527):
      * `CommandJiraRefreshData struct {}` — empty for v1 (D-RPC-02). Leave a `// Reserved for future Force/JqlOverride — see CONTEXT D-RPC-02` comment so future additions land obviously.
      * `CommandJiraRefreshRtnData struct { IssueCount int; AttachmentCount int; CommentCount int; ElapsedMs int64; CachePath string; FetchedAt string }` with json tags `issuecount`, `attachmentcount`, `commentcount`, `elapsedms`, `cachepath`, `fetchedat` (matches existing snake-less lowercased convention used in this file — confirm against BlocksListEntry before writing) (D-RPC-03).
    - Test file `pkg/wshrpc/wshserver/wshserver-jira_test.go` exercises the (not-yet-written) handler through a seam. Because the real handler will call `jira.LoadConfig()` + `jira.Refresh()`, introduce a package-level indirection in the handler source for test override:
      ```go
      // In wshserver-jira.go (created in Task 2):
      var jiraLoadConfig = jira.LoadConfig
      var jiraRefresh = jira.Refresh
      ```
      The test swaps these via `t.Cleanup`-restored assignments. Test cases (all in one `TestJiraRefreshCommand` with table subtests):
      * `success` — LoadConfig returns a valid Config, Refresh returns a populated *RefreshReport. Expect: no error, returned `CommandJiraRefreshRtnData` mirrors the report, `ElapsedMs == report.Elapsed.Milliseconds()`, `FetchedAt` is an RFC3339 string within 2s of time.Now() at call start.
      * `config_not_found` — LoadConfig returns `jira.ErrConfigNotFound`. Expect: error whose `.Error()` equals `"설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."` (exact D-ERR-01 string). Return struct is zero-valued.
      * `config_incomplete` — LoadConfig returns `jira.ErrConfigIncomplete` wrapped with missing-field context. Expect: error whose message starts with the same "설정 파일이 없습니다" prefix (treat incomplete config identically to missing for widget UX — document in code comment that Phase 4 will split these).
      * `unauthorized` — LoadConfig OK; Refresh returns `fmt.Errorf("%w", jira.ErrUnauthorized)` (chain through `%w`). Expect: error with exact string `"인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)"`.
      * `rate_limited` — Refresh returns `jira.ErrRateLimited`. Expect: error matching pattern `"Jira 서버가 요청을 제한했습니다.*잠시 후 다시 시도"` (Claude's discretion on exact phrasing — just ensure it mentions rate-limit and is in Korean; document in test the exact string chosen).
      * `network` — Refresh returns a `*net.OpError` or any non-APIError error. Expect: error whose `.Error()` has prefix `"Jira 서버에 연결할 수 없습니다:"` and does NOT contain the word "token" or "apiToken".
      * `generic` — Refresh returns `errors.New("disk full")`. Expect: error whose `.Error()` starts with `"refresh failed:"` and contains `"disk full"`; MUST NOT contain the substring `apiToken` or the raw token value.
    - Test also verifies T-01-02 defensively: inject a Config with `ApiToken: "SECRET_TOKEN_12345"` in the `success` case, capture stderr+stdout via `t.TempDir`/log redirect, assert `"SECRET_TOKEN_12345"` never appears in captured output.
    - Test file imports: `testing`, `context`, `errors`, `fmt`, `strings`, `time`, `github.com/wavetermdev/waveterm/pkg/jira`, `github.com/wavetermdev/waveterm/pkg/wshrpc`. Place in package `wshserver` (not `wshserver_test`) so it can access the package-level `jiraLoadConfig`/`jiraRefresh` vars.
    - Running `go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand` MUST fail to compile (handler + seam vars don't exist yet). This is the intentional RED state per D-TEST-01.
  </behavior>
  <action>
    1. Open `pkg/wshrpc/wshrpctypes.go`. In the `WshRpcInterface` block (around the BlocksListCommand region near line 84 — group with other block/info commands OR add a new `// jira` comment block if the planner prefers semantic grouping), insert:
       ```go
       JiraRefreshCommand(ctx context.Context, data CommandJiraRefreshData) (CommandJiraRefreshRtnData, error)
       ```
       Claude's discretion (D-RPC-05 note) on exact position — prefer a new `// jira` section comment to keep it discoverable.
    2. In the types section of the same file (near BlocksListRequest around line 527), add:
       ```go
       type CommandJiraRefreshData struct {
           // Reserved for future Force / JqlOverride — see CONTEXT D-RPC-02.
       }

       type CommandJiraRefreshRtnData struct {
           IssueCount      int    `json:"issuecount"`
           AttachmentCount int    `json:"attachmentcount"`
           CommentCount    int    `json:"commentcount"`
           ElapsedMs       int64  `json:"elapsedms"`
           CachePath       string `json:"cachepath"`
           FetchedAt       string `json:"fetchedat"` // RFC3339
       }
       ```
       Verify lowercased-nosnake json tags match the file's local convention by grepping existing `BlocksListEntry` tags.
    3. Create `pkg/wshrpc/wshserver/wshserver-jira_test.go` with the table-driven test described in `<behavior>` above. Implement each subtest as a stand-alone closure that restores the package-level seams in `t.Cleanup`. Use `strings.Contains` for substring assertions and `==` for exact-match expectations (D-ERR-01 strings are exact).
    4. Do NOT create `wshserver-jira.go` yet — Task 2 creates it. Verify `go build ./pkg/wshrpc/...` FAILS with "undefined: jiraLoadConfig" (or similar) — this is the RED state.
    5. Do NOT run `task generate` yet — Task 3 does that after the handler compiles.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm && go vet ./pkg/wshrpc/ 2>&amp;1 | head -30 &amp;&amp; (go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand 2>&amp;1 | tee /tmp/wave1-red.log; grep -qE "undefined: jira(LoadConfig|Refresh)|CommandJiraRefresh" /tmp/wave1-red.log &amp;&amp; echo "RED-OK" || (echo "RED-FAIL: expected compile error referencing missing handler seams" &amp;&amp; exit 1))</automated>
  </verify>
  <done>
    - `wshrpctypes.go` compiles on its own (`go vet ./pkg/wshrpc/` passes); interface method + both data types are exported.
    - `wshserver-jira_test.go` exists and references `jiraLoadConfig` / `jiraRefresh` package seams.
    - `go test ./pkg/wshrpc/wshserver/` fails with a compile error about the missing seams (Nyquist RED confirmed).
  </done>
</task>

<task type="auto" tdd="true">
  <name>Task 2: Implement WshServer.JiraRefreshCommand handler + error-mapping helper (GREEN)</name>
  <files>pkg/wshrpc/wshserver/wshserver-jira.go</files>
  <behavior>
    - After this task, `go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand` passes all subtests from Task 1.
    - Handler signature exactly matches the interface (Task 1 enforces this at compile time; WshServer otherwise fails to implement WshRpcInterface).
    - Error mapping helper is a private function `mapJiraError(err error) error` that uses `errors.Is` dispatch per D-ERR-01. Token/body are never included in any returned message.
    - `panichandler.PanicHandler("JiraRefreshCommand", recover())` deferred at top of handler (matches convention in existing WshServer methods).
    - Activity telemetry: emit `sendActivity("jira-refresh", rtnErr == nil)` equivalent if the server has an analogue; otherwise omit (Claude's discretion — server-side handlers in wshserver.go don't use sendActivity; CLI plan handles it). Default: omit here.
  </behavior>
  <action>
    1. Create `pkg/wshrpc/wshserver/wshserver-jira.go` with copyright header matching siblings (`// Copyright 2026, Command Line Inc.` — look at wshcmd-badge.go for the 2026 precedent) and `package wshserver`.
    2. Imports: `context`, `errors`, `fmt`, `time`, `github.com/wavetermdev/waveterm/pkg/jira`, `github.com/wavetermdev/waveterm/pkg/panichandler`, `github.com/wavetermdev/waveterm/pkg/wshrpc`.
    3. Declare package-level seams (for test override):
       ```go
       // jiraLoadConfig and jiraRefresh are overridable seams for unit tests.
       // Production code assigns them to the real pkg/jira implementations.
       var jiraLoadConfig = jira.LoadConfig
       var jiraRefresh    = jira.Refresh
       ```
    4. Implement the handler:
       ```go
       func (ws *WshServer) JiraRefreshCommand(ctx context.Context, data wshrpc.CommandJiraRefreshData) (wshrpc.CommandJiraRefreshRtnData, error) {
           defer func() {
               panichandler.PanicHandler("JiraRefreshCommand", recover())
           }()
           started := time.Now()
           cfg, err := jiraLoadConfig()
           if err != nil {
               return wshrpc.CommandJiraRefreshRtnData{}, mapJiraError(err)
           }
           report, err := jiraRefresh(ctx, jira.RefreshOpts{Config: cfg})
           if err != nil {
               return wshrpc.CommandJiraRefreshRtnData{}, mapJiraError(err)
           }
           return wshrpc.CommandJiraRefreshRtnData{
               IssueCount:      report.IssueCount,
               AttachmentCount: report.AttachmentCount,
               CommentCount:    report.CommentCount,
               ElapsedMs:       report.Elapsed.Milliseconds(),
               CachePath:       report.CachePath,
               FetchedAt:       started.UTC().Format(time.RFC3339),
           }, nil
       }
       ```
    5. Implement `mapJiraError` per D-ERR-01 exactly:
       ```go
       func mapJiraError(err error) error {
           switch {
           case errors.Is(err, jira.ErrConfigNotFound), errors.Is(err, jira.ErrConfigIncomplete):
               return fmt.Errorf("설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요.")
           case errors.Is(err, jira.ErrUnauthorized):
               return fmt.Errorf("인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)")
           case errors.Is(err, jira.ErrRateLimited):
               return fmt.Errorf("Jira 서버가 요청을 제한했습니다. 잠시 후 다시 시도하세요.")
           case isNetworkError(err):
               return fmt.Errorf("Jira 서버에 연결할 수 없습니다: %v", sanitizeErrMessage(err))
           default:
               return fmt.Errorf("refresh failed: %v", sanitizeErrMessage(err))
           }
       }
       ```
       - `isNetworkError(err error) bool` — use `errors.As` on `*net.OpError`, `*url.Error` (import `net`, `net/url`). Also treat `strings.Contains(err.Error(), "dial tcp")` and `"i/o timeout"` as fallbacks. Return false for `*jira.APIError` (APIError is categorized by status code, not transport).
       - `sanitizeErrMessage(err error) string` — returns `err.Error()` BUT if the result contains any character that looks like an API token (heuristic: substring of length >= 20 matching `[A-Za-z0-9_=+/-]{20,}`), replace the matched region with `"<redacted>"`. Use `regexp.MustCompile` at package init. This defends against future code paths that accidentally wrap a token; APIError.Error() already omits Body per T-01-02-01 but we defend in depth.
    6. Add a top-of-file comment referencing `CONTEXT D-ERR-01..04` and `T-01-02` so future editors don't remove the sanitization by accident.
    7. Do NOT log the config, the raw error body, or any token-like substring. Never call `log.Printf` with `err` — if logging is desired, log only `mapJiraError(err).Error()` (the already-sanitized form). Prefer no logging in the handler; the caller sees the mapped error.
    8. Add NEW imports to Task 1 test file if needed: if `net` / `net/url` are needed to construct the `network` test case, add them there too. Construct the network error in the test as `&net.OpError{Op: "dial", Err: errors.New("connection refused")}` so `isNetworkError` matches.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go build ./pkg/wshrpc/... &amp;&amp; go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand -v 2>&amp;1 | tail -40</automated>
  </verify>
  <done>
    - `go build ./pkg/wshrpc/...` succeeds (WshServer now satisfies the expanded WshRpcInterface).
    - All TestJiraRefreshCommand subtests from Task 1 pass (GREEN).
    - No test captures contain the literal `"SECRET_TOKEN_12345"` string.
    - Handler + helper are <100 LOC combined.
  </done>
</task>

<task type="auto">
  <name>Task 3: Regenerate TS bindings via `task generate` and verify generated artifacts</name>
  <files>pkg/wshrpc/wshclient/wshclient.go, frontend/app/store/wshclientapi.ts, frontend/types/gotypes.d.ts</files>
  <action>
    1. Run `task generate` from the repo root (this invokes `go run cmd/generatets/main-generatets.go` then `go run cmd/generatego/main-generatego.go` per Taskfile.yml lines 360-371). On Windows where `task` CLI may be absent, fall back to running the two `go run` commands directly in the same order. Note: `task generate` has `deps: [build:schema]` — schema may rebuild first; let it.
    2. After regen, git-diff these files and CONFIRM:
       - `pkg/wshrpc/wshclient/wshclient.go` gained a `JiraRefreshCommand(w *wshutil.WshRpc, data wshrpc.CommandJiraRefreshData, opts *wshrpc.RpcOpts) (wshrpc.CommandJiraRefreshRtnData, error)` function matching the convention of neighboring generated functions.
       - `frontend/app/store/wshclientapi.ts` gained `JiraRefreshCommand` on the `RpcApi` object (or equivalent — inspect how existing methods like `BlocksListCommand` are shaped and confirm Jira follows the same pattern).
       - `frontend/types/gotypes.d.ts` gained `CommandJiraRefreshData` and `CommandJiraRefreshRtnData` type declarations with fields matching the Go json tags (issuecount, attachmentcount, commentcount, elapsedms, cachepath, fetchedat).
    3. If ANY of the three generated artifacts is missing the new symbol, STOP and investigate — do NOT hand-edit generated files (they will be clobbered on next regen). Most likely cause: wrong json tag convention or missing export in wshrpctypes.go. Fix the source, rerun `task generate`, re-check.
    4. Do a full typecheck pass to confirm nothing upstream broke: `npx tsc --noEmit` from repo root (or `task check:ts`) — must exit 0.
    5. Do a full backend build to confirm the generated Go client compiles cleanly: `go build ./...` from repo root — must exit 0.
    6. DO NOT commit — the orchestrator handles commits. But stage nothing, don't `git add`.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; grep -q "JiraRefreshCommand" pkg/wshrpc/wshclient/wshclient.go &amp;&amp; grep -q "JiraRefreshCommand" frontend/app/store/wshclientapi.ts &amp;&amp; grep -q "CommandJiraRefreshRtnData" frontend/types/gotypes.d.ts &amp;&amp; go build ./... &amp;&amp; npx tsc --noEmit</automated>
  </verify>
  <done>
    - All three generated files contain the new symbols.
    - `go build ./...` passes.
    - `npx tsc --noEmit` passes.
    - Unit tests from Task 2 still pass (`go test ./pkg/wshrpc/wshserver/`).
  </done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| frontend → wshserver (RPC) | Widget (trusted UI but could be misrouted) invokes JiraRefreshCommand. No user input flows through CommandJiraRefreshData in v1 (empty struct), so injection surface is zero today. |
| wshserver → pkg/jira | Handler calls LoadConfig (reads apiToken from disk) and Refresh (HTTP egress to kakaovx.atlassian.net). |
| handler error → RPC client | Handler error messages cross back to widget + CLI. These are surfaced verbatim to user. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-03-01 | Information Disclosure | mapJiraError / sanitizeErrMessage | mitigate | Never wrap `*jira.APIError` directly in a user-facing message; pass through sanitizeErrMessage which redacts 20+ char token-like substrings. Unit test asserts "SECRET_TOKEN_12345" never appears in any error message. |
| T-03-02 | Information Disclosure | handler logging | mitigate | Do not call `log.Printf(err)` on the raw error. Only log the mapped (sanitized) message if logging is ever added. Code comment references T-01-02. |
| T-03-03 | Tampering | CommandJiraRefreshData expansion | accept | Struct is empty in v1; when Phase 5 adds JqlOverride, a new threat row must be added covering JQL injection (Atlassian validates JQL server-side, but we still treat it as untrusted input for logging). |
| T-03-04 | Denial of Service | unbounded Refresh duration | accept | Handler passes the caller's ctx through unchanged; widget + CLI are responsible for timeouts (CLI plan sets 60s default). Adding a server-side default timeout is deferred to Phase 5. |
| T-03-05 | Elevation of Privilege | regenerated wshclient.go | mitigate | Generated file is machine-written from wshrpctypes.go; reviewers don't edit it by hand. Verify the regen output in Task 3 so a compromised generator can't silently downgrade the handler signature. |
</threat_model>

<verification>
1. `go vet ./...` passes.
2. `go build ./...` passes.
3. `go test ./pkg/wshrpc/wshserver/ -run TestJiraRefreshCommand -v` — all subtests pass.
4. `go test ./pkg/jira/...` still passes (no regression from generator touching types).
5. `npx tsc --noEmit` passes.
6. `grep -r "SECRET_TOKEN\|apiToken.*[A-Za-z0-9]\{20,\}" pkg/wshrpc/` — returns no matches (sanity).
</verification>

<success_criteria>
- `WshServer.JiraRefreshCommand` exists, satisfies the interface, and handles error classification per D-ERR-01 (Korean strings exact).
- Generated TS client (`RpcApi.JiraRefreshCommand`) and types (`CommandJiraRefreshRtnData`) are present in the checked-in-generated files.
- Nyquist RED→GREEN cycle observable in git history: Task 1 produces failing compile; Task 2 produces passing tests; Task 3 produces regenerated artifacts that also compile.
- No test or handler path leaks the API token value in any captured output.
</success_criteria>

<output>
After completion, create `.planning/phases/03-wsh-rpc-widget-wireup/03-01-SUMMARY.md` summarizing: RPC method signature added, exact error-mapping strings used, generator command that was run, and any deviations from D-RPC-03 json tags.
</output>

---
phase: 03-wsh-rpc-widget-wireup
plan: 02
subsystem: wsh-cli
tags: [cli, cobra, jira, rpc-consumer]
requires:
  - "03-01 (JiraRefreshCommand RPC method + handler + regenerated wshclient bindings)"
provides:
  - "wsh jira refresh subcommand wired to wshclient.JiraRefreshCommand"
  - "Human-readable summary output + --json scripting mode"
  - "D-ERR-04 exit-code mapping (0/1/2/3) as pure, independently testable helper"
  - "sendActivity(\"jira-refresh\", success) telemetry hook"
affects:
  - "cmd/wsh/cmd/ (new jira parent + refresh subcommand registered on rootCmd)"
tech-stack:
  added:
    - "(none — reuses existing cobra + wshrpc + wshclient + wshutil)"
  patterns:
    - "Cobra parent+subcommand via cobra.Command.AddCommand"
    - "PreRunE: preRunSetupRpcClient to obtain RpcClient before RPC"
    - "WshExitCode pattern (no os.Exit) for non-zero exits"
    - "TDD RED → GREEN via pure helper extraction (exitCodeForError, formatRefreshSummary)"
key-files:
  created:
    - cmd/wsh/cmd/wshcmd-jira.go
    - cmd/wsh/cmd/wshcmd-jira_test.go
  modified:
    - "(none)"
decisions:
  - "Used WshExitCode package variable (already present, see wshcmd-getvar.go) instead of os.Exit — matches established convention. No exitCodeError sentinel exists in cmd/wsh."
  - "Empty Route when WAVETERM_TABID is unset (refresh works from standalone shells) — documented inline. Diverges from wshcmd-ai.go which errors out in that case."
  - "Tested help via cmd.Help() directly (not Execute()) because subcommand Execute() routes through the root and prints the root's help."
metrics:
  duration: "~15 minutes"
  tasks_completed: 1
  commits: 2
  tests_added: 5
completed: 2026-04-15
---

# Phase 03 Plan 02: wsh jira CLI Summary

Added `wsh jira refresh` so engineers can trigger the Phase 2 cache refresh flow from any terminal (or script) without going through the widget. Consumes the `JiraRefreshCommand` RPC introduced in Plan 03-01.

## One-liner

Cobra subcommand `wsh jira refresh` that calls `wshclient.JiraRefreshCommand`, prints either a human-readable summary or JSON, and exits 0/1/2/3 per D-ERR-04.

## What Was Built

### `cmd/wsh/cmd/wshcmd-jira.go`

- **`jiraCmd`** — parent `cobra.Command` (`Use: "jira"`), no `RunE`. Cobra auto-prints help on bare invocation.
- **`jiraRefreshCmd`** — subcommand (`Use: "refresh"`), `PreRunE: preRunSetupRpcClient`, `RunE: jiraRefreshRun`.
- Flags:
  - `--json` (bool, default false) — emits `CommandJiraRefreshRtnData` as 2-space-indented JSON.
  - `--timeout` (int, default 60) — RPC timeout in seconds, converted to ms for `RpcOpts.Timeout`.
- Route selection: `WAVETERM_TABID` → `wshutil.MakeTabRouteId(tabId)`; empty → default wavesrv route.
- On RPC error: prints `err.Error()` to stderr, sets `WshExitCode`, returns nil (cobra doesn't print its own error line).
- Pure helpers extracted for unit testability:
  - `exitCodeForError(err error) int` — prefix-match on the Korean mapped-error strings from Plan 03-01's `mapJiraError`.
  - `formatRefreshSummary(rtn) string` — D-CLI-02 format: `"Fetched N issues (A attachments, C comments) in X.Xs → /path"` with literal `→` (U+2192).

### `cmd/wsh/cmd/wshcmd-jira_test.go`

Five tests, all pure-helper or cobra-help plumbing (no RpcClient):

| Test | What it asserts |
|------|-----------------|
| `TestJiraCmdHelp` | `jiraCmd.Help()` output contains `"refresh"` and the long description mentioning `jira.json`. |
| `TestJiraRefreshHelp` | `jiraRefreshCmd.Help()` output contains `--json` and `--timeout` flag names. |
| `TestJiraRefreshExitCodeMapping` | Table-driven: nil→0, `인증 실패…`→1, `설정 파일이 없습니다…`→2, network/other→3. |
| `TestJiraRefreshExitCodeNoTokenLeak` | T-03-06 regression: unclassified error with token-like substring maps to 3 (helper never echoes input). |
| `TestFormatRefreshSummary` | Plural case, sub-second (500ms→"0.5s"), over-minute (61000ms→"61.0s") — confirms exact format per D-CLI-02. |

## Exit-Code Mapping Decisions

Prefix match is contractually tied to Plan 03-01's `mapJiraError` output strings (pkg/wshrpc/wshserver/wshserver-jira.go). Mapping:

| Error prefix | Exit code | Rationale |
|--------------|-----------|-----------|
| `인증 실패` | 1 | PAT/auth failure (D-ERR-04 + REQ JIRA-04 Korean text) |
| `설정 파일이 없습니다` | 2 | First-run / missing jira.json |
| (anything else) | 3 | Network, disk, unknown — user still sees the Korean message on stderr |
| `nil` | 0 | Success |

**Why prefix match (not `errors.Is`):** Plan 03-01 wraps its sentinels with `fmt.Errorf("%s: %w", msg, err)` for some paths but returns fixed strings for the two classified cases. The CLI lives in a different process image and receives only `err.Error()` over the wire (RPC collapses errors to strings). Prefix match is the portable contract across the RPC boundary and was explicitly spec'd by the plan.

## exitCodeError Pattern Reuse Check

Grepped `cmd/wsh` for `exitCodeError` and `os.Exit(` — **no existing sentinel pattern found**. The repo uses a package-level `WshExitCode int` variable (declared in `wshcmd-root.go:37`) that `Execute()` forwards to `wshutil.DoShutdown`. `wshcmd-getvar.go:98` demonstrates the pattern:

```go
WshExitCode = 1
return nil
```

We follow that convention. Documented in a file-top comment in `wshcmd-jira.go` so future readers understand why there's no `os.Exit` call despite the plan's fallback suggestion of `os.Exit`.

## Help-output Snapshot

```
$ wsh jira --help
Jira commands. Configure ~/.config/waveterm/jira.json first. See README.

Usage:
  wsh jira [command]

Available Commands:
  refresh     Fetch latest Jira issues into the cache

Flags:
  -h, --help   help for jira

Global Flags:
  -b, --block string   for commands which require a block id

Use "wsh jira [command] --help" for more information about a command.
```

```
$ wsh jira refresh --help
Fetch latest Jira issues into the cache

Usage:
  wsh jira refresh [flags]

Flags:
  -h, --help          help for refresh
      --json          emit JSON instead of human-readable summary
      --timeout int   RPC timeout in seconds (default 60)

Global Flags:
  -b, --block string   for commands which require a block id
```

## Verification

- `go build ./cmd/wsh/...` — PASS (no output).
- `go test ./cmd/wsh/cmd/ -run "TestJira.*|TestFormatRefreshSummary" -v` — PASS (5 tests, all subtests).
- `go test ./cmd/wsh/cmd/` (full suite, no `-run`) — PASS (no regressions in existing tests).
- Real binary smoke: `/tmp/wsh-test jira --help` and `/tmp/wsh-test jira refresh --help` produce the snapshots above.

Deferred per D-TEST-02: live RPC-path integration test (requires a running wavesrv + configured jira.json — phase-level UAT covers this).

## Deviations from Plan

**Minor test-infrastructure adjustment (not a deviation from plan semantics):**

The plan suggested asserting help output via `jiraCmd.SetArgs([]string{"--help"}) + jiraCmd.Execute()`. In this repo's cobra wiring, `Execute()` on a subcommand walks back to the root, so `rootCmd.Help()` (listing ALL wsh commands) was printed instead of `jiraCmd.Help()`. Changed the tests to call `cmd.Help()` directly — same semantic assertion (help output of the command under test), just via the correct API. No change to production code.

Also adjusted the `TestJiraCmdHelp` substring check from "PAT-authenticated" (Short) to "jira.json" (Long) because cobra's `Help()` shows Long, not Short, when Long is set.

No Rule 1/2/3 auto-fixes were needed. No Rule 4 architectural stops. No authentication gates (CLI plumbing only; the RPC call is not exercised in unit tests).

## Known Stubs

None. All code paths are fully wired; the refresh path is end-to-end functional (will succeed against a real wavesrv + configured jira.json as of Plan 03-01).

## Deferred Issues

None from this plan. Plan 03-03 (widget wire-up) is running in parallel in another worktree.

## Self-Check: PASSED

- `cmd/wsh/cmd/wshcmd-jira.go` — FOUND
- `cmd/wsh/cmd/wshcmd-jira_test.go` — FOUND
- Commit `29f749f3` (RED) — FOUND
- Commit `32f1b06c` (GREEN) — FOUND
- `go build ./cmd/wsh/...` — PASSED
- All 5 new tests — PASSED

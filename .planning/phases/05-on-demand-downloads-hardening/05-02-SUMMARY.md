---
phase: "05"
plan: "02"
subsystem: jira-download
tags: [jira, download, rpc, cli, attachments]
dependency-graph:
  requires: [05-01]
  provides: [jira-download-logic, jira-download-rpc, wsh-jira-download-cli]
  affects: [frontend-types, wshclient-bindings]
tech-stack:
  added: []
  patterns: [streaming-io-copy, test-seam-function-vars, atomic-cache-update]
key-files:
  created:
    - pkg/jira/download.go
    - pkg/jira/download_test.go
  modified:
    - pkg/wshrpc/wshrpctypes.go
    - pkg/wshrpc/wshserver/wshserver-jira.go
    - pkg/wshrpc/wshserver/wshserver-jira_test.go
    - cmd/wsh/cmd/wshcmd-jira.go
    - cmd/wsh/cmd/wshcmd-jira_test.go
    - pkg/wshrpc/wshclient/wshclient.go (generated)
    - frontend/types/gotypes.d.ts (generated)
    - frontend/app/store/wshclientapi.ts (generated)
decisions:
  - Used function-variable test seams (cacheFilePathFn, attachmentDirFn, jiraDownload) for testability without interfaces
  - Downloads use attachment WebUrl from cache (Content URL) with Basic auth, not constructing REST API path
  - Skip-if-exists logic checks both localPath in cache AND file existence on disk
  - mapJiraDownloadError delegates config/auth/network errors to mapJiraError, adds download-specific prefixes
metrics:
  duration: "8m 36s"
  completed: "2026-04-16T01:45:04Z"
  tasks: 5
  files-changed: 10
---

# Phase 5 Plan 02: Attachment Download RPC + CLI Summary

Streaming attachment download to disk with cache localPath update, JiraDownloadCommand RPC handler, and `wsh jira download` CLI subcommand with generated TypeScript bindings.

## What Was Built

### 1. Download Logic (`pkg/jira/download.go`)
- `DownloadAttachments(ctx, opts)` function: reads cache, finds issue, filters by optional filename, streams each attachment to `~/.config/waveterm/jira-attachments/<KEY>/<filename>` using `io.Copy` (no in-memory buffer per D-DL-05)
- Atomic file write via temp file + rename pattern
- Skip-if-exists: checks `localPath` non-empty AND file present on disk
- Cache update: after download, sets `localPath` on matching attachment entries, writes atomically via `fileutil.AtomicWriteFile`
- Test seams: `cacheFilePathFn` and `attachmentDirFn` function variables for testability

### 2. Download Tests (`pkg/jira/download_test.go`)
- 7 test cases: success path, filename filter, issue not in cache, no filename match, server error (403), empty issue key validation, skip existing files
- All use httptest servers and temp directories

### 3. RPC Interface + Types (`pkg/wshrpc/wshrpctypes.go`)
- `JiraDownloadCommand` added to `WshRpcInterface`
- `CommandJiraDownloadData`: `IssueKey` (required), `Filename` (optional)
- `CommandJiraDownloadFileResult`: per-file result with `Skipped` flag
- `CommandJiraDownloadRtnData`: aggregate with `Files` slice and `TotalBytes`

### 4. RPC Handler (`pkg/wshrpc/wshserver/wshserver-jira.go`)
- `JiraDownloadCommand` handler with `jiraDownload` test seam
- `mapJiraDownloadError` for download-specific messages (not-in-cache, no-attachment, no-attachments), delegates to `mapJiraError` for config/auth/network
- 3 handler tests: success, config error, not-in-cache error

### 5. CLI Subcommand (`cmd/wsh/cmd/wshcmd-jira.go`)
- `wsh jira download <ISSUE-KEY> [filename]` with cobra, `Args: RangeArgs(1, 2)`
- `--json` and `--timeout` flags (default timeout 120s for large files)
- `formatDownloadSummary`: "N files (M downloaded, K skipped, X.Y MB total) for KEY"
- 4 CLI tests: help output, parent help lists download, format summary (3 cases)

### 6. Generated Bindings
- `task generate` ran successfully
- `wshclient.go`: `JiraDownloadCommand` function
- `gotypes.d.ts`: TypeScript types for download data/result
- `wshclientapi.ts`: frontend `JiraDownloadCommand` binding

## Commits

| # | Hash | Message |
|---|------|---------|
| 1 | f0cbe008 | feat(05-02): implement attachment download logic with tests |
| 2 | d36ccf34 | feat(05-02): add JiraDownloadCommand RPC interface, handler, and tests |
| 3 | 0646184b | feat(05-02): add wsh jira download CLI subcommand and tests |
| 4 | f67f8908 | chore(05-02): run task generate for JiraDownloadCommand bindings |

## Verification Results

- `go build ./...` -- PASS (clean)
- `go test ./pkg/jira/ -count=1` -- PASS (all 38 tests including 7 new download tests)
- `go test ./pkg/wshrpc/wshserver/ -count=1` -- PASS (all 10 tests including 3 new download handler tests)
- `go test ./cmd/wsh/cmd/ -count=1` -- PASS (all 24 tests including 4 new download CLI tests)
- `npx tsc --noEmit` -- PASS (clean)

## Deviations from Plan

None -- plan executed exactly as specified in CONTEXT.md decisions D-DL-01..D-DL-06.

## Decisions Made

1. **Test seam pattern**: Used function variables (`cacheFilePathFn`, `attachmentDirFn`) instead of interfaces for overriding file paths in tests -- matches the existing `jiraLoadConfig`/`jiraRefresh` pattern in the codebase.

2. **Download URL source**: Used the `WebUrl` field from the cache (which is the Jira `Content` URL) directly, with Basic auth headers. This avoids constructing a separate REST API path and works with the existing attachment data.

3. **Error mapping strategy**: Created `mapJiraDownloadError` that checks for download-specific error strings first, then delegates to the existing `mapJiraError` for config/auth/network errors. This keeps error handling consistent.

## Self-Check: PASSED

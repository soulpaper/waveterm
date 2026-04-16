---
status: partial
phase: 03-wsh-rpc-widget-wireup
source: [03-VERIFICATION.md]
started: 2026-04-15
updated: 2026-04-15
---

## Current Test

[awaiting human testing — requires running Electron + live Jira]

## Tests

### 1. `wsh jira refresh` end-to-end (ROADMAP Success #1)
expected: Running `wsh jira refresh` from a Waveterm terminal writes `~/.config/waveterm/jira-cache.json` and exits 0. Human-readable stdout summary, `--json` flag produces parseable JSON matching `CommandJiraRefreshRtnData`.
result: [pending]

### 2. Widget ☁️ button triggers same flow (ROADMAP Success #2)
expected: Clicking the ☁️ button on the Jira Tasks widget sets `loadingAtom=true` (spinner visible), calls `JiraRefreshCommand` RPC directly, re-loads cache on success, shows post-hoc summary ("N 이슈 · X.Xs") via `refreshProgressAtom` which fades after 5s.
result: [pending]

### 3. No `claude` subprocess spawned (ROADMAP Success #3)
expected: During a widget-triggered refresh, there is no `claude "..."` child process. Process tree inspection during click confirms Waveterm main → wsh RPC handler only.
result: [pending]

### 4. Error messages surface human-readable (ROADMAP Success #4)
expected:
- Delete/rename `~/.config/waveterm/jira.json` → click ☁️ → widget `errorAtom` shows string starting with "설정 파일이 없습니다".
- Edit `jira.json` with a bad `apiToken` → click ☁️ → widget `errorAtom` shows string starting with "인증 실패 — PAT 재발급 필요".
- `wsh jira refresh` exit codes match: 2 for config missing, 1 for auth failure.
result: [pending]

## Summary

total: 4
passed: 0
issues: 0
pending: 4
skipped: 0
blocked: 0

## Gaps

None — all automated checks GREEN. These UAT items deliberately require the Electron runtime and live Jira per D-TEST-02/D-TEST-03.

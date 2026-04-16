---
phase: 1
slug: jira-http-client-config
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-04-15
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (go1.26.1) |
| **Config file** | none — Go `testing` needs no config |
| **Quick run command** | `go test ./pkg/jira/... -count=1` |
| **Full suite command** | `go test ./pkg/jira/... -count=1 -race` |
| **Estimated runtime** | ~3 seconds (pure httptest + in-memory; no network) |

---

## Sampling Rate

- **After every task commit:** Run `go test ./pkg/jira/... -count=1`
- **After every plan wave:** Run `go test ./pkg/jira/... -count=1 -race`
- **Before `/gsd-verify-work`:** Full suite must be green
- **Max feedback latency:** ~3 seconds

---

## Per-Task Verification Map

Populated by planner — one row per task in the generated PLAN.md. Planner must add each task's `id` and automated command here when tasks are authored.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| (filled by planner) | | | | | | | | | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] Create `pkg/jira/` directory (does not exist today)
- [ ] Stub test files (planner decides split; minimum set below):
  - [ ] `pkg/jira/client_test.go` — HTTP path coverage for JIRA-01 (SearchIssues, GetIssue, auth header, 401/404/429/5xx)
  - [ ] `pkg/jira/config_test.go` — JIRA-02 config loader cases (happy, missing, malformed, incomplete, defaults fill)
  - [ ] `pkg/jira/adf_test.go` — D-14 node coverage + unknown-node-in-mixed-tree case
- [ ] Temp-file-based config tests MUST use `t.TempDir()` (Windows-safe)

*No framework install needed — Go stdlib `testing` ships with the toolchain.*

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| (none — all phase behaviors are automated via httptest + temp dirs) | | | |

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending

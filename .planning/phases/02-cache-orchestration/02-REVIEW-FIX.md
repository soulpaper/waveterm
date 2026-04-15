---
phase: 02-cache-orchestration
fixed_at: 2026-04-15T00:00:00Z
review_path: .planning/phases/02-cache-orchestration/02-REVIEW.md
iteration: 1
findings_in_scope: 2
fixed: 2
skipped: 0
status: all_fixed
---

# Phase 2: Code Review Fix Report

**Fixed at:** 2026-04-15T00:00:00Z
**Source review:** `.planning/phases/02-cache-orchestration/02-REVIEW.md`
**Iteration:** 1

**Summary:**
- Findings in scope: 2 (Critical + Warning; Info-level findings deferred per scope)
- Fixed: 2
- Skipped: 0
- Info findings deferred: IN-01, IN-02, IN-03, IN-04 (policy / nice-to-haves — not in scope)

**Verification:**
- `go test ./pkg/jira/ -count=1` — PASS (14/14 tests, includes new `TestRefresh_CommentBodyTruncationIsUTF8Safe`)
- `go vet ./pkg/jira/...` — clean

## Fixed Issues

### WR-01: Comment body truncation can produce invalid UTF-8 (mid-rune slice)

**Files modified:** `pkg/jira/refresh.go`, `pkg/jira/refresh_test.go`
**Commit:** `933e06a6`
**Applied fix:**

Replaced the byte-boundary slice `body[:2000]` with a rune-boundary walkback:

```go
cut := 2000
for cut > 0 && !utf8.RuneStart(body[cut]) {
    cut--
}
body = body[:cut]
```

Added `unicode/utf8` to the import block. Truncation now drops at most 1–3
trailing bytes to land on a valid rune boundary — the worst case for any
UTF-8-encoded codepoint, which is exactly what we want for Korean / CJK /
emoji content.

Added regression test `TestRefresh_CommentBodyTruncationIsUTF8Safe` that
drives the code path with 700 × "한" (U+D55C, 3 bytes each → 2100 bytes,
overruns the 2000 cap). The naive byte slice would land mid-rune at offset
2000 (2000 % 3 == 2); the fix walks back to byte 1998 (666 complete 3-byte
runes). Assertions:

- `utf8.ValidString(got)` is true
- `len(got) == 1998` (exact rune-aligned cut)
- `utf8.RuneCountInString(got) == 666`
- The written cache file is valid UTF-8 and contains no U+FFFD replacement
  character.

**Note on existing fixture:** `issue-comment-truncation.json` was not
modified; the new dedicated Korean-payload test supersedes its role for
this invariant and exercises a 3-byte-rune boundary directly. The existing
`TestRefresh_CommentCapAndTruncation` continues to cover the ASCII-path
invariants (length == 2000, `Truncated=true`, `"truncated":true` occurrence
count).

### WR-02: Cache file written with world-readable mode 0o644

**Files modified:** `pkg/jira/refresh.go`
**Commit:** `5e121f6a`
**Applied fix:**

Changed `fileutil.AtomicWriteFile(cachePath, data, 0o644)` to
`fileutil.AtomicWriteFile(cachePath, data, 0o600)`, matching the Phase 1
`config.json` mode so the two files have the same access posture on a
shared POSIX workstation. Added an inline comment explaining which fields
in the cache are sensitive-adjacent (issue summaries/descriptions, comment
bodies, `accountId`).

No tests broke — all Phase 2 tests run as the same user that writes the
file. On Windows (the developer's platform), POSIX mode bits are largely
advisory but the value is still recorded correctly by `os.WriteFile` and
preserves semantic intent for any Linux CI or deployment.

## Skipped Issues

None.

## Info Findings Deferred

Per fix scope (`critical_warning`), the following Info-level findings were
not applied. They remain documented in `02-REVIEW.md` for future cleanup:

- **IN-01** — `GetMyself` / `cacheFilePath()` ordering (hygiene, no behavior change)
- **IN-02** — `paginateSearch` requests unused `fields:["summary"]` (bandwidth)
- **IN-03** — No lockfile / no concurrency guard on `Refresh` (decision deferred
  until Phase 3 wsh + Phase 4 widget integration clarifies the call-site
  pattern — the reviewer explicitly suggested flagging it for that phase)
- **IN-04** — Test helper `runRefreshOneIssue` loses `*httptest.Server` reference
  (cosmetic)

---

_Fixed: 2026-04-15T00:00:00Z_
_Fixer: Claude (gsd-code-fixer)_
_Iteration: 1_

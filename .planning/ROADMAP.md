# Roadmap — Milestone v1.0 Jira Team Integration

**5 phases** | **10 requirements mapped** | All covered

| # | Phase | Goal | Requirements | Success Criteria |
|---|---|---|---|---|
| 1 | Jira HTTP Client + Config | Provide a reusable Go client that authenticates with PAT and reads user config | JIRA-01, JIRA-02 | 4 |
| 2 | Cache Orchestration | Fetch issues (incl. description/attachments/comments), convert ADF, truncate, atomic-write to existing schema | JIRA-06, JIRA-07 | 5 |
| 3 | wsh RPC + Widget Wire-up | Expose `wsh jira refresh`; widget calls RPC instead of injecting `claude "..."`; progress streamed to widget | JIRA-03, JIRA-08 | 4 |
| 4 | Setup UX + Docs | Actionable empty/error states; one-page README for team onboarding | JIRA-04, JIRA-09 | 3 |
| 5 | On-demand Downloads + Hardening | `wsh jira download <key>`; rate limits + retries | JIRA-05, JIRA-10 | 4 |

---

## Phase 1: Jira HTTP Client + Config

**Goal:** Provide a Go package that any Waveterm subsystem can use to call Jira Cloud REST v3 with PAT auth, and a config loader that reads `~/.config/waveterm/jira.json` with sensible defaults.

**Requirements:** JIRA-01, JIRA-02

**Plans:** 4 plans

Plans:
- [x] 01-01-scaffold-and-stubs-PLAN.md — Create pkg/jira/ directory + failing Nyquist stub tests for every D-22 coverage item
- [x] 01-02-config-and-errors-PLAN.md — Implement Config struct + LoadConfig (D-07..D-12) and APIError + sentinel errors (D-18..D-20)
- [x] 01-03-adf-converter-PLAN.md — Implement ADFToMarkdown for 10 block node types + 4 inline marks + hardBreak + mention (D-13..D-17)
- [x] 01-04-http-client-PLAN.md — Implement Client.SearchIssues (POST /rest/api/3/search/jql) + Client.GetIssue (D-01..D-06, D-19..D-20)

**Deliverables:**
- `pkg/jira/config.go` — struct + loader with defaults
- `pkg/jira/client.go` — HTTP client wrapping `http.Client`, Basic auth, JSON decode
- `pkg/jira/client_test.go` — httptest-backed unit tests covering auth, search, issue GET, error paths
- `pkg/jira/adf.go` — minimal ADF -> text/markdown converter (paragraphs, headings, lists, code, mentions, hard breaks)

**Success criteria:**
1. `jira.Client.SearchIssues(ctx, jql, nextToken)` returns issue keys and pagination cursor correctly (verified via httptest).
2. `jira.Client.GetIssue(ctx, key, fields)` returns parsed description + attachments + comments.
3. `jira.LoadConfig()` reads `~/.config/waveterm/jira.json`, fills missing fields with defaults, returns typed error when file missing.
4. Unit tests cover 200, 401, 429, 5xx response paths; all pass on Windows.

**Out of scope this phase:** wsh integration, cache writing, widget changes.

---

## Phase 2: Cache Orchestration

**Goal:** Combine Phase 1 primitives into a refresh operation that fetches all assigned issues and writes the cache in the schema the widget already consumes.

**Requirements:** JIRA-06, JIRA-07

**Plans:** 2 plans

Plans:
- [x] 02-01-client-extensions-and-tdd-red-PLAN.md — Additive Client extensions (Status.StatusCategory, GetMyself) + cache_types.go + failing refresh_test.go suite with fixtures and golden file (Nyquist RED)
- [x] 02-02-refresh-orchestration-green-PLAN.md — Implement pkg/jira/refresh.go (Refresh/RefreshOpts/RefreshReport + helpers) to turn all Wave-1 tests GREEN

**Deliverables:**
- `pkg/jira/refresh.go` — orchestration entry point `Refresh(ctx, opts) (*Report, error)`
- Field mapping: Jira response -> `JiraIssue` with description (ADF->md), attachments (metadata only, localPath empty by default), comments (latest 10, body truncated to 2000)
- Atomic cache write (temp file + rename)
- Preserve existing attachment `localPath` values (don't blow away previously-downloaded files)

**Success criteria:**
1. Running `jira.Refresh()` against a fake Jira server produces a cache JSON byte-identical to the widget's expected schema.
2. `commentCount` reflects total from API; `comments[]` capped at 10; `truncated: true` set when body exceeds 2000.
3. `lastCommentAt` = max(updated, created) across kept comments.
4. Existing `localPath` fields for downloaded attachments survive a refresh cycle.
5. Widget (un-modified) successfully reads cache produced by this phase and renders all expected UI.

**Out of scope this phase:** RPC plumbing, UI changes.

---

## Phase 3: wsh RPC + Widget Wire-up

**Goal:** Expose the refresh operation as a wsh command and wire the widget's button to call it directly instead of spawning a Claude terminal.

**Requirements:** JIRA-03, JIRA-08

**Plans:** 3 plans

Plans:
- [x] 03-01-rpc-method-and-handler-PLAN.md — Add JiraRefreshCommand to WshRpcInterface + WshServer handler with Korean error-class mapping (D-ERR-01) + Nyquist RED->GREEN test + `task generate` TS regen (Wave 1)
- [x] 03-02-wsh-jira-cli-PLAN.md — cmd/wsh/cmd/wshcmd-jira.go: `wsh jira refresh` cobra subcommand with --json/--timeout flags + exit-code mapping per D-ERR-04 (Wave 2, parallel to 03-03)
- [x] 03-03-widget-wireup-PLAN.md — Rewrite requestJiraRefresh() to call RpcApi.JiraRefreshCommand directly, add refreshProgressAtom, update tooltip, manual UAT checkpoint for ROADMAP Success #2-4 (Wave 2, parallel to 03-02)

**Deliverables:**
- New RPC command in `pkg/wshrpc/wshrpctypes.go` + handler
- `wsh jira refresh` CLI verb
- TS type regeneration (`task generate`)
- Widget `requestJiraRefresh()` calls RPC; surfaces progress via existing `loadingAtom` / new `refreshProgressAtom`
- Error path: RPC returns typed error -> widget shows message without re-reading cache

**Success criteria:**
1. `wsh jira refresh` run from terminal writes the cache file and exits 0.
2. Widget button triggers the same flow, spinner shown, cache re-loaded on completion.
3. No `claude "..."` subprocess is spawned by the refresh button anymore.
4. Refresh errors (bad token, network) surface to the widget with human-readable text, not just silently fail.

---

## Phase 4: Setup UX + Docs

**Goal:** First-run experience that guides a teammate from "just installed Waveterm" to "widget populated" without asking a Waveterm developer for help.

**Requirements:** JIRA-04, JIRA-09

**Deliverables:**
- Empty state in widget when `jira.json` missing: actionable card with Claude prompt snippet + link to README
- Distinct error state for 401 (token issue) vs network/unknown
- README section (`docs/jira-widget-setup.md` or append to existing): how to get a PAT, jira.json template, verify with `wsh jira refresh`
- "Let Claude auto-set this up for me" prompt that guides Claude to gather cloudId/email/PAT from user and write jira.json

**Success criteria:**
1. New teammate following README can produce a working jira.json in under 5 minutes.
2. Missing-config and invalid-token states are visually distinct and each include the fix action.
3. "Ask Claude to set up Jira" flow actually produces a valid jira.json (end-to-end verified with a fresh profile).

---

## Phase 5: On-demand Downloads + Hardening

**Goal:** Let users pull specific attachments to disk when they need them and make the refresh resilient to Jira's rate limits and transient failures.

**Requirements:** JIRA-05, JIRA-10

**Plans:** 3 plans

Plans:
- [x] 05-01-PLAN.md — Rate limiter + retry transports (RateLimitedTransport + RetryTransport in transport.go with TDD)
- [ ] 05-02-PLAN.md — Attachment download logic (download.go) + RPC handler + `wsh jira download` CLI + `task generate`
- [ ] 05-03-PLAN.md — Wire hardened transport into NewClient default + integration test (429/5xx retry through full Client)

**Deliverables:**
- `wsh jira download <KEY> [filename]` — pulls attachment(s), streams to disk, updates cache entry's `localPath`
- Rate limiter (token bucket, 10 req/s default) applied to all Jira HTTP calls
- Retry on 429 (honor `Retry-After`) and 5xx (exponential backoff, max 3)
- Progress logging for long refreshes (N issues fetched, M comments processed)

**Success criteria:**
1. `wsh jira download ITSM-3135` downloads both attachments to `~/.config/waveterm/jira-attachments/ITSM-3135/`, updates cache, widget next load shows local preview on click.
2. Simulated 429 response triggers backoff and eventual success (verified in unit test).
3. Simulated 500 is retried 3 times and reports a final actionable error.
4. Refresh of 50 issues completes without tripping Atlassian's rate limit in real testing.

---

## Notes

- Phases 1 -> 2 -> 3 must run sequentially (each builds on prior).
- Phase 4 can run concurrently with Phase 5 if time permits; otherwise serial.
- Each phase ends with its own commit and PHASE.md in `.planning/phases/`.

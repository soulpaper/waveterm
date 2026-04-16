# Phase 1: Jira HTTP Client + Config — Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-04-15
**Phase:** 01-jira-http-client-config
**Areas discussed:** All four gray areas (bulk-accepted via recommended defaults)

---

## Gray area selection (meta)

| Option | Description | Selected |
|--------|-------------|----------|
| Jira API endpoint / pagination | v3 legacy `/search` vs new `/search/jql` (cursor). Fields parameter handling. | (bulk) |
| Client API shape (Go signature) | Options struct vs variadic functional options vs positional args. Internal `http.Client` vs injected. | (bulk) |
| ADF converter scope / output format | Which nodes to support, plain vs markdown output, unknown-node policy. | (bulk) |
| Error typing for downstream phases | Sentinel errors vs typed struct vs both. How Phase 4 distinguishes "bad token" from "no config". | (bulk) |

**User's choice:** Initial reply was "뭔소리 인지 모르겠다. 니가 알아서 잘해봐라 설명을 자세히 하든지." — after each area was re-explained with concrete examples and a recommendation, user selected "내 추천대로 다 가자 (Recommended)" covering all four areas.

**Notes:** User explicitly delegated implementation-detail decisions to the assistant once the options were clearly explained. Decisions in CONTEXT.md reflect the recommended option for each area, expanded into concrete specifics.

---

## Area 1 — Jira API endpoint / pagination

| Option | Description | Selected |
|--------|-------------|----------|
| Legacy `/rest/api/3/search` (startAt/maxResults) | Offset-based, stable for years. Atlassian has announced deprecation. | |
| Enhanced `/rest/api/3/search/jql` (nextPageToken) | Cursor-based, current recommended endpoint. | ✓ |
| Both, switch at runtime | Support legacy as fallback. Extra code. | |

**User's choice:** Enhanced endpoint only (via "내 추천대로 다 가자").
**Notes:** Internal team is on Jira Cloud with enhanced search available. No value in carrying legacy-API code that'll be removed.

---

## Area 2 — Client API shape (Go)

| Option | Description | Selected |
|--------|-------------|----------|
| Options struct (`SearchOpts`, `GetIssueOpts`) | Easy to extend without breaking callers. Matches existing project style. | ✓ |
| Variadic functional options (`WithPageToken`, `WithFields`) | Idiomatic in some Go libs but heavier for a small surface. | |
| Positional args | Simplest but brittle when adding params later. | |

**User's choice:** Options struct.
**Notes:** Phase 2 (refresh orchestration) will add fields; struct keeps call sites stable.

---

## Area 3 — ADF converter scope / output

| Option | Description | Selected |
|--------|-------------|----------|
| Core nodes only (paragraph/heading/list/code/mention/hardBreak), plain text | Minimal scope, fastest to ship. | |
| Core + tables + inline marks, **markdown** output | Covers ~95% of real issue descriptions. Widget already renders markdown. | ✓ |
| Comprehensive (all ADF nodes including media/panel/emoji), markdown | Most complete, larger surface to test/maintain. | |

**User's choice:** Core + tables + inline marks, markdown.
**Notes:** Unknown nodes are silently skipped with a debug log — partial render is preferred over failure.

---

## Area 4 — Error typing

| Option | Description | Selected |
|--------|-------------|----------|
| Sentinel errors only (`errors.Is` checks) | Simple, cheap. Loses status code / body context. | |
| Typed struct only (`errors.As`) | Keeps all context. Callers always have to type-assert for class checks. | |
| **Both** — sentinel + struct with `Unwrap()` bridging | Callers pick granularity. `errors.Is` for class, `errors.As` for details. | ✓ |

**User's choice:** Both.
**Notes:** Phase 4 needs to distinguish 401 (bad token) from network/unknown via simple `errors.Is`; Phase 5's retry logic needs the `Retry-After` header value off the struct.

---

## Claude's Discretion

The following were explicitly left to planner/executor judgment (captured in CONTEXT.md §"Claude's Discretion"):
- Internal file split within `pkg/jira/`
- Exact struct field names on `SearchResult` / `Issue` (mirror Jira's JSON shape)
- Visitor pattern vs recursive `map[string]any` walk for ADF
- Package logging style (`log` vs `slog`) — match existing `pkg/` convention

## Deferred Ideas

- Retry / backoff (Phase 5)
- Rate limiter (Phase 5)
- Attachment download (Phase 5)
- Comment truncation / "keep latest 10" (Phase 2 — transformation concern, not HTTP concern)
- Jira Server support (out of milestone)
- OAuth, safeStorage, settings modal (out of milestone — `JIRA-F-01..03`)

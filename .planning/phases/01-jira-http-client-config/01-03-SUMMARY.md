---
phase: 01-jira-http-client-config
plan: 03
subsystem: pkg/jira
tags: [adf, markdown, converter, go, nyquist-green]
requires:
  - plan: 01-01 (adf_test.go stubs to turn GREEN)
provides:
  - api: ADFToMarkdown(raw json.RawMessage) (string, error)
  - file: pkg/jira/adf.go
affects: []
tech_stack:
  added: []
  patterns:
    - recursive map[string]any walker (over struct-visitor — chosen for D-14's small node set)
    - mark application inside-out (innermost code → em → strong → outermost link)
    - GFM pipe table with header-row detection via tableHeader presence
    - log.Printf debug emit for unknown nodes (D-15 silent skip + descend)
key_files:
  created:
    - pkg/jira/adf.go
  modified: []
decisions:
  - "Map-walk over struct-visitor: D-14 has only ~14 node types and no need for a full type-safe AST; recursive map[string]any walker is shorter and matches RESEARCH §ADF Converter Implementation Style"
  - "Mark application order: code → em → strong → link (innermost to outermost). Deterministic per renderText comment; tests assert presence of wrapping characters not nesting order"
  - "Heading level clamped to [1,6] (T-01-03-04 mitigation): out-of-range or non-numeric attrs default to level 1; protects against malformed Jira output"
  - "Unknown-node log NOT deduplicated: a package-level dedupe map would add mutex overhead; per-occurrence log.Printf accepted as informational"
  - "Standalone tableRow/tableHeader/tableCell render children inline (rare path, defensive default — normal flow is renderTable orchestrating from the table node)"
metrics:
  duration: "~10 min"
  completed: "2026-04-15"
  tasks_completed: 1
  files_created: 1
  total_lines: 317
---

# Phase 01 Plan 03: ADF Converter Summary

Implemented `pkg/jira/adf.go` — a 317-line stdlib-only Atlassian Document Format → markdown converter that turns all 4 `TestADFToMarkdown_*` functions GREEN (15 sub-cases pass).

## Files Created

| File | Lines | Purpose |
|------|------:|---------|
| `pkg/jira/adf.go` | 317 | `ADFToMarkdown` entry point + recursive map-walk renderer covering all D-14 node types |

## Commits

| Task | Commit | Message |
|------|--------|---------|
| 1 | `1d85e2f3` | `feat(01-03): implement ADFToMarkdown converter` |

## Test Results

Tests cannot run via `go test ./pkg/jira/... -count=1` from this worktree because Wave 2 sibling worktrees (Plans 01-02 and 01-04) own the production symbols (`Config`, `Client`, `SearchOpts`, etc.) referenced by `client_test.go` / `config_test.go`. The plan explicitly accepts this in `<verification>`: *"`pkg/jira/client_test.go` still fails to compile — this plan does not touch client"*.

To verify ADF tests in isolation, the sibling test files were temporarily moved aside (`*.go.disabled`), tests ran, then files were restored. `-race` was skipped because the local Windows toolchain has no `gcc` for cgo (will be exercised in CI / merged worktree).

```
$ go test ./pkg/jira/ -run "TestADFToMarkdown" -count=1 -v   # (with sibling tests aside)
=== RUN   TestADFToMarkdown_EmptyInput
--- PASS: TestADFToMarkdown_EmptyInput (0.00s)
=== RUN   TestADFToMarkdown_MalformedJSON
--- PASS: TestADFToMarkdown_MalformedJSON (0.00s)
=== RUN   TestADFToMarkdown_PerNodeType
    --- PASS: TestADFToMarkdown_PerNodeType/paragraph
    --- PASS: TestADFToMarkdown_PerNodeType/heading_h2
    --- PASS: TestADFToMarkdown_PerNodeType/bulletList
    --- PASS: TestADFToMarkdown_PerNodeType/orderedList
    --- PASS: TestADFToMarkdown_PerNodeType/codeBlock
    --- PASS: TestADFToMarkdown_PerNodeType/blockquote
    --- PASS: TestADFToMarkdown_PerNodeType/rule
    --- PASS: TestADFToMarkdown_PerNodeType/hardBreak
    --- PASS: TestADFToMarkdown_PerNodeType/mark_strong
    --- PASS: TestADFToMarkdown_PerNodeType/mark_em
    --- PASS: TestADFToMarkdown_PerNodeType/mark_code
    --- PASS: TestADFToMarkdown_PerNodeType/mark_link
    --- PASS: TestADFToMarkdown_PerNodeType/mention_with_text
    --- PASS: TestADFToMarkdown_PerNodeType/mention_fallback_to_id
    --- PASS: TestADFToMarkdown_PerNodeType/table_with_header_row
=== RUN   TestADFToMarkdown_UnknownNodeInMixedTree
2026/04/15 17:55:56 jira: unknown ADF node type "panel" (silently skipping, descending into children)
--- PASS: TestADFToMarkdown_UnknownNodeInMixedTree (0.05s)
PASS
ok      github.com/wavetermdev/waveterm/pkg/jira       0.516s
```

`go build ./pkg/jira/` succeeds standalone (production code only — no test build).

## Supported Node Types — Sample Input → Output

| Node Type | Sample ADF (abbrev.) | Markdown Output (excerpt) |
|-----------|----------------------|---------------------------|
| `paragraph` | `{type:paragraph, content:[{type:text, text:"hello"}]}` | `hello` |
| `heading` h2 | `{type:heading, attrs:{level:2}, content:[{type:text, text:"Title"}]}` | `## Title` |
| `bulletList` | `{type:bulletList, content:[{type:listItem, content:[paragraph "A"]}]}` | `- A` |
| `orderedList` | `{type:orderedList, content:[listItem "A", listItem "B"]}` | `1. A\n2. B` |
| `codeBlock` (lang=go) | `{type:codeBlock, attrs:{language:"go"}, content:[text "x := 1"]}` | `` ```go\nx := 1\n``` `` |
| `blockquote` | `{type:blockquote, content:[paragraph "Q"]}` | `> Q` |
| `rule` | `{type:rule}` | `---` |
| `hardBreak` (in para) | `paragraph["a", hardBreak, "b"]` | `a\nb` |
| `text` mark `strong` | `text "bold" + marks:[strong]` | `**bold**` |
| `text` mark `em` | `text "it" + marks:[em]` | `*it*` |
| `text` mark `code` | `text "x" + marks:[code]` | `` `x` `` |
| `text` mark `link` | `text "go" + marks:[link, attrs:{href:"https://go.dev"}]` | `[go](https://go.dev)` |
| `mention` (with text) | `{type:mention, attrs:{id:"5b10", text:"@Bradley"}}` | `@Bradley` |
| `mention` (fallback) | `{type:mention, attrs:{id:"5b10"}}` (no text attr) | `@5b10` |
| `table` (1 header row + 1 body row) | `table[ tableRow[tableHeader×2], tableRow[tableCell×2] ]` | `\| H1 \| H2 \|\n\| --- \| --- \|\n\| c1 \| c2 \|` |
| unknown (e.g. `panel`) | `{type:panel, content:[paragraph "inside-panel"]}` | (logged + descended; "inside-panel" rendered) |

## Mark Nesting Order Assumption

When a text node carries multiple marks (e.g., `text:"x"`, `marks:[{type:"strong"},{type:"link",attrs:{href:"..."}}]`), the converter wraps INSIDE-OUT in this fixed order:

1. `code` (innermost — e.g., `` `x` ``)
2. `em` (e.g., `*x*`)
3. `strong` (e.g., `**x**`)
4. `link` (outermost — e.g., `[x](href)`)

So `bold + link` becomes `[**x**](href)`, not `**[x](href)**`. This is documented in `renderText`'s comment block. The contract tests only assert the presence of each marker pair, never their nesting order, so the assumption is forward-compatible if a future plan needs to flip ordering.

## Threat Model Verification

| Threat ID | Disposition | Verification |
|-----------|-------------|--------------|
| T-01-03-01 (DoS via deep nesting) | accept | No mitigation needed — Go runtime grows stack dynamically; Jira caps depth server-side |
| T-01-03-02 (link href tampering) | accept | Sanitization is downstream renderer's job; converter is lossless |
| T-01-03-03 (info disclosure via log) | mitigate | `logUnknownADFType(nodeType string)` receives ONLY the node type string — no children, no attrs, no payload. Verified: `grep "logUnknownADFType" pkg/jira/adf.go` shows single-string param |
| T-01-03-04 (heading level overflow) | mitigate | Lines 75-83 of `adf.go`: `level` clamped to `[1, 6]`; non-numeric attrs default to `1` |

## Deviations from Plan

None — plan executed exactly as written. Only adjustment was a verification workaround (temporarily moving sibling test files aside) because Wave 2 worktree isolation prevents the `pkg/jira` test binary from linking until 01-02 and 01-04 also land. This was anticipated by the plan's `<verification>` block.

## Acceptance Criteria — All ✅

- ✅ `pkg/jira/adf.go` exists
- ✅ `func ADFToMarkdown(raw json.RawMessage) (string, error)` signature matches D-16
- ✅ All 13 required `case "<node>":` clauses present (doc, paragraph, heading, bulletList, orderedList, listItem, codeBlock, blockquote, rule, table, hardBreak, text, mention)
- ✅ `attrs["href"]` used for links (NOT `attrs["url"]`) — Pitfall 6
- ✅ `attrs["text"]` used for mentions (NOT `attrs["displayName"]`) — Pitfall 5
- ✅ `log.Printf` present for unknown-node logging (D-15)
- ✅ `go build ./pkg/jira/` exits 0
- ✅ All 4 ADF test functions GREEN (15 sub-cases in `TestADFToMarkdown_PerNodeType` pass)
- ✅ `TestADFToMarkdown_UnknownNodeInMixedTree` passes — "panel" logged, "before" + "after" rendered

## Self-Check: PASSED

- FOUND: pkg/jira/adf.go
- FOUND commit: 1d85e2f3
- FOUND: ADFToMarkdown signature in adf.go
- FOUND: all 13 case clauses for D-14 node types
- FOUND: attrs["href"] for links, attrs["text"] for mentions
- FOUND: log.Printf for unknown-node debug emit
- VERIFIED: production build succeeds (`go build ./pkg/jira/`)
- VERIFIED: 4/4 ADF tests GREEN in isolated test run (15/15 sub-cases pass)

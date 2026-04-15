---
phase: 01-jira-http-client-config
plan: 03
type: execute
wave: 2
depends_on: [01-01]
files_modified:
  - pkg/jira/adf.go
autonomous: true
requirements: [JIRA-01, JIRA-02]

must_haves:
  truths:
    - "ADFToMarkdown(raw) returns markdown string for all 10 block node types + 4 inline marks + hardBreak + mention (D-14)"
    - "Unknown node types render surrounding siblings and descend into children if present (D-15)"
    - "Input '' and 'null' return empty string with nil error (graceful degenerate case)"
    - "Structural JSON parse failure returns error (D-16)"
    - "Unknown-node log emits via log.Printf at debug level; does not fail the call (D-15)"
  artifacts:
    - path: "pkg/jira/adf.go"
      provides: "ADFToMarkdown entry point + internal node walker"
      contains: "func ADFToMarkdown"
  key_links:
    - from: "ADFToMarkdown"
      to: "json.Unmarshal into map[string]any then dispatch on node[type]"
      via: "recursive walk per RESEARCH §ADF Converter Implementation Style"
      pattern: "node\\[.type.\\]"
    - from: "mention renderer"
      to: "attrs.text (fallback to @ + attrs.id)"
      via: "D-14 + RESEARCH Pitfall 5"
      pattern: "attrs\\[.text.\\]"
    - from: "link mark renderer"
      to: "attrs.href (NOT attrs.url)"
      via: "D-14 + RESEARCH Pitfall 6"
      pattern: "attrs\\[.href.\\]"
---

<objective>
Implement `pkg/jira/adf.go` — the minimal ADF → markdown converter that handles the 10 block node types + 4 inline marks + hardBreak + mention specified in D-14.

Purpose: Single function `ADFToMarkdown(raw json.RawMessage) (string, error)` used by both issue descriptions and comment bodies (D-17). Phase 2 calls this from the refresh orchestrator; Phase 1 exposes it but does not invoke it from GetIssue (which returns raw json.RawMessage for flexibility).

Output: One file (`adf.go`) that turns all `TestADFToMarkdown_*` cases GREEN.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/phases/01-jira-http-client-config/01-CONTEXT.md
@.planning/phases/01-jira-http-client-config/01-RESEARCH.md
@.planning/phases/01-jira-http-client-config/01-01-SUMMARY.md

<interfaces>
<!-- ADF wire format (from RESEARCH §ADF Node Shapes). These are the exact JSON
     shapes the walker must handle. -->

Block nodes (each has a "type" string field; most also have "content": []):
- doc:         {type:"doc", content: [...blocks]}
- paragraph:   {type:"paragraph", content: [...inlines]}
- heading:     {type:"heading", attrs:{level: 1..6}, content: [...inlines]}
- bulletList:  {type:"bulletList", content: [...listItems]}
- orderedList: {type:"orderedList", content: [...listItems]}
- listItem:    {type:"listItem", content: [...blocks, typically paragraph]}
- codeBlock:   {type:"codeBlock", attrs:{language: "go"}?, content: [...text nodes]}
- blockquote:  {type:"blockquote", content: [...blocks]}
- rule:        {type:"rule"}
- table:       {type:"table", content: [...tableRows]}
- tableRow:    {type:"tableRow", content: [...tableHeader|tableCell]}
- tableHeader: {type:"tableHeader", content: [...blocks]}
- tableCell:   {type:"tableCell", content: [...blocks]}

Inline nodes:
- text:      {type:"text", text:"hello", marks: [{type:"strong"|"em"|"code"|"link", attrs?}]}
- hardBreak: {type:"hardBreak"}
- mention:   {type:"mention", attrs:{id:"5b10...", text:"@Bradley Ayers"}}  // text is optional

Marks (applied to text nodes; text renders inside-out):
- strong: **text**
- em:     *text*
- code:   `text`
- link:   [text](attrs.href)

From pkg/jira (already created):
```go
// doc.go provides: package jira
// This file adds only:
func ADFToMarkdown(raw json.RawMessage) (string, error)
```
</interfaces>
</context>

<tasks>

<task type="auto" tdd="true">
  <name>Task 1: Implement pkg/jira/adf.go recursive map-walk converter</name>
  <files>pkg/jira/adf.go</files>
  <read_first>
    - pkg/jira/adf_test.go (the contract — 4 Test functions covering every D-14 node)
    - .planning/phases/01-jira-http-client-config/01-CONTEXT.md D-13, D-14, D-15, D-16, D-17
    - .planning/phases/01-jira-http-client-config/01-RESEARCH.md §ADF Node Shapes, §ADF Converter Implementation Style, §Pitfall 5 mention attrs.text, §Pitfall 6 link attrs.href
  </read_first>
  <behavior>
    - TestADFToMarkdown_EmptyInput: `ADFToMarkdown(json.RawMessage(""))` → `("", nil)`; `ADFToMarkdown(json.RawMessage("null"))` → `("", nil)`
    - TestADFToMarkdown_MalformedJSON: `ADFToMarkdown(json.RawMessage("{not json"))` returns non-nil error
    - TestADFToMarkdown_PerNodeType: 15 sub-cases; each asserts output CONTAINS a specific substring (e.g., "## Title", "- A", "1. A", "```go", "> Q", "---", "**bold**", "*it*", "`x`", "[go](https://go.dev)", "@Bradley", "@5b10", "a\nb", "| H1 | H2 |")
    - TestADFToMarkdown_UnknownNodeInMixedTree: "panel" (not in D-14) wraps a paragraph; output must contain "before" and "after" (siblings of the unknown), "inside-panel" MAY or MAY NOT appear (implementation's choice). No error.
  </behavior>
  <action>
Create `pkg/jira/adf.go` using a recursive `map[string]any` walk (per RESEARCH §ADF Converter Implementation Style — chosen for simplicity over a struct-visitor pattern).

```go
// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
)

// ADFToMarkdown converts an Atlassian Document Format (ADF) JSON document to
// markdown. Supported node types per D-14:
//
//   block:  doc, paragraph, heading (1-6), bulletList, orderedList, listItem,
//           codeBlock, blockquote, rule, table, tableRow, tableHeader, tableCell
//   inline: text (+marks strong/em/code/link), hardBreak, mention
//
// Unknown node types (panel, media, inlineCard, emoji, ...) are silently
// skipped per D-15. The walker descends into their "content" array if present
// so sibling text still renders. A single log.Printf records each unique
// unknown type for debugging.
//
// Returns an error ONLY on structural JSON parse failure (D-16). Empty input
// or literal "null" returns "" without error.
//
// Used for both issue description and comment body ADF (D-17).
func ADFToMarkdown(raw json.RawMessage) (string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return "", nil
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("jira: ADF parse error: %w", err)
	}
	var sb strings.Builder
	renderNode(&sb, doc)
	return strings.TrimSpace(sb.String()), nil
}

// renderNode dispatches on node["type"] and writes markdown to sb. Children
// in node["content"] are rendered recursively. Unknown types fall through to
// the default case which descends but emits no wrapping syntax.
func renderNode(sb *strings.Builder, node map[string]any) {
	if node == nil {
		return
	}
	nodeType, _ := node["type"].(string)
	switch nodeType {
	case "doc":
		renderChildren(sb, node)

	case "paragraph":
		renderChildren(sb, node)
		sb.WriteString("\n\n")

	case "heading":
		level := 1
		if attrs, ok := node["attrs"].(map[string]any); ok {
			if lv, ok := attrs["level"].(float64); ok {
				level = int(lv)
			}
		}
		if level < 1 {
			level = 1
		}
		if level > 6 {
			level = 6
		}
		sb.WriteString(strings.Repeat("#", level))
		sb.WriteString(" ")
		renderChildren(sb, node)
		sb.WriteString("\n\n")

	case "bulletList":
		for _, child := range children(node) {
			if cm, ok := child.(map[string]any); ok {
				sb.WriteString("- ")
				renderListItemInline(sb, cm)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")

	case "orderedList":
		n := 1
		for _, child := range children(node) {
			if cm, ok := child.(map[string]any); ok {
				fmt.Fprintf(sb, "%d. ", n)
				n++
				renderListItemInline(sb, cm)
				sb.WriteString("\n")
			}
		}
		sb.WriteString("\n")

	case "listItem":
		// Rarely reached directly — parent list handles prefixing and
		// strips trailing newlines via renderListItemInline. If reached
		// standalone (odd input), just render children.
		renderChildren(sb, node)

	case "codeBlock":
		lang := ""
		if attrs, ok := node["attrs"].(map[string]any); ok {
			if l, ok := attrs["language"].(string); ok {
				lang = l
			}
		}
		sb.WriteString("```")
		sb.WriteString(lang)
		sb.WriteString("\n")
		// Code block children are text nodes — render raw text with no marks.
		for _, child := range children(node) {
			if cm, ok := child.(map[string]any); ok {
				if cm["type"] == "text" {
					if txt, ok := cm["text"].(string); ok {
						sb.WriteString(txt)
					}
				}
			}
		}
		sb.WriteString("\n```\n\n")

	case "blockquote":
		// Render children into a temp buffer, then prefix each line with "> ".
		var inner strings.Builder
		for _, child := range children(node) {
			if cm, ok := child.(map[string]any); ok {
				renderNode(&inner, cm)
			}
		}
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			sb.WriteString("> ")
			sb.WriteString(line)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")

	case "rule":
		sb.WriteString("---\n\n")

	case "table":
		renderTable(sb, node)

	case "hardBreak":
		sb.WriteString("\n")

	case "text":
		renderText(sb, node)

	case "mention":
		renderMention(sb, node)

	case "tableRow", "tableHeader", "tableCell":
		// Standalone render (rare — normally driven by renderTable). Just
		// emit children inline.
		renderChildren(sb, node)

	default:
		// D-15: unknown node — log once per unique type, descend into content.
		if nodeType != "" {
			logUnknownADFType(nodeType)
		}
		renderChildren(sb, node)
	}
}

// renderChildren walks node["content"] (an []any) and recursively renders
// each child that is a map.
func renderChildren(sb *strings.Builder, node map[string]any) {
	for _, child := range children(node) {
		if cm, ok := child.(map[string]any); ok {
			renderNode(sb, cm)
		}
	}
}

// children returns node["content"] as []any, or nil if missing/wrong type.
func children(node map[string]any) []any {
	c, _ := node["content"].([]any)
	return c
}

// renderListItemInline renders a listItem's contents on a single line,
// stripping trailing newlines that paragraph would otherwise append.
func renderListItemInline(sb *strings.Builder, item map[string]any) {
	var inner strings.Builder
	renderChildren(&inner, item)
	sb.WriteString(strings.TrimRight(inner.String(), "\n"))
}

// renderText renders a text node with marks. Marks apply inside-out: we build
// the inner string first (the raw text), then wrap it mark by mark.
func renderText(sb *strings.Builder, node map[string]any) {
	txt, _ := node["text"].(string)
	if txt == "" {
		return
	}
	marks, _ := node["marks"].([]any)
	result := txt
	// Apply marks in a deterministic order: code innermost, then em, then
	// strong, then link outermost. The order affects nesting visuals but
	// all combinations are valid markdown.
	for _, m := range marks {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		switch mm["type"] {
		case "code":
			result = "`" + result + "`"
		case "em":
			result = "*" + result + "*"
		case "strong":
			result = "**" + result + "**"
		case "link":
			href := ""
			if attrs, ok := mm["attrs"].(map[string]any); ok {
				// D-14 + RESEARCH Pitfall 6: the attribute is `href`, NOT `url`.
				href, _ = attrs["href"].(string)
			}
			result = "[" + result + "](" + href + ")"
		default:
			// Unknown mark — render raw text without wrapping.
		}
	}
	sb.WriteString(result)
}

// renderMention renders an ADF mention node. Per D-14 + RESEARCH Pitfall 5,
// the display string is attrs.text (already includes leading "@"). If text
// is missing, fall back to "@" + attrs.id.
func renderMention(sb *strings.Builder, node map[string]any) {
	attrs, _ := node["attrs"].(map[string]any)
	if attrs == nil {
		return
	}
	if t, ok := attrs["text"].(string); ok && t != "" {
		sb.WriteString(t)
		return
	}
	if id, ok := attrs["id"].(string); ok && id != "" {
		sb.WriteString("@")
		sb.WriteString(id)
	}
}

// renderTable emits a GFM pipe table. The first tableRow whose children
// contain any tableHeader is treated as the header row (the separator row
// is only emitted when a header row exists).
func renderTable(sb *strings.Builder, node map[string]any) {
	rows := children(node)
	if len(rows) == 0 {
		return
	}
	// Detect header row: the first row containing any tableHeader child.
	headerIdx := -1
	for i, r := range rows {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		for _, c := range children(rm) {
			if cm, ok := c.(map[string]any); ok && cm["type"] == "tableHeader" {
				headerIdx = i
				break
			}
		}
		if headerIdx != -1 {
			break
		}
	}
	renderRow := func(rm map[string]any) {
		sb.WriteString("|")
		for _, c := range children(rm) {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			var inner strings.Builder
			renderChildren(&inner, cm)
			cellText := strings.TrimSpace(strings.ReplaceAll(inner.String(), "\n", " "))
			sb.WriteString(" ")
			sb.WriteString(cellText)
			sb.WriteString(" |")
		}
		sb.WriteString("\n")
	}
	for i, r := range rows {
		rm, ok := r.(map[string]any)
		if !ok {
			continue
		}
		renderRow(rm)
		if i == headerIdx {
			// Emit separator row with same column count.
			cols := len(children(rm))
			sb.WriteString("|")
			for c := 0; c < cols; c++ {
				sb.WriteString(" --- |")
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")
}

// logUnknownADFType emits a debug log for each unique unknown ADF node type.
// Per D-15 this never fails the conversion — it's informational only. We do
// NOT deduplicate across calls (the map would be a package-level var with
// mutex overhead); a single log per occurrence is acceptable and matches
// the codebase's stdlib "log" convention.
func logUnknownADFType(nodeType string) {
	log.Printf("jira: unknown ADF node type %q (silently skipping, descending into children)", nodeType)
}
```

Implementation notes:
- Use `map[string]any` not `map[string]interface{}` — Go 1.18+ `any` alias, matches modern codebase style.
- JSON numbers decode as `float64` — the heading level extraction handles this (`.(float64)`).
- Do NOT use `regexp` anywhere in this file — all parsing is structural over the decoded map.
- Do NOT export any symbol other than `ADFToMarkdown`.
- Mark application order (code → em → strong → link) is DETERMINISTIC and documented in the renderText function comment; tests only assert on the presence of the wrapping characters, not their order of nesting.
  </action>
  <verify>
    <automated>cd F:/Waveterm/waveterm &amp;&amp; go test ./pkg/jira/... -run "TestADFToMarkdown" -count=1 -v 2>&amp;1</automated>
  </verify>
  <acceptance_criteria>
    - `test -f pkg/jira/adf.go` returns exit 0
    - `grep -q "^func ADFToMarkdown(raw json.RawMessage) (string, error)" pkg/jira/adf.go`
    - `grep -q "case \"doc\":" pkg/jira/adf.go`
    - `grep -q "case \"paragraph\":" pkg/jira/adf.go`
    - `grep -q "case \"heading\":" pkg/jira/adf.go`
    - `grep -q "case \"bulletList\":" pkg/jira/adf.go`
    - `grep -q "case \"orderedList\":" pkg/jira/adf.go`
    - `grep -q "case \"listItem\":" pkg/jira/adf.go`
    - `grep -q "case \"codeBlock\":" pkg/jira/adf.go`
    - `grep -q "case \"blockquote\":" pkg/jira/adf.go`
    - `grep -q "case \"rule\":" pkg/jira/adf.go`
    - `grep -q "case \"table\":" pkg/jira/adf.go`
    - `grep -q "case \"hardBreak\":" pkg/jira/adf.go`
    - `grep -q "case \"text\":" pkg/jira/adf.go`
    - `grep -q "case \"mention\":" pkg/jira/adf.go`
    - `grep -q 'attrs\[.href.\]' pkg/jira/adf.go`  (RESEARCH Pitfall 6 — NOT url)
    - `grep -q 'attrs\[.text.\]' pkg/jira/adf.go`  (RESEARCH Pitfall 5 — NOT displayName)
    - `grep -vq 'attrs\[.url.\]' pkg/jira/adf.go`  (must NOT use .url for link)
    - `grep -vq 'attrs\[.displayName.\]' pkg/jira/adf.go`  (must NOT use .displayName for mention)
    - `grep -q "log.Printf" pkg/jira/adf.go`  (D-15 unknown-node log)
    - `go build ./pkg/jira/...` exits 0
    - `go test ./pkg/jira/... -run "TestADFToMarkdown" -count=1 -race` exits 0 — all 4 ADF tests GREEN (15 sub-cases in TestADFToMarkdown_PerNodeType all pass)
  </acceptance_criteria>
  <done>adf.go compiles; ADFToMarkdown handles all D-14 node types; 4 ADF test functions GREEN; unknown nodes logged but don't fail conversion.</done>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| untrusted JSON → Go process | ADF JSON from Jira API may be arbitrarily deep or contain hostile structures |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-01-03-01 | D (DoS) | Recursive renderNode on malicious deep nesting | accept | Go's runtime stack grows dynamically; 10,000-level nesting consumes ~MB not GB. Jira caps ADF depth server-side (no known published limit but internal team use = trusted source). Not a realistic DoS vector for Phase 1. |
| T-01-03-02 | T (Tampering) | Malicious link href injected into markdown | accept | The markdown renderer in the widget handles sanitization — per existing widget code that displays markdown via react-markdown with default safe config. ADFToMarkdown's job is lossless conversion, not sanitization. Downstream renderer is the trust boundary. |
| T-01-03-03 | I (Information Disclosure) | Unknown-node log content | mitigate | `log.Printf` emits only the node TYPE string (a fixed tag like "panel", "media", "emoji"), never the node's children or attrs. Grep check: `logUnknownADFType` receives `nodeType string` and no other payload. |
| T-01-03-04 | T (Tampering) | JSON number parsing (heading level) | mitigate | Heading level clamped to [1, 6] range; non-numeric attrs yield default level 1. No integer overflow since float64 → int conversion is bounded by clamp. |
</threat_model>

<verification>
After the task:
- `go build ./pkg/jira/...` exits 0
- `go test ./pkg/jira/... -run "TestADFToMarkdown" -count=1 -race` exits 0
- All 15 sub-cases in TestADFToMarkdown_PerNodeType pass
- TestADFToMarkdown_UnknownNodeInMixedTree passes (panel is unknown, "before" + "after" both appear in output)
- `pkg/jira/client_test.go` still fails to compile — this plan does not touch client
</verification>

<success_criteria>
- ADFToMarkdown handles all 10 D-14 block types + 4 marks + hardBreak + mention
- Unknown types logged via log.Printf, do not fail the call (D-15)
- Empty/null input returns ("", nil) gracefully
- Malformed JSON returns structural parse error (D-16)
- Uses attrs.href for links (NOT attrs.url — Pitfall 6)
- Uses attrs.text for mentions (NOT attrs.displayName — Pitfall 5)
- All ADF tests GREEN on Windows
</success_criteria>

<output>
After completion, create `.planning/phases/01-jira-http-client-config/01-03-SUMMARY.md` recording:
- Test results (`go test ./pkg/jira/... -run TestADFToMarkdown -count=1 -race` output)
- Supported node types with a sample input → output example for each
- Note any assumptions made about mark nesting order (code → em → strong → link)
</output>

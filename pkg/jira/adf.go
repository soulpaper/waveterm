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
//	block:  doc, paragraph, heading (1-6), bulletList, orderedList, listItem,
//	        codeBlock, blockquote, rule, table, tableRow, tableHeader, tableCell
//	inline: text (+marks strong/em/code/link), hardBreak, mention
//
// Unknown node types (panel, media, inlineCard, emoji, ...) are silently
// skipped per D-15. The walker descends into their "content" array if present
// so sibling text still renders. A single log.Printf records each unknown
// type for debugging.
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
		// D-15: unknown node — log once per occurrence, descend into content.
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
// the inner string first (the raw text), then wrap it mark by mark. Order of
// application is deterministic: code → em → strong → link (innermost to
// outermost).
func renderText(sb *strings.Builder, node map[string]any) {
	txt, _ := node["text"].(string)
	if txt == "" {
		return
	}
	marks, _ := node["marks"].([]any)
	result := txt
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

// logUnknownADFType emits a debug log for each unknown ADF node type.
// Per D-15 this never fails the conversion — it's informational only. We do
// NOT deduplicate across calls (a package-level dedupe map would add mutex
// overhead); a single log per occurrence is acceptable and matches the
// codebase's stdlib "log" convention.
func logUnknownADFType(nodeType string) {
	log.Printf("jira: unknown ADF node type %q (silently skipping, descending into children)", nodeType)
}

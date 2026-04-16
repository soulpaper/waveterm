// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestADFToMarkdown_EmptyInput(t *testing.T) {
	// Empty raw JSON and literal null must return empty string, no error.
	for _, in := range []string{"", "null"} {
		out, err := ADFToMarkdown(json.RawMessage(in))
		if err != nil {
			t.Errorf("input %q: unexpected error %v", in, err)
		}
		if out != "" {
			t.Errorf("input %q: got %q want empty", in, out)
		}
	}
}

func TestADFToMarkdown_MalformedJSON(t *testing.T) {
	_, err := ADFToMarkdown(json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestADFToMarkdown_PerNodeType(t *testing.T) {
	// Table-driven per D-14. Each case supplies an ADF doc and a substring
	// that must appear in the markdown output. Full markdown assertions are
	// fleshed out in Plan 05 (TestFleshing); this stub only confirms the
	// converter runs and produces SOMETHING containing the expected marker.
	cases := []struct {
		name string
		adf  string
		want string
	}{
		{"paragraph", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hello"}]}]}`, "hello"},
		{"heading h2", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]}]}`, "## Title"},
		{"bulletList", `{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]}]}]}`, "- A"},
		{"orderedList", `{"type":"doc","content":[{"type":"orderedList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]}]}`, "1. A"},
		{"codeBlock", `{"type":"doc","content":[{"type":"codeBlock","attrs":{"language":"go"},"content":[{"type":"text","text":"x := 1"}]}]}`, "```go"},
		{"blockquote", `{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"Q"}]}]}]}`, "> Q"},
		{"rule", `{"type":"doc","content":[{"type":"rule"}]}`, "---"},
		{"hardBreak", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"},{"type":"hardBreak"},{"type":"text","text":"b"}]}]}`, "a\nb"},
		{"mark strong", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]}]}]}`, "**bold**"},
		{"mark em", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"it","marks":[{"type":"em"}]}]}]}`, "*it*"},
		{"mark code", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"x","marks":[{"type":"code"}]}]}]}`, "`x`"},
		{"mark link", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"go","marks":[{"type":"link","attrs":{"href":"https://go.dev"}}]}]}]}`, "[go](https://go.dev)"},
		{"mention with text", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"5b10","text":"@Bradley"}}]}]}`, "@Bradley"},
		{"mention fallback to id", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"mention","attrs":{"id":"5b10"}}]}]}`, "@5b10"},
		{"table with header row", `{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"H1"}]}]},{"type":"tableHeader","content":[{"type":"paragraph","content":[{"type":"text","text":"H2"}]}]}]},{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"c1"}]}]},{"type":"tableCell","content":[{"type":"paragraph","content":[{"type":"text","text":"c2"}]}]}]}]}]}`, "| H1 | H2 |"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ADFToMarkdown(json.RawMessage(tc.adf))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("output missing %q; full output:\n%s", tc.want, out)
			}
		})
	}
}

func TestADFToMarkdown_UnknownNodeInMixedTree(t *testing.T) {
	// D-15: unknown node types silently skipped; surrounding text still renders.
	// A "panel" node wraps a paragraph with real text. Panel is NOT in D-14;
	// converter must descend into its content (or skip it entirely while
	// preserving siblings). The "after" text must appear in output.
	adf := `{"type":"doc","content":[
		{"type":"paragraph","content":[{"type":"text","text":"before"}]},
		{"type":"panel","attrs":{"panelType":"info"},"content":[
			{"type":"paragraph","content":[{"type":"text","text":"inside-panel"}]}
		]},
		{"type":"paragraph","content":[{"type":"text","text":"after"}]}
	]}`
	out, err := ADFToMarkdown(json.RawMessage(adf))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "before") {
		t.Errorf("missing 'before' in output: %s", out)
	}
	if !strings.Contains(out, "after") {
		t.Errorf("missing 'after' in output: %s", out)
	}
	// The converter is allowed either to render "inside-panel" (descend into
	// unknown's children) or to drop the entire unknown subtree; both are
	// spec-compliant per D-15. We do NOT assert on "inside-panel" here.
}

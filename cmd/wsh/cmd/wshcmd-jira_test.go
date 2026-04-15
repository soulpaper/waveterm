// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// TestJiraCmdHelp asserts `wsh jira --help` lists the refresh subcommand. We
// call Help() directly on jiraCmd rather than Execute() because Execute() on a
// subcommand routes through its root and prints the root's help instead.
func TestJiraCmdHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	jiraCmd.SetOut(buf)
	jiraCmd.SetErr(buf)
	t.Cleanup(func() {
		jiraCmd.SetOut(nil)
		jiraCmd.SetErr(nil)
	})
	if err := jiraCmd.Help(); err != nil {
		t.Fatalf("jiraCmd.Help() returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "refresh") {
		t.Errorf("expected `jira` help output to contain \"refresh\", got:\n%s", out)
	}
	if !strings.Contains(out, "jira.json") {
		t.Errorf("expected `jira` help output to contain the long description mentioning jira.json, got:\n%s", out)
	}
}

// TestJiraRefreshHelp asserts `wsh jira refresh --help` lists flags.
func TestJiraRefreshHelp(t *testing.T) {
	buf := &bytes.Buffer{}
	jiraRefreshCmd.SetOut(buf)
	jiraRefreshCmd.SetErr(buf)
	t.Cleanup(func() {
		jiraRefreshCmd.SetOut(nil)
		jiraRefreshCmd.SetErr(nil)
	})
	if err := jiraRefreshCmd.Help(); err != nil {
		t.Fatalf("jiraRefreshCmd.Help() returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "--json") {
		t.Errorf("expected `jira refresh` help output to contain \"--json\", got:\n%s", out)
	}
	if !strings.Contains(out, "--timeout") {
		t.Errorf("expected `jira refresh` help output to contain \"--timeout\", got:\n%s", out)
	}
}

// TestJiraRefreshExitCodeMapping asserts D-ERR-04 exit-code rules.
func TestJiraRefreshExitCodeMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil returns 0", nil, 0},
		{"auth prefix returns 1", errors.New("인증 실패 — PAT 만료"), 1},
		{"config missing prefix returns 2", errors.New("설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."), 2},
		{"network error returns 3", errors.New("Jira 서버에 연결할 수 없습니다: dial tcp"), 3},
		{"other error returns 3", errors.New("refresh failed: disk full"), 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := exitCodeForError(tc.err)
			if got != tc.want {
				t.Errorf("exitCodeForError(%q) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestJiraRefreshExitCodeNoTokenLeak — T-03-06 guard: verify that mapping does
// not reflect token-like substrings differently (smoke check: the helper should
// only look at prefixes, never echo input).
func TestJiraRefreshExitCodeNoTokenLeak(t *testing.T) {
	// A pathological error string carrying what looks like a token.
	leakish := errors.New("random failure with ATATT3xFfGF0abcdef1234567890abcdef1234567890abcd secret-ish blob")
	got := exitCodeForError(leakish)
	if got != 3 {
		t.Errorf("expected exit 3 for unclassified error, got %d", got)
	}
}

// TestFormatRefreshSummary asserts D-CLI-02 output format with singular/plural
// and elapsed-time formatting cases.
func TestFormatRefreshSummary(t *testing.T) {
	cases := []struct {
		name string
		in   wshrpc.CommandJiraRefreshRtnData
		want string
	}{
		{
			name: "typical plural",
			in: wshrpc.CommandJiraRefreshRtnData{
				IssueCount:      23,
				AttachmentCount: 4,
				CommentCount:    107,
				ElapsedMs:       1234,
				CachePath:       "/home/user/.config/waveterm/jira-cache.json",
			},
			want: "Fetched 23 issues (4 attachments, 107 comments) in 1.2s → /home/user/.config/waveterm/jira-cache.json",
		},
		{
			name: "sub-second elapsed rounds to one decimal",
			in: wshrpc.CommandJiraRefreshRtnData{
				IssueCount:      1,
				AttachmentCount: 0,
				CommentCount:    0,
				ElapsedMs:       500,
				CachePath:       "C:\\Users\\me\\jira-cache.json",
			},
			want: "Fetched 1 issues (0 attachments, 0 comments) in 0.5s → C:\\Users\\me\\jira-cache.json",
		},
		{
			name: "over one minute elapsed",
			in: wshrpc.CommandJiraRefreshRtnData{
				IssueCount:      5000,
				AttachmentCount: 99,
				CommentCount:    2000,
				ElapsedMs:       61000,
				CachePath:       "/tmp/c.json",
			},
			want: "Fetched 5000 issues (99 attachments, 2000 comments) in 61.0s → /tmp/c.json",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatRefreshSummary(tc.in)
			if got != tc.want {
				t.Errorf("formatRefreshSummary mismatch:\n  got:  %q\n  want: %q", got, tc.want)
			}
		})
	}
}

// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package wshserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/wavetermdev/waveterm/pkg/jira"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// TestJiraRefreshCommand verifies error-class mapping (D-ERR-01) and success
// path for the JiraRefreshCommand handler. Tests swap the package-level
// seams jiraLoadConfig / jiraRefresh per D-TEST-01. T-01-02 is defended by
// asserting that neither the returned error nor any mapped output contains
// the planted SECRET_TOKEN literal.
func TestJiraRefreshCommand(t *testing.T) {
	// Exact D-ERR-01 user-facing strings (locked by REQUIREMENTS JIRA-04 +
	// CONTEXT D-ERR-01). Tests compare with == on the error.Error() value
	// so typos here or in the handler fail loudly.
	const (
		msgConfigMissing = "설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."
		msgUnauthorized  = "인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)"
		plantedToken     = "SECRET_TOKEN_12345"
	)

	ws := &WshServer{}

	// helper to restore seams after every subtest — never leak a mock into
	// the next subtest if the handler is modified to call Refresh twice.
	restoreSeams := func(t *testing.T) {
		t.Helper()
		origLoad := jiraLoadConfig
		origRefresh := jiraRefresh
		t.Cleanup(func() {
			jiraLoadConfig = origLoad
			jiraRefresh = origRefresh
		})
	}

	t.Run("success", func(t *testing.T) {
		restoreSeams(t)
		cfg := jira.Config{
			BaseUrl:  "https://example.atlassian.net",
			CloudId:  "cloud-id",
			Email:    "user@example.com",
			ApiToken: plantedToken,
			Jql:      "assignee=currentUser()",
			PageSize: 50,
		}
		jiraLoadConfig = func() (jira.Config, error) { return cfg, nil }
		report := &jira.RefreshReport{
			IssueCount:      23,
			AttachmentCount: 4,
			CommentCount:    107,
			Elapsed:         1250 * time.Millisecond,
			CachePath:       "/tmp/jira-cache.json",
		}
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			// Verify config propagates to Refresh
			if opts.Config.ApiToken != plantedToken {
				t.Errorf("refresh opts.Config.ApiToken mismatch")
			}
			return report, nil
		}

		startCall := time.Now()
		got, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.IssueCount != 23 || got.AttachmentCount != 4 || got.CommentCount != 107 {
			t.Errorf("counts mirror mismatch: got=%+v", got)
		}
		if got.ElapsedMs != report.Elapsed.Milliseconds() {
			t.Errorf("ElapsedMs = %d, want %d", got.ElapsedMs, report.Elapsed.Milliseconds())
		}
		if got.CachePath != report.CachePath {
			t.Errorf("CachePath = %q, want %q", got.CachePath, report.CachePath)
		}
		parsed, perr := time.Parse(time.RFC3339, got.FetchedAt)
		if perr != nil {
			t.Fatalf("FetchedAt not RFC3339: %q (%v)", got.FetchedAt, perr)
		}
		if parsed.Before(startCall.Add(-2*time.Second)) || parsed.After(time.Now().Add(2*time.Second)) {
			t.Errorf("FetchedAt %v outside expected window around %v", parsed, startCall)
		}
		// T-01-02 defense: no field should ever echo the token.
		if strings.Contains(fmt.Sprintf("%+v", got), plantedToken) {
			t.Errorf("return struct leaked token")
		}
	})

	t.Run("config_not_found", func(t *testing.T) {
		restoreSeams(t)
		jiraLoadConfig = func() (jira.Config, error) { return jira.Config{}, jira.ErrConfigNotFound }
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			t.Fatalf("refresh should not be called when LoadConfig fails")
			return nil, nil
		}
		got, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if err.Error() != msgConfigMissing {
			t.Errorf("error = %q, want %q", err.Error(), msgConfigMissing)
		}
		if (got != wshrpc.CommandJiraRefreshRtnData{}) {
			t.Errorf("expected zero-valued return, got %+v", got)
		}
	})

	t.Run("config_incomplete", func(t *testing.T) {
		// Phase 4 will differentiate incomplete vs missing; for now both map
		// to the same prefix so widget UX stays uniform (see handler comment).
		restoreSeams(t)
		wrapped := fmt.Errorf("%w: missing required fields: apiToken", jira.ErrConfigIncomplete)
		jiraLoadConfig = func() (jira.Config, error) { return jira.Config{}, wrapped }
		got, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.HasPrefix(err.Error(), "설정 파일이 없습니다") {
			t.Errorf("error = %q, want prefix %q", err.Error(), "설정 파일이 없습니다")
		}
		if (got != wshrpc.CommandJiraRefreshRtnData{}) {
			t.Errorf("expected zero-valued return, got %+v", got)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		restoreSeams(t)
		cfg := jira.Config{BaseUrl: "https://x", Email: "u", ApiToken: plantedToken}
		jiraLoadConfig = func() (jira.Config, error) { return cfg, nil }
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			// Simulate the real path: APIError{401} wraps ErrUnauthorized via Unwrap.
			return nil, fmt.Errorf("%w", jira.ErrUnauthorized)
		}
		_, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if err.Error() != msgUnauthorized {
			t.Errorf("error = %q, want %q", err.Error(), msgUnauthorized)
		}
		if strings.Contains(err.Error(), plantedToken) {
			t.Errorf("unauthorized error leaked token")
		}
	})

	t.Run("rate_limited", func(t *testing.T) {
		restoreSeams(t)
		jiraLoadConfig = func() (jira.Config, error) { return jira.Config{BaseUrl: "x", Email: "x", ApiToken: "x"}, nil }
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			return nil, fmt.Errorf("%w", jira.ErrRateLimited)
		}
		_, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		re := regexp.MustCompile(`Jira 서버가 요청을 제한했습니다.*잠시 후 다시 시도`)
		if !re.MatchString(err.Error()) {
			t.Errorf("error = %q, want match %v", err.Error(), re)
		}
	})

	t.Run("network", func(t *testing.T) {
		restoreSeams(t)
		jiraLoadConfig = func() (jira.Config, error) { return jira.Config{BaseUrl: "x", Email: "x", ApiToken: plantedToken}, nil }
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
		}
		_, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.HasPrefix(err.Error(), "Jira 서버에 연결할 수 없습니다:") {
			t.Errorf("error = %q, want prefix %q", err.Error(), "Jira 서버에 연결할 수 없습니다:")
		}
		if strings.Contains(strings.ToLower(err.Error()), "token") || strings.Contains(err.Error(), "apiToken") {
			t.Errorf("network error leaked token-related word: %q", err.Error())
		}
	})

	t.Run("generic", func(t *testing.T) {
		restoreSeams(t)
		jiraLoadConfig = func() (jira.Config, error) { return jira.Config{BaseUrl: "x", Email: "x", ApiToken: plantedToken}, nil }
		jiraRefresh = func(ctx context.Context, opts jira.RefreshOpts) (*jira.RefreshReport, error) {
			return nil, errors.New("disk full")
		}
		_, err := ws.JiraRefreshCommand(context.Background(), wshrpc.CommandJiraRefreshData{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if !strings.HasPrefix(err.Error(), "refresh failed:") {
			t.Errorf("error = %q, want prefix %q", err.Error(), "refresh failed:")
		}
		if !strings.Contains(err.Error(), "disk full") {
			t.Errorf("error = %q, want contain %q", err.Error(), "disk full")
		}
		if strings.Contains(err.Error(), plantedToken) || strings.Contains(err.Error(), "apiToken") {
			t.Errorf("generic error leaked token: %q", err.Error())
		}
	})
}

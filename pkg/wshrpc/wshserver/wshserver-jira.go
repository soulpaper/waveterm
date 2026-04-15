// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// wshserver-jira.go — JiraRefreshCommand handler.
//
// Security invariants referenced by every function in this file:
//
//   - CONTEXT D-ERR-01..04: user-facing strings are the exact Korean messages
//     the widget + CLI display verbatim. Do NOT translate or reword them
//     without updating CONTEXT and REQUIREMENTS (JIRA-04 locks the wording).
//   - CONTEXT D-ERR-02 + T-01-02: NEVER log raw pkg/jira errors (APIError.Body
//     is truncated to 1 KB but still could contain token-adjacent info) and
//     NEVER log the Config (holds apiToken).
//   - sanitizeErrMessage is a defense-in-depth scrubber: even if a future
//     pkg/jira change accidentally embeds a token-shaped string in err.Error(),
//     this function redacts it before the message crosses the RPC boundary.
//
// If you are editing this file, preserve the above invariants.

package wshserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/jira"
	"github.com/wavetermdev/waveterm/pkg/panichandler"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
)

// jiraLoadConfig and jiraRefresh are overridable seams for unit tests
// (D-TEST-01). Production code uses the real pkg/jira implementations.
var (
	jiraLoadConfig = jira.LoadConfig
	jiraRefresh    = jira.Refresh
)

// tokenLikeRegexp matches 20+ char runs of base64/JWT/token-shaped characters.
// Heuristic per T-03-01. The regex intentionally does NOT include spaces or
// punctuation that would appear in a real Korean error message, so wrapping
// messages like "Jira 서버에 연결할 수 없습니다:" are never trimmed.
var tokenLikeRegexp = regexp.MustCompile(`[A-Za-z0-9_=+/\-]{20,}`)

// JiraRefreshCommand triggers a synchronous refresh of the on-disk Jira cache
// via pkg/jira.Refresh. Returns a populated CommandJiraRefreshRtnData on
// success. On failure, returns a user-actionable Korean message per
// D-ERR-01; the original error is sanitized (see mapJiraError) so that
// callers rendering err.Error() directly never see the apiToken or APIError.Body.
func (ws *WshServer) JiraRefreshCommand(ctx context.Context, data wshrpc.CommandJiraRefreshData) (wshrpc.CommandJiraRefreshRtnData, error) {
	defer func() {
		panichandler.PanicHandler("JiraRefreshCommand", recover())
	}()
	started := time.Now()
	cfg, err := jiraLoadConfig()
	if err != nil {
		return wshrpc.CommandJiraRefreshRtnData{}, mapJiraError(err)
	}
	report, err := jiraRefresh(ctx, jira.RefreshOpts{Config: cfg})
	if err != nil {
		return wshrpc.CommandJiraRefreshRtnData{}, mapJiraError(err)
	}
	return wshrpc.CommandJiraRefreshRtnData{
		IssueCount:      report.IssueCount,
		AttachmentCount: report.AttachmentCount,
		CommentCount:    report.CommentCount,
		ElapsedMs:       report.Elapsed.Milliseconds(),
		CachePath:       report.CachePath,
		FetchedAt:       started.UTC().Format(time.RFC3339),
	}, nil
}

// mapJiraError translates a pkg/jira error into the exact user-facing Korean
// messages specified by D-ERR-01. Unknown errors fall through to a "refresh
// failed" wrapper that passes the underlying message through sanitizeErrMessage
// to redact any accidental token-shaped substrings.
//
// ErrConfigNotFound and ErrConfigIncomplete share a message for v1 — Phase 4's
// setup modal will differentiate the two UX paths. Both surface as "설정 파일이
// 없습니다" so the widget's empty-state CTA fires uniformly.
func mapJiraError(err error) error {
	switch {
	case errors.Is(err, jira.ErrConfigNotFound), errors.Is(err, jira.ErrConfigIncomplete):
		return fmt.Errorf("설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요.")
	case errors.Is(err, jira.ErrUnauthorized):
		return fmt.Errorf("인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)")
	case errors.Is(err, jira.ErrRateLimited):
		return fmt.Errorf("Jira 서버가 요청을 제한했습니다. 잠시 후 다시 시도하세요.")
	case isNetworkError(err):
		return fmt.Errorf("Jira 서버에 연결할 수 없습니다: %s", sanitizeErrMessage(err))
	default:
		return fmt.Errorf("refresh failed: %s", sanitizeErrMessage(err))
	}
}

// isNetworkError reports whether err looks like a transport-layer failure
// (TCP connect, TLS handshake, DNS, i/o timeout) rather than an HTTP-level
// response classified by pkg/jira. APIError is categorized by status code
// upstream and must NOT be treated as a network error — returning false for
// *APIError ensures those paths fall through to the generic branch or the
// status-specific sentinels (ErrRateLimited, ErrUnauthorized, ...).
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *jira.APIError
	if errors.As(err, &apiErr) {
		return false
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	msg := err.Error()
	if strings.Contains(msg, "dial tcp") || strings.Contains(msg, "i/o timeout") {
		return true
	}
	return false
}

// sanitizeErrMessage returns err.Error() with any 20+ character token-shaped
// substring replaced by <redacted>. This is defense-in-depth: pkg/jira.APIError
// already strips Body from its Error() implementation (T-01-02-01), but a
// future code path that wraps a config with %v or formats a raw request body
// into an error would otherwise leak the token. We assume no legitimate error
// message contains a 20+ char contiguous alphanumeric run.
func sanitizeErrMessage(err error) string {
	if err == nil {
		return ""
	}
	return tokenLikeRegexp.ReplaceAllString(err.Error(), "<redacted>")
}

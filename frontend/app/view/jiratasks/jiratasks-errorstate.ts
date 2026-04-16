// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0
//
// jiratasks-errorstate.ts — pure helpers for mapping backend error strings
// to visual widget states. String prefixes are LOCKED contracts with
// pkg/wshrpc/wshserver/wshserver-jira.go:mapJiraError (CONTEXT D-STATE-01..04).
// Do NOT translate or reword these prefixes without also updating CONTEXT
// and the Go handler.
//
// This file is intentionally React-free so it can be unit-tested without a DOM.

export type ErrorState = "setup" | "auth" | "network" | "unknown";

// Canonical Atlassian PAT management URL. Used by the "auth" card CTA.
export const ATLASSIAN_PAT_URL =
    "https://id.atlassian.com/manage-profile/security/api-tokens";

// Exact prompt locked by CONTEXT D-UI-03 (and mirrored in docs per D-DOC-02 §6).
// Any edit here MUST also update docs/docs/jira-widget.mdx to keep the two copies
// identical. Single-line, no leading/trailing whitespace — verified by test.
export const CLAUDE_SETUP_PROMPT =
    "~/.config/waveterm/jira.json 파일을 만들어줘. Atlassian site URL, cloudId, email, PAT(api token), JQL(assignee = currentUser() ORDER BY updated DESC), pageSize(50)을 물어봐서 채워줘. 파일은 권한 0600으로 저장.";

// classifyErrorMessage maps an error string from errorAtom to a visual state.
// Matches use startsWith so sanitized tails (network address, rate-limit detail)
// don't break classification. Returns null when there is no error to surface.
export function classifyErrorMessage(
    msg: string | null | undefined
): ErrorState | null {
    if (!msg) return null;

    // D-STATE-01 setup: missing config file OR empty cache (D-UI-04 variant).
    // Both lead to the same visual state with minor copy differences handled
    // by the caller using the raw prefix.
    if (msg.startsWith("설정 파일이 없습니다")) return "setup";
    if (msg.startsWith("캐시 파일이 비어있습니다")) return "setup";

    // D-STATE-02 auth: invalid / expired PAT.
    if (msg.startsWith("인증 실패")) return "auth";

    // D-STATE-03 network: transport-layer failures AND rate-limit throttling.
    // Rate-limit is grouped here because the remediation ("try again later")
    // is identical to a transient network failure.
    if (msg.startsWith("Jira 서버에 연결할 수 없습니다")) return "network";
    if (msg.startsWith("Jira 서버가 요청을 제한했습니다")) return "network";

    // D-STATE-04 unknown: anything else (incl. "refresh failed: ..." default
    // and the local "Jira 캐시를 읽을 수 없습니다" read-failure path).
    return "unknown";
}

// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { describe, it, expect } from "vitest";
import {
    classifyErrorMessage,
    CLAUDE_SETUP_PROMPT,
    ATLASSIAN_PAT_URL,
} from "./jiratasks-errorstate";

describe("classifyErrorMessage", () => {
    // Null / empty — nothing to show
    it("returns null for null input", () => {
        expect(classifyErrorMessage(null)).toBe(null);
    });

    it("returns null for empty string", () => {
        expect(classifyErrorMessage("")).toBe(null);
    });

    it("returns null for undefined input", () => {
        expect(classifyErrorMessage(undefined)).toBe(null);
    });

    // D-STATE-01 — setup (missing config)
    it("classifies missing config as setup", () => {
        expect(
            classifyErrorMessage(
                "설정 파일이 없습니다. Claude에게 jira 설정 생성을 요청하세요."
            )
        ).toBe("setup");
    });

    // D-UI-04 — empty-cache variant also maps to setup
    it("classifies empty cache as setup (D-UI-04)", () => {
        expect(
            classifyErrorMessage(
                "캐시 파일이 비어있습니다. Claude에게 'jira 이슈 새로고침'을 요청하세요."
            )
        ).toBe("setup");
    });

    // D-STATE-02 — auth (invalid PAT)
    it("classifies invalid auth as auth", () => {
        expect(
            classifyErrorMessage(
                "인증 실패 — ~/.config/waveterm/jira.json의 apiToken을 확인하세요 (PAT 만료/오타 가능)"
            )
        ).toBe("auth");
    });

    // D-STATE-03 — network (transport failure)
    it("classifies network error with sanitized tail as network", () => {
        expect(
            classifyErrorMessage(
                "Jira 서버에 연결할 수 없습니다: dial tcp 1.2.3.4:443 connect: connection refused"
            )
        ).toBe("network");
    });

    // D-STATE-03 — rate limit routes to network
    it("classifies rate limit as network", () => {
        expect(
            classifyErrorMessage(
                "Jira 서버가 요청을 제한했습니다. 잠시 후 다시 시도하세요."
            )
        ).toBe("network");
    });

    // D-STATE-04 — unknown default
    it("classifies refresh failed (default) as unknown", () => {
        expect(classifyErrorMessage("refresh failed: something weird")).toBe("unknown");
    });

    // Local cache read failure (widget-side) — unknown
    it("classifies local cache read failure as unknown", () => {
        expect(
            classifyErrorMessage(
                "Jira 캐시를 읽을 수 없습니다. Claude에게 'jira 이슈 새로고침'을 요청하세요."
            )
        ).toBe("unknown");
    });
});

describe("CLAUDE_SETUP_PROMPT (D-UI-03)", () => {
    it("is a non-empty single-line string", () => {
        expect(typeof CLAUDE_SETUP_PROMPT).toBe("string");
        expect(CLAUDE_SETUP_PROMPT.length).toBeGreaterThan(0);
        expect(CLAUDE_SETUP_PROMPT).not.toContain("\n");
        // No leading / trailing whitespace
        expect(CLAUDE_SETUP_PROMPT).toBe(CLAUDE_SETUP_PROMPT.trim());
    });

    it("references the target config file", () => {
        expect(CLAUDE_SETUP_PROMPT).toContain("jira.json 파일을 만들어줘");
    });

    it("specifies 0600 permissions", () => {
        expect(CLAUDE_SETUP_PROMPT).toContain("권한 0600으로 저장");
    });

    it("lists required fields", () => {
        // sanity — prompt must mention the key inputs Claude needs to ask for
        expect(CLAUDE_SETUP_PROMPT).toContain("cloudId");
        expect(CLAUDE_SETUP_PROMPT).toContain("PAT");
        expect(CLAUDE_SETUP_PROMPT).toContain("JQL");
    });
});

describe("ATLASSIAN_PAT_URL", () => {
    it("points at the canonical Atlassian API tokens page", () => {
        expect(ATLASSIAN_PAT_URL).toBe(
            "https://id.atlassian.com/manage-profile/security/api-tokens"
        );
    });
});

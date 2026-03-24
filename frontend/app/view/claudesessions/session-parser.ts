// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

export type SessionStatus = "incomplete" | "complete" | "abandoned";

export type ClaudeSessionInfo = {
    sessionId: string;
    projectPath: string;
    projectName: string;
    firstUserMessage: string;
    lastMessageType: string;
    lastMessagePreview: string;
    messageCount: number;
    modTime: number;
    status: SessionStatus;
    sessionName?: string;
};

const COMPLETE_KEYWORDS = [
    "완료",
    "끝",
    "커밋",
    "done",
    "complete",
    "finished",
    "commit",
    "pushed",
    "merged",
    "deployed",
];

export function determineSessionStatus(
    lastMessageType: string,
    lastAssistantText: string,
    messageCount: number
): SessionStatus {
    if (messageCount <= 2 && lastMessageType === "user") {
        return "abandoned";
    }
    if (lastMessageType === "user") {
        return "incomplete";
    }
    if (lastMessageType === "assistant" && lastAssistantText) {
        const lower = lastAssistantText.toLowerCase();
        for (const kw of COMPLETE_KEYWORDS) {
            if (lower.includes(kw)) {
                return "complete";
            }
        }
    }
    if (lastMessageType === "tool_use" || lastMessageType === "tool_result") {
        return "incomplete";
    }
    return "complete";
}

export function parseSessionJsonl(raw: string): {
    firstUserMessage: string;
    lastMessageType: string;
    lastAssistantText: string;
    messageCount: number;
    sessionName: string | null;
} {
    const lines = raw.split("\n").filter((l) => l.trim());
    let firstUserMessage = "";
    let lastMessageType = "";
    let lastAssistantText = "";
    let messageCount = 0;
    let sessionName: string | null = null;

    for (const line of lines) {
        try {
            const obj = JSON.parse(line);
            const type = obj.type;
            if (!type) continue;

            if (type === "user" || type === "assistant") {
                messageCount++;
            }

            if (type === "user" && !firstUserMessage) {
                const content = obj.message?.content;
                if (typeof content === "string") {
                    firstUserMessage = content.slice(0, 200).split("\n")[0];
                }
                // Check for session name from CLI --name flag
                if (obj.sessionName) {
                    sessionName = obj.sessionName;
                }
            }

            if (type === "user" || type === "assistant" || type === "tool_use" || type === "tool_result") {
                lastMessageType = type;
            }

            if (type === "assistant") {
                const content = obj.message?.content;
                if (Array.isArray(content)) {
                    for (const part of content) {
                        if (part.type === "text" && typeof part.text === "string") {
                            lastAssistantText = part.text.slice(0, 300);
                        }
                    }
                } else if (typeof content === "string") {
                    lastAssistantText = content.slice(0, 300);
                }
            }
        } catch {
            // skip malformed lines
        }
    }

    return { firstUserMessage, lastMessageType, lastAssistantText, messageCount, sessionName };
}

/**
 * Parse only the first and last portions of a large JSONL string.
 * Extracts the first user message from the head and the last message info from the tail.
 */
export function parseSessionJsonlPartial(
    headChunk: string,
    tailChunk: string
): {
    firstUserMessage: string;
    lastMessageType: string;
    lastAssistantText: string;
    messageCount: number;
    sessionName: string | null;
} {
    // Parse head for first user message
    const headResult = parseSessionJsonl(headChunk);

    // Parse tail for last message info
    const tailResult = parseSessionJsonl(tailChunk);

    return {
        firstUserMessage: headResult.firstUserMessage || tailResult.firstUserMessage,
        lastMessageType: tailResult.lastMessageType || headResult.lastMessageType,
        lastAssistantText: tailResult.lastAssistantText || headResult.lastAssistantText,
        messageCount: -1, // unknown for partial reads
        sessionName: headResult.sessionName,
    };
}

export function projectDirToName(dirName: string): string {
    // Convert "F--Waveterm" -> "F:/Waveterm", "C--Users-USER" -> "C:/Users/USER"
    return dirName.replace(/^([A-Za-z])--/, "$1:/").replace(/-/g, "/");
}

export function formatRelativeTime(timestamp: number): string {
    const now = Date.now();
    const diff = now - timestamp;
    const seconds = Math.floor(diff / 1000);
    const minutes = Math.floor(seconds / 60);
    const hours = Math.floor(minutes / 60);
    const days = Math.floor(hours / 24);

    if (days > 0) return `${days}일 전`;
    if (hours > 0) return `${hours}시간 전`;
    if (minutes > 0) return `${minutes}분 전`;
    return "방금 전";
}

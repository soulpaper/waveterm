// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BlockNodeModel } from "@/app/block/blocktypes";
import { createBlock, globalStore } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import type { TabModel } from "@/app/store/tab-model";
import { base64ToString, fireAndForget } from "@/util/util";
import clsx from "clsx";
import { atom, Atom, PrimitiveAtom, useAtomValue } from "jotai";
import { useEffect } from "react";
import {
    ClaudeSessionInfo,
    determineSessionStatus,
    formatRelativeTime,
    parseSessionJsonl,
    parseSessionJsonlPartial,
    projectDirToName,
    SessionStatus,
} from "./session-parser";
import "./claudesessions.scss";

const PARTIAL_READ_THRESHOLD = 100 * 1024; // 100KB
const HEAD_CHUNK_SIZE = 8 * 1024; // 8KB
const TAIL_CHUNK_SIZE = 16 * 1024; // 16KB

type FilterMode = "incomplete" | "all";

export class ClaudeSessionsViewModel implements ViewModel {
    viewType = "claudesessions";
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    viewIcon: Atom<string> = atom("clock-rotate-left");
    viewName: Atom<string> = atom("Sessions");
    viewComponent = ClaudeSessionsView;

    sessionsAtom: PrimitiveAtom<ClaudeSessionInfo[]> = atom<ClaudeSessionInfo[]>([]);
    loadingAtom: PrimitiveAtom<boolean> = atom(false);
    errorAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    filterAtom: PrimitiveAtom<FilterMode> = atom<FilterMode>("incomplete");

    endIconButtons: Atom<IconButtonDecl[]>;

    constructor({ blockId, nodeModel, tabModel }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;

        this.endIconButtons = atom<IconButtonDecl[]>([
            {
                elemtype: "iconbutton",
                icon: "rotate-right",
                title: "Refresh",
                click: () => {
                    fireAndForget(() => this.loadSessions());
                },
            },
        ]);
    }

    async loadSessions(): Promise<void> {
        globalStore.set(this.loadingAtom, true);
        globalStore.set(this.errorAtom, null);

        try {
            const projectsPath = "~/.claude/projects";
            const projectDirs = await RpcApi.FileListCommand(TabRpcClient, { path: projectsPath });

            if (!projectDirs || projectDirs.length === 0) {
                globalStore.set(this.sessionsAtom, []);
                return;
            }

            const allSessions: ClaudeSessionInfo[] = [];

            for (const projDir of projectDirs) {
                if (!projDir.isdir) continue;
                const projName = projDir.name;

                let files: FileInfo[];
                try {
                    files = await RpcApi.FileListCommand(TabRpcClient, {
                        path: `${projectsPath}/${projName}`,
                    });
                } catch {
                    continue;
                }
                if (!files || files.length === 0) continue;

                for (const file of files) {
                    if (!file.name?.endsWith(".jsonl")) continue;

                    const sessionId = file.name.replace(".jsonl", "");
                    const fileSize = file.size ?? 0;
                    const modTime = file.modtime ?? 0;

                    try {
                        let parsed;

                        if (fileSize > PARTIAL_READ_THRESHOLD) {
                            // Large file: read head + tail only
                            const headData = await RpcApi.FileReadCommand(TabRpcClient, {
                                info: { path: `${projectsPath}/${projName}/${file.name}` },
                                at: { offset: 0, size: HEAD_CHUNK_SIZE },
                            });
                            const tailOffset = Math.max(0, fileSize - TAIL_CHUNK_SIZE);
                            const tailData = await RpcApi.FileReadCommand(TabRpcClient, {
                                info: { path: `${projectsPath}/${projName}/${file.name}` },
                                at: { offset: tailOffset, size: TAIL_CHUNK_SIZE },
                            });

                            const headStr = headData.data64 ? base64ToString(headData.data64) : "";
                            const tailStr = tailData.data64 ? base64ToString(tailData.data64) : "";
                            parsed = parseSessionJsonlPartial(headStr, tailStr);
                        } else {
                            // Small file: read entirely
                            const fileData = await RpcApi.FileReadCommand(TabRpcClient, {
                                info: { path: `${projectsPath}/${projName}/${file.name}` },
                            });
                            const raw = fileData.data64 ? base64ToString(fileData.data64) : "";
                            parsed = parseSessionJsonl(raw);
                        }

                        if (!parsed.firstUserMessage && !parsed.sessionName) continue;

                        const status = determineSessionStatus(
                            parsed.lastMessageType,
                            parsed.lastAssistantText,
                            parsed.messageCount
                        );

                        allSessions.push({
                            sessionId,
                            projectPath: `${projectsPath}/${projName}`,
                            projectName: projectDirToName(projName),
                            firstUserMessage: parsed.firstUserMessage,
                            lastMessageType: parsed.lastMessageType,
                            lastMessagePreview: parsed.lastAssistantText?.slice(0, 100) || "",
                            messageCount: parsed.messageCount,
                            modTime,
                            status,
                            sessionName: parsed.sessionName ?? undefined,
                        });
                    } catch {
                        // skip unreadable sessions
                    }
                }
            }

            // Sort by modification time, newest first
            allSessions.sort((a, b) => b.modTime - a.modTime);
            globalStore.set(this.sessionsAtom, allSessions);
        } catch (err) {
            globalStore.set(this.errorAtom, `Failed to load sessions: ${err}`);
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    async resumeSession(session: ClaudeSessionInfo): Promise<void> {
        const blockDef: BlockDef = {
            meta: {
                view: "term",
                controller: "cmd",
                cmd: `claude -r ${session.sessionId}`,
                "cmd:runonstart": true,
                "cmd:interactive": true,
            },
        };
        await createBlock(blockDef);
    }

    toggleFilter(): void {
        const current = globalStore.get(this.filterAtom);
        globalStore.set(this.filterAtom, current === "incomplete" ? "all" : "incomplete");
    }
}

const STATUS_CONFIG: Record<SessionStatus, { label: string; icon: string; className: string }> = {
    incomplete: { label: "미완료", icon: "fa-circle-exclamation", className: "status-incomplete" },
    complete: { label: "완료", icon: "fa-circle-check", className: "status-complete" },
    abandoned: { label: "중단", icon: "fa-circle-xmark", className: "status-abandoned" },
};

function SessionCard({
    session,
    onResume,
}: {
    session: ClaudeSessionInfo;
    onResume: (s: ClaudeSessionInfo) => void;
}) {
    const statusConfig = STATUS_CONFIG[session.status];

    return (
        <div
            className={clsx("session-card", statusConfig.className)}
            onClick={() => onResume(session)}
            title={`Click to resume: ${session.sessionId}`}
        >
            <div className="session-card-header">
                <div className="session-status-badge">
                    <i className={clsx("fa-solid", statusConfig.icon)} />
                    <span>{statusConfig.label}</span>
                </div>
                <span className="session-time">{formatRelativeTime(session.modTime)}</span>
            </div>
            <div className="session-topic">
                {session.sessionName || session.firstUserMessage || "(empty session)"}
            </div>
            <div className="session-meta">
                <span className="session-project">
                    <i className="fa-solid fa-folder" />
                    {session.projectName}
                </span>
                {session.messageCount > 0 && (
                    <span className="session-msg-count">
                        <i className="fa-solid fa-message" />
                        {session.messageCount}
                    </span>
                )}
            </div>
        </div>
    );
}

function ClaudeSessionsView({ model }: { model: ClaudeSessionsViewModel }) {
    const sessions = useAtomValue(model.sessionsAtom);
    const loading = useAtomValue(model.loadingAtom);
    const error = useAtomValue(model.errorAtom);
    const filter = useAtomValue(model.filterAtom);

    useEffect(() => {
        fireAndForget(() => model.loadSessions());
    }, []);

    const filteredSessions = filter === "incomplete" ? sessions.filter((s) => s.status !== "complete") : sessions;

    const handleResume = (session: ClaudeSessionInfo) => {
        fireAndForget(() => model.resumeSession(session));
    };

    return (
        <div className="claudesessions-view">
            <div className="claudesessions-toolbar">
                <button
                    className={clsx("filter-btn", filter === "incomplete" && "active")}
                    onClick={() => model.toggleFilter()}
                >
                    {filter === "incomplete" ? "미완료만" : "전체"}
                </button>
                <span className="session-count">
                    {filteredSessions.length}개 세션
                    {filter === "incomplete" && sessions.length !== filteredSessions.length && (
                        <span className="total-count"> / {sessions.length}개 전체</span>
                    )}
                </span>
            </div>

            <div className="claudesessions-list">
                {loading && sessions.length === 0 && (
                    <div className="claudesessions-empty">
                        <i className="fa-solid fa-spinner fa-spin" />
                        <span>세션 로딩 중...</span>
                    </div>
                )}
                {error && (
                    <div className="claudesessions-error">
                        <i className="fa-solid fa-triangle-exclamation" />
                        <span>{error}</span>
                    </div>
                )}
                {!loading && !error && filteredSessions.length === 0 && (
                    <div className="claudesessions-empty">
                        <i className="fa-solid fa-circle-check" />
                        <span>{filter === "incomplete" ? "미완료 세션이 없습니다" : "세션이 없습니다"}</span>
                    </div>
                )}
                {filteredSessions.map((session) => (
                    <SessionCard key={session.sessionId} session={session} onResume={handleResume} />
                ))}
            </div>
        </div>
    );
}

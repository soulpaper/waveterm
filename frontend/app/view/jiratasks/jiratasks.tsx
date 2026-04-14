// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BlockNodeModel } from "@/app/block/blocktypes";
import { createBlock, globalStore } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import type { TabModel } from "@/app/store/tab-model";
import { getLayoutModelForStaticTab } from "@/layout/index";
import { base64ToString, stringToBase64, fireAndForget } from "@/util/util";
import clsx from "clsx";
import { atom, Atom, PrimitiveAtom, useAtomValue } from "jotai";
import { useCallback, useEffect } from "react";
import "./jiratasks.scss";

const CACHE_PATH = "~/.config/waveterm/jira-cache.json";

interface JiraIssue {
    key: string;
    id: string;
    summary: string;
    status: string;
    statusCategory: string;
    issueType: string;
    priority: string;
    projectKey: string;
    projectName: string;
    updated: string;
    created: string;
    webUrl: string;
}

interface JiraCache {
    cloudId: string;
    baseUrl: string;
    accountId: string;
    fetchedAt: string;
    issues: JiraIssue[];
}

type LaunchMode = "new" | "current";

const PRIORITY_ICONS: Record<string, string> = {
    "최상": "fa-angles-up",
    "높음": "fa-angle-up",
    "보통": "fa-minus",
    "낮음": "fa-angle-down",
    "최하": "fa-angles-down",
    "Highest": "fa-angles-up",
    "High": "fa-angle-up",
    "Medium": "fa-minus",
    "Low": "fa-angle-down",
    "Lowest": "fa-angles-down",
};

const STATUS_CATEGORY_CLASS: Record<string, string> = {
    "new": "status-todo",
    "indeterminate": "status-inprogress",
    "done": "status-done",
};

function formatUpdatedTime(isoStr: string): string {
    if (!isoStr) return "";
    const date = new Date(isoStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    if (diffMins < 1) return "방금";
    if (diffMins < 60) return `${diffMins}분 전`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}시간 전`;
    const diffDays = Math.floor(diffHours / 24);
    if (diffDays < 30) return `${diffDays}일 전`;
    return date.toLocaleDateString("ko-KR");
}

export class JiraTasksViewModel implements ViewModel {
    viewType = "jiratasks";
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    viewIcon: Atom<string> = atom("list-check");
    viewName: Atom<string> = atom("Jira Tasks");
    viewComponent = JiraTasksView;

    issuesAtom: PrimitiveAtom<JiraIssue[]> = atom<JiraIssue[]>([]);
    loadingAtom: PrimitiveAtom<boolean> = atom(false);
    errorAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    launchModeAtom: PrimitiveAtom<LaunchMode> = atom<LaunchMode>("new");
    fetchedAtAtom: PrimitiveAtom<string> = atom("");

    endIconButtons: Atom<IconButtonDecl[]>;

    constructor({ blockId, nodeModel, tabModel }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;

        this.endIconButtons = atom<IconButtonDecl[]>([
            {
                elemtype: "iconbutton",
                icon: "rotate-right",
                title: "Refresh cache",
                click: () => {
                    fireAndForget(() => this.loadFromCache());
                },
            },
        ]);
    }

    async loadFromCache(): Promise<void> {
        globalStore.set(this.loadingAtom, true);
        globalStore.set(this.errorAtom, null);

        try {
            const fileData = await RpcApi.FileReadCommand(TabRpcClient, {
                info: { path: CACHE_PATH },
            });
            const raw = fileData.data64 ? base64ToString(fileData.data64) : "";
            if (!raw) {
                globalStore.set(this.errorAtom, "캐시 파일이 비어있습니다. Claude에게 'jira 이슈 새로고침'을 요청하세요.");
                return;
            }
            const cache: JiraCache = JSON.parse(raw);
            globalStore.set(this.issuesAtom, cache.issues || []);
            globalStore.set(this.fetchedAtAtom, cache.fetchedAt || "");
        } catch (err) {
            globalStore.set(this.errorAtom, "Jira 캐시를 읽을 수 없습니다. Claude에게 'jira 이슈 새로고침'을 요청하세요.");
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    async openIssueInNewTerminal(issue: JiraIssue): Promise<void> {
        const prompt = `Jira 이슈를 분석해줘.\n\nKey: ${issue.key}\nSummary: ${issue.summary}\nStatus: ${issue.status}\nPriority: ${issue.priority}\nProject: ${issue.projectKey}\nURL: ${issue.webUrl}\n\n이 이슈의 상세 내용을 Jira에서 가져와서 분석하고, 관련 코드가 있으면 찾아서 설명해줘.`;

        const blockDef: BlockDef = {
            meta: {
                view: "term",
                controller: "cmd",
                cmd: `claude "${prompt.replace(/"/g, '\\"')}"`,
                "cmd:runonstart": true,
                "cmd:interactive": true,
            },
        };
        await createBlock(blockDef);
    }

    async openIssueInCurrentTerminal(issue: JiraIssue): Promise<void> {
        const layoutModel = getLayoutModelForStaticTab();
        const focusedNode = globalStore.get(layoutModel.focusedNode);
        const focusedBlockId = focusedNode?.data?.blockId;

        if (!focusedBlockId) {
            // fallback: open in new terminal
            await this.openIssueInNewTerminal(issue);
            return;
        }

        const prompt = `Jira 이슈를 분석해줘. Key: ${issue.key}, Summary: ${issue.summary}, Status: ${issue.status}, Priority: ${issue.priority}, Project: ${issue.projectKey}, URL: ${issue.webUrl}. 이 이슈의 상세 내용을 Jira에서 가져와서 분석하고, 관련 코드가 있으면 찾아서 설명해줘.`;
        const cmd = `claude "${prompt.replace(/"/g, '\\"')}"\n`;

        await RpcApi.ControllerInputCommand(TabRpcClient, {
            blockid: focusedBlockId,
            inputdata64: stringToBase64(cmd),
        });
    }

    async openIssue(issue: JiraIssue): Promise<void> {
        const mode = globalStore.get(this.launchModeAtom);
        if (mode === "current") {
            await this.openIssueInCurrentTerminal(issue);
        } else {
            await this.openIssueInNewTerminal(issue);
        }
    }

    toggleLaunchMode(): void {
        const current = globalStore.get(this.launchModeAtom);
        globalStore.set(this.launchModeAtom, current === "new" ? "current" : "new");
    }
}

function IssueCard({
    issue,
    onOpen,
}: {
    issue: JiraIssue;
    onOpen: (issue: JiraIssue) => void;
}) {
    const statusClass = STATUS_CATEGORY_CLASS[issue.statusCategory] || "status-todo";
    const priorityIcon = PRIORITY_ICONS[issue.priority] || "fa-minus";

    return (
        <div
            className={clsx("jira-issue-card", statusClass)}
            onClick={() => onOpen(issue)}
            title={`${issue.key}: ${issue.summary}`}
        >
            <div className="issue-card-header">
                <span className="issue-key">{issue.key}</span>
                <span className="issue-time">{formatUpdatedTime(issue.updated)}</span>
            </div>
            <div className="issue-summary">{issue.summary}</div>
            <div className="issue-meta">
                <span className="issue-status">
                    <span className={clsx("status-dot", statusClass)} />
                    {issue.status}
                </span>
                <span className="issue-priority">
                    <i className={clsx("fa-solid", priorityIcon)} />
                    {issue.priority}
                </span>
                <span className="issue-type">
                    {issue.issueType}
                </span>
            </div>
        </div>
    );
}

function JiraTasksView({ model }: { model: JiraTasksViewModel }) {
    const issues = useAtomValue(model.issuesAtom);
    const loading = useAtomValue(model.loadingAtom);
    const error = useAtomValue(model.errorAtom);
    const launchMode = useAtomValue(model.launchModeAtom);
    const fetchedAt = useAtomValue(model.fetchedAtAtom);

    useEffect(() => {
        fireAndForget(() => model.loadFromCache());
    }, []);

    const handleOpen = useCallback((issue: JiraIssue) => {
        fireAndForget(() => model.openIssue(issue));
    }, [model]);

    return (
        <div className="jiratasks-view">
            <div className="jiratasks-toolbar">
                <div className="toolbar-left">
                    <span className="issue-count">{issues.length}개 이슈</span>
                    {fetchedAt && (
                        <span className="fetched-at" title={fetchedAt}>
                            {formatUpdatedTime(fetchedAt)} 동기화
                        </span>
                    )}
                </div>
                <div className="toolbar-right">
                    <button
                        className={clsx("mode-toggle", launchMode === "new" && "mode-new", launchMode === "current" && "mode-current")}
                        onClick={() => model.toggleLaunchMode()}
                        title={launchMode === "new" ? "새 터미널에서 열기" : "현재 터미널에서 열기"}
                    >
                        <i className={clsx("fa-solid", launchMode === "new" ? "fa-plus" : "fa-arrow-right")} />
                        {launchMode === "new" ? "새 터미널" : "현재 터미널"}
                    </button>
                </div>
            </div>

            <div className="jiratasks-list">
                {loading && issues.length === 0 && (
                    <div className="jiratasks-empty">
                        <i className="fa-solid fa-spinner fa-spin" />
                        <span>로딩 중...</span>
                    </div>
                )}
                {error && (
                    <div className="jiratasks-error">
                        <i className="fa-solid fa-triangle-exclamation" />
                        <span>{error}</span>
                    </div>
                )}
                {!loading && !error && issues.length === 0 && (
                    <div className="jiratasks-empty">
                        <i className="fa-solid fa-circle-check" />
                        <span>할당된 이슈가 없습니다</span>
                    </div>
                )}
                {issues.map((issue) => (
                    <IssueCard key={issue.key} issue={issue} onOpen={handleOpen} />
                ))}
            </div>
        </div>
    );
}

// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BlockNodeModel } from "@/app/block/blocktypes";
import { createBlock, getBlockMetaKeyAtom, globalStore } from "@/app/store/global";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import type { TabModel } from "@/app/store/tab-model";
import { getLayoutModelForStaticTab } from "@/layout/index";
import { base64ToString, stringToBase64, fireAndForget } from "@/util/util";
import clsx from "clsx";
import { atom, Atom, PrimitiveAtom, useAtomValue } from "jotai";
import { useCallback, useEffect, useRef } from "react";
import "./jiratasks.scss";

interface TerminalOption {
    blockId: string;
    label: string;
}

type DateFilter = "all" | "24h" | "7d" | "30d";
type StatusCategory = "new" | "indeterminate" | "done";

interface ProjectOption {
    key: string;
    name: string;
    count: number;
}

interface JiraPrefs {
    projectFilter: string | null;
    statusFilter: StatusCategory[];
    dateFilter: DateFilter;
    refreshInterval: number;
    lastSeenAt: string;
    analyzeSkill: string;
    analyzeCli: string;
}

const DEFAULT_PREFS: JiraPrefs = {
    projectFilter: null,
    statusFilter: ["new", "indeterminate"],
    dateFilter: "all",
    refreshInterval: 0,
    lastSeenAt: "",
    analyzeSkill: "",
    analyzeCli: "claude",
};

const SKILL_SUGGESTIONS: string[] = [
    "/gsd-explore",
    "/gsd-debug",
    "/gsd-plan-phase",
    "/gsd-research-phase",
    "/gsd-audit-fix",
    "/gsd-quick",
    "/gsd-fast",
    "/gsd-do",
];

const CACHE_PATH = "~/.config/waveterm/jira-cache.json";
const ATTACHMENTS_DIR = "~/.config/waveterm/jira-attachments";
const PREFS_KEY_PREFIX = "jiratasks:prefs:";

const REFRESH_OPTIONS: { value: number; label: string }[] = [
    { value: 0, label: "끔" },
    { value: 60, label: "1분" },
    { value: 300, label: "5분" },
    { value: 900, label: "15분" },
    { value: 1800, label: "30분" },
    { value: 3600, label: "1시간" },
];

const DATE_OPTIONS: { value: DateFilter; label: string }[] = [
    { value: "all", label: "전체 기간" },
    { value: "24h", label: "24시간" },
    { value: "7d", label: "7일" },
    { value: "30d", label: "30일" },
];

const STATUS_LABELS: Record<StatusCategory, string> = {
    "new": "할 일",
    "indeterminate": "진행 중",
    "done": "완료",
};

function dateFilterCutoff(filter: DateFilter): number | null {
    const now = Date.now();
    switch (filter) {
        case "24h": return now - 24 * 3600 * 1000;
        case "7d": return now - 7 * 24 * 3600 * 1000;
        case "30d": return now - 30 * 24 * 3600 * 1000;
        default: return null;
    }
}

function loadPrefs(blockId: string): JiraPrefs {
    try {
        const raw = localStorage.getItem(PREFS_KEY_PREFIX + blockId);
        if (!raw) return { ...DEFAULT_PREFS };
        const parsed = JSON.parse(raw);
        return { ...DEFAULT_PREFS, ...parsed };
    } catch {
        return { ...DEFAULT_PREFS };
    }
}

function savePrefs(blockId: string, prefs: JiraPrefs): void {
    try {
        localStorage.setItem(PREFS_KEY_PREFIX + blockId, JSON.stringify(prefs));
    } catch {
        // quota or unavailable — ignore
    }
}

interface JiraAttachment {
    id: string;
    filename: string;
    mimeType: string;
    size: number;
    localPath: string;
    webUrl: string;
}

interface JiraComment {
    id: string;
    author: string;
    created: string;
    updated: string;
    body: string;
    truncated?: boolean;
}

interface JiraIssue {
    key: string;
    id: string;
    summary: string;
    description: string;
    status: string;
    statusCategory: string;
    issueType: string;
    priority: string;
    projectKey: string;
    projectName: string;
    updated: string;
    created: string;
    webUrl: string;
    attachments: JiraAttachment[];
    comments: JiraComment[];
    commentCount: number;
    lastCommentAt: string;
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

function formatFileSize(bytes: number): string {
    if (!bytes) return "";
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`;
}

function attachmentIcon(mimeType: string, filename: string): string {
    const mt = (mimeType || "").toLowerCase();
    const ext = (filename.split(".").pop() || "").toLowerCase();
    if (mt.startsWith("image/")) return "fa-file-image";
    if (mt.startsWith("video/")) return "fa-file-video";
    if (mt.startsWith("audio/")) return "fa-file-audio";
    if (mt.includes("pdf") || ext === "pdf") return "fa-file-pdf";
    if (mt.includes("zip") || ["zip", "tar", "gz", "7z", "rar"].includes(ext)) return "fa-file-zipper";
    if (["md", "txt", "log"].includes(ext)) return "fa-file-lines";
    if (["js", "ts", "tsx", "jsx", "go", "py", "rs", "java", "cpp", "c", "h", "sh", "json", "yaml", "yml"].includes(ext)) return "fa-file-code";
    return "fa-file";
}

export class JiraTasksViewModel implements ViewModel {
    viewType = "jiratasks";
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    viewIcon: Atom<string>;
    viewName: Atom<string>;
    viewComponent = JiraTasksView;

    issuesAtom: PrimitiveAtom<JiraIssue[]> = atom<JiraIssue[]>([]);
    loadingAtom: PrimitiveAtom<boolean> = atom(false);
    errorAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    // D-UI-02: post-hoc refresh summary, auto-clears after 5s.
    refreshProgressAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    launchModeAtom: PrimitiveAtom<LaunchMode> = atom<LaunchMode>("current");
    fetchedAtAtom: PrimitiveAtom<string> = atom("");
    expandedKeyAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    targetBlockIdAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;

    projectFilterAtom: PrimitiveAtom<string | null> = atom<string | null>(null) as PrimitiveAtom<string | null>;
    statusFilterAtom: PrimitiveAtom<StatusCategory[]> = atom<StatusCategory[]>(["new", "indeterminate"]);
    dateFilterAtom: PrimitiveAtom<DateFilter> = atom<DateFilter>("all");
    refreshIntervalAtom: PrimitiveAtom<number> = atom(0);
    lastSeenAtAtom: PrimitiveAtom<string> = atom("");
    filtersOpenAtom: PrimitiveAtom<boolean> = atom(false);
    analyzeSkillAtom: PrimitiveAtom<string> = atom("");
    analyzeCliAtom: PrimitiveAtom<string> = atom("claude");
    extraPromptAtom: PrimitiveAtom<string> = atom("");

    availableProjectsAtom: Atom<ProjectOption[]>;
    filteredIssuesAtom: Atom<JiraIssue[]>;
    newIssuesCountAtom: Atom<number>;

    terminalsAtom: Atom<TerminalOption[]> = atom((get) => {
        const layoutModel = getLayoutModelForStaticTab();
        if (!layoutModel) return [];
        const order = get(layoutModel.leafOrder);
        const results: TerminalOption[] = [];
        let autoIdx = 0;
        for (const entry of order || []) {
            const view = get(getBlockMetaKeyAtom(entry.blockid, "view"));
            if (view !== "term") continue;
            autoIdx++;
            const displayName = get(getBlockMetaKeyAtom(entry.blockid, "display:name" as any)) as string | undefined;
            const frameTitle = get(getBlockMetaKeyAtom(entry.blockid, "frame:title" as any)) as string | undefined;
            const cwd = get(getBlockMetaKeyAtom(entry.blockid, "cmd:cwd")) as string | undefined;
            const cmd = get(getBlockMetaKeyAtom(entry.blockid, "cmd")) as string | undefined;
            const controller = get(getBlockMetaKeyAtom(entry.blockid, "controller")) as string | undefined;
            const connection = get(getBlockMetaKeyAtom(entry.blockid, "connection")) as string | undefined;
            const short = entry.blockid.slice(0, 6);

            const cwdBase = cwd ? cwd.replace(/\\/g, "/").replace(/\/$/, "").split("/").filter(Boolean).pop() : "";
            const cmdShort = cmd ? (cmd.length > 24 ? cmd.slice(0, 21) + "…" : cmd) : "";
            const primary = displayName || frameTitle || cmdShort || cwdBase || (controller === "shell" ? "shell" : "term");
            const secondary = [connection && connection !== "local" ? `@${connection}` : "", cwdBase && primary !== cwdBase ? cwdBase : ""]
                .filter(Boolean).join(" ");

            const label = `#${autoIdx} ${primary}${secondary ? " · " + secondary : ""} · ${short}`;
            results.push({ blockId: entry.blockid, label });
        }
        return results;
    });

    endIconButtons: Atom<IconButtonDecl[]>;

    constructor({ blockId, nodeModel, tabModel }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;

        const prefs = loadPrefs(blockId);
        globalStore.set(this.projectFilterAtom, prefs.projectFilter);
        globalStore.set(this.statusFilterAtom, prefs.statusFilter);
        globalStore.set(this.dateFilterAtom, prefs.dateFilter);
        globalStore.set(this.refreshIntervalAtom, prefs.refreshInterval);
        globalStore.set(this.lastSeenAtAtom, prefs.lastSeenAt);
        globalStore.set(this.analyzeSkillAtom, prefs.analyzeSkill);
        globalStore.set(this.analyzeCliAtom, prefs.analyzeCli);

        this.availableProjectsAtom = atom((get) => {
            const issues = get(this.issuesAtom);
            const map = new Map<string, ProjectOption>();
            for (const i of issues) {
                const key = i.projectKey || "";
                if (!key) continue;
                const existing = map.get(key);
                if (existing) existing.count++;
                else map.set(key, { key, name: i.projectName || key, count: 1 });
            }
            return Array.from(map.values()).sort((a, b) => b.count - a.count);
        });

        this.filteredIssuesAtom = atom((get) => {
            const issues = get(this.issuesAtom);
            const project = get(this.projectFilterAtom);
            const statuses = get(this.statusFilterAtom);
            const dateFilter = get(this.dateFilterAtom);
            const cutoff = dateFilterCutoff(dateFilter);
            const statusSet = new Set(statuses);
            return issues.filter((i) => {
                if (project && i.projectKey !== project) return false;
                if (statusSet.size > 0 && !statusSet.has(i.statusCategory as StatusCategory)) return false;
                if (cutoff !== null) {
                    const t = Date.parse(i.updated || "");
                    if (!Number.isFinite(t) || t < cutoff) return false;
                }
                return true;
            });
        });

        this.newIssuesCountAtom = atom((get) => {
            const issues = get(this.issuesAtom);
            const lastSeen = get(this.lastSeenAtAtom);
            if (!lastSeen) return 0;
            const lastSeenMs = Date.parse(lastSeen);
            if (!Number.isFinite(lastSeenMs)) return 0;
            let count = 0;
            for (const i of issues) {
                const t = Date.parse(i.updated || "");
                if (Number.isFinite(t) && t > lastSeenMs) count++;
            }
            return count;
        });

        this.viewIcon = atom((get) => (get(this.newIssuesCountAtom) > 0 ? "bell" : "list-check"));
        this.viewName = atom((get) => {
            const n = get(this.newIssuesCountAtom);
            return n > 0 ? `Jira Tasks · ${n} new` : "Jira Tasks";
        });

        this.endIconButtons = atom<IconButtonDecl[]>((get) => {
            const newCount = get(this.newIssuesCountAtom);
            const filtersOpen = get(this.filtersOpenAtom);
            const buttons: IconButtonDecl[] = [];
            if (newCount > 0) {
                buttons.push({
                    elemtype: "iconbutton",
                    icon: "check",
                    title: `신규 ${newCount}개 확인`,
                    click: () => this.markAllRead(),
                });
            }
            buttons.push({
                elemtype: "iconbutton",
                icon: "filter",
                title: filtersOpen ? "필터 닫기" : "필터 열기",
                click: () => this.toggleFilters(),
            });
            buttons.push({
                elemtype: "iconbutton",
                icon: "cloud-arrow-down",
                title: "Jira에서 새로고침",
                click: () => fireAndForget(() => this.requestJiraRefresh()),
            });
            buttons.push({
                elemtype: "iconbutton",
                icon: "rotate-right",
                title: "캐시 다시 읽기",
                click: () => fireAndForget(() => this.loadFromCache()),
            });
            return buttons;
        });
    }

    async requestJiraRefresh(): Promise<void> {
        // WR-01: guard against concurrent refresh calls (e.g., double-click on ☁️ button).
        // Two in-flight JiraRefreshCommand RPCs would race on ~/.config/waveterm/jira-cache.json.
        if (globalStore.get(this.loadingAtom)) {
            return;
        }
        globalStore.set(this.loadingAtom, true);
        globalStore.set(this.errorAtom, null);
        try {
            const rtn = await RpcApi.JiraRefreshCommand(TabRpcClient, {});
            // Success: reload cache so issuesAtom reflects new data, then surface a summary.
            await this.loadFromCache();
            const elapsedSec = (rtn.elapsedms / 1000).toFixed(1);
            const summary = `${rtn.issuecount} 이슈 · ${elapsedSec}s`;
            globalStore.set(this.refreshProgressAtom, summary);
            // Auto-clear after 5s per D-UI-02.
            setTimeout(() => {
                // Guard: don't clear a newer refresh's summary.
                if (globalStore.get(this.refreshProgressAtom) === summary) {
                    globalStore.set(this.refreshProgressAtom, null);
                }
            }, 5000);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : String(err);
            globalStore.set(this.errorAtom, msg);
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    private persistPrefs(): void {
        const prefs: JiraPrefs = {
            projectFilter: globalStore.get(this.projectFilterAtom),
            statusFilter: globalStore.get(this.statusFilterAtom),
            dateFilter: globalStore.get(this.dateFilterAtom),
            refreshInterval: globalStore.get(this.refreshIntervalAtom),
            lastSeenAt: globalStore.get(this.lastSeenAtAtom),
            analyzeSkill: globalStore.get(this.analyzeSkillAtom),
            analyzeCli: globalStore.get(this.analyzeCliAtom),
        };
        savePrefs(this.blockId, prefs);
    }

    setAnalyzeSkill(v: string): void {
        globalStore.set(this.analyzeSkillAtom, v);
        this.persistPrefs();
    }

    setAnalyzeCli(v: string): void {
        globalStore.set(this.analyzeCliAtom, v);
        this.persistPrefs();
    }

    setExtraPrompt(v: string): void {
        globalStore.set(this.extraPromptAtom, v);
    }

    setProjectFilter(key: string | null): void {
        globalStore.set(this.projectFilterAtom, key);
        this.persistPrefs();
    }

    toggleStatus(cat: StatusCategory): void {
        const current = globalStore.get(this.statusFilterAtom);
        const next = current.includes(cat) ? current.filter((c) => c !== cat) : [...current, cat];
        globalStore.set(this.statusFilterAtom, next);
        this.persistPrefs();
    }

    setDateFilter(f: DateFilter): void {
        globalStore.set(this.dateFilterAtom, f);
        this.persistPrefs();
    }

    setRefreshInterval(secs: number): void {
        globalStore.set(this.refreshIntervalAtom, secs);
        this.persistPrefs();
    }

    toggleFilters(): void {
        globalStore.set(this.filtersOpenAtom, !globalStore.get(this.filtersOpenAtom));
    }

    markAllRead(): void {
        const issues = globalStore.get(this.issuesAtom);
        let maxT = 0;
        for (const i of issues) {
            const t = Date.parse(i.updated || "");
            if (Number.isFinite(t) && t > maxT) maxT = t;
        }
        const ts = maxT > 0 ? new Date(maxT).toISOString() : new Date().toISOString();
        globalStore.set(this.lastSeenAtAtom, ts);
        this.persistPrefs();
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
            const issues = (cache.issues || []).map((i) => {
                const comments = i.comments || [];
                const lastCommentAt = i.lastCommentAt
                    || (comments.length > 0 ? comments[comments.length - 1].updated || comments[comments.length - 1].created : "");
                return {
                    ...i,
                    description: i.description || "",
                    attachments: i.attachments || [],
                    comments,
                    commentCount: typeof i.commentCount === "number" ? i.commentCount : comments.length,
                    lastCommentAt,
                };
            });
            globalStore.set(this.issuesAtom, issues);
            globalStore.set(this.fetchedAtAtom, cache.fetchedAt || "");

            if (!globalStore.get(this.lastSeenAtAtom)) {
                let maxT = 0;
                for (const i of issues) {
                    const t = Date.parse(i.updated || "");
                    if (Number.isFinite(t) && t > maxT) maxT = t;
                }
                if (maxT > 0) {
                    globalStore.set(this.lastSeenAtAtom, new Date(maxT).toISOString());
                    this.persistPrefs();
                }
            }
        } catch (err) {
            globalStore.set(this.errorAtom, "Jira 캐시를 읽을 수 없습니다. Claude에게 'jira 이슈 새로고침'을 요청하세요.");
        } finally {
            globalStore.set(this.loadingAtom, false);
        }
    }

    toggleExpanded(key: string): void {
        const current = globalStore.get(this.expandedKeyAtom);
        globalStore.set(this.expandedKeyAtom, current === key ? null : key);
    }

    buildAnalysisPrompt(issue: JiraIssue, multiline: boolean): string {
        const header = [
            `Key: ${issue.key}`,
            `Summary: ${issue.summary}`,
            `Status: ${issue.status}`,
            `Priority: ${issue.priority}`,
            `Project: ${issue.projectKey}`,
            `URL: ${issue.webUrl}`,
        ];
        const attachLines = (issue.attachments || [])
            .filter((a) => a.localPath)
            .map((a) => `- ${a.filename} (${a.mimeType}) -> ${a.localPath}`);
        const sep = multiline ? "\n" : ", ";
        const body = header.join(sep);
        const attachBlock =
            attachLines.length > 0
                ? (multiline ? "\n\nAttachments:\n" : " | Attachments: ") + attachLines.join(multiline ? "\n" : "; ")
                : "";
        const comments = issue.comments || [];
        const commentBlock = comments.length > 0
            ? (multiline
                ? `\n\nComments (${comments.length}):\n` + comments.map((c) => {
                    const when = c.updated || c.created || "";
                    return `--- ${c.author} @ ${when}${c.truncated ? " [truncated]" : ""}\n${c.body}`;
                }).join("\n\n")
                : ` | Comments(${comments.length}): ${comments.map((c) => `[${c.author}] ${c.body.replace(/\s+/g, " ").slice(0, 120)}`).join(" // ")}`)
            : "";
        const extra = (globalStore.get(this.extraPromptAtom) || "").trim();
        const extraBlock = extra ? (multiline ? `\n\n추가 지시:\n${extra}` : ` | 추가 지시: ${extra}`) : "";
        const skill = (globalStore.get(this.analyzeSkillAtom) || "").trim();
        const instruction = skill
            ? `${skill} 로 이 Jira 이슈를 처리해줘.`
            : "이 이슈의 본문, 첨부파일, 댓글을 모두 참고해서 분석해줘. 댓글에 해결 방향 힌트가 있을 수 있으니 놓치지 말고, 필요하면 첨부파일을 Read 도구로 직접 읽어보고 관련 코드가 있으면 찾아서 설명해줘.";
        const intro = skill ? skill : "Jira 이슈를 분석해줘.";
        return `${intro}${multiline ? "\n\n" : " "}${body}${attachBlock}${commentBlock}${extraBlock}${multiline ? "\n\n" : " "}${instruction}`;
    }

    private getCli(): string {
        const cli = (globalStore.get(this.analyzeCliAtom) || "claude").trim();
        return cli || "claude";
    }

    async analyzeIssueInNewTerminal(issue: JiraIssue): Promise<void> {
        const prompt = this.buildAnalysisPrompt(issue, true);
        const cli = this.getCli();
        const blockDef: BlockDef = {
            meta: {
                view: "term",
                controller: "cmd",
                cmd: `${cli} "${prompt.replace(/"/g, '\\"')}"`,
                "cmd:runonstart": true,
                "cmd:interactive": true,
            },
        };
        await createBlock(blockDef);
    }

    resolveTargetTerminal(): string | null {
        const terminals = globalStore.get(this.terminalsAtom);
        const selected = globalStore.get(this.targetBlockIdAtom);
        if (selected && terminals.some((t) => t.blockId === selected)) {
            return selected;
        }
        if (selected) {
            globalStore.set(this.targetBlockIdAtom, null);
        }
        const layoutModel = getLayoutModelForStaticTab();
        const focusedNode = globalStore.get(layoutModel.focusedNode);
        const focusedBlockId = focusedNode?.data?.blockId;
        if (focusedBlockId && terminals.some((t) => t.blockId === focusedBlockId)) {
            return focusedBlockId;
        }
        if (terminals.length === 1) {
            return terminals[0].blockId;
        }
        return null;
    }

    async analyzeIssueInCurrentTerminal(issue: JiraIssue): Promise<void> {
        const target = this.resolveTargetTerminal();
        if (!target) {
            await this.analyzeIssueInNewTerminal(issue);
            return;
        }

        const prompt = this.buildAnalysisPrompt(issue, false);
        const cli = this.getCli();
        const cmd = `${cli} "${prompt.replace(/"/g, '\\"')}"\r`;

        await RpcApi.ControllerInputCommand(TabRpcClient, {
            blockid: target,
            inputdata64: stringToBase64(cmd),
        });
    }

    setTargetBlockId(blockId: string | null): void {
        globalStore.set(this.targetBlockIdAtom, blockId);
    }

    async analyzeIssue(issue: JiraIssue): Promise<void> {
        const mode = globalStore.get(this.launchModeAtom);
        if (mode === "current") {
            await this.analyzeIssueInCurrentTerminal(issue);
        } else {
            await this.analyzeIssueInNewTerminal(issue);
        }
    }

    async openAttachment(att: JiraAttachment): Promise<void> {
        if (att.localPath) {
            await createBlock({ meta: { view: "preview", file: att.localPath } });
            return;
        }
        if (att.webUrl) {
            await createBlock({ meta: { view: "web", url: att.webUrl } });
            return;
        }
    }

    async openIssueInBrowser(issue: JiraIssue): Promise<void> {
        const blockDef: BlockDef = {
            meta: { view: "web", url: issue.webUrl },
        };
        await createBlock(blockDef);
    }

    toggleLaunchMode(): void {
        const current = globalStore.get(this.launchModeAtom);
        globalStore.set(this.launchModeAtom, current === "new" ? "current" : "new");
    }
}

function AttachmentRow({
    attachment,
    onOpen,
}: {
    attachment: JiraAttachment;
    onOpen: (att: JiraAttachment) => void;
}) {
    const icon = attachmentIcon(attachment.mimeType, attachment.filename);
    const hasLocal = !!attachment.localPath;
    const hasRemote = !!attachment.webUrl;
    const clickable = hasLocal || hasRemote;
    return (
        <div
            className={clsx("attachment-row", !hasLocal && hasRemote && "attachment-remote")}
            onClick={(e) => {
                e.stopPropagation();
                if (clickable) onOpen(attachment);
            }}
            title={hasLocal ? attachment.localPath : hasRemote ? "Jira 웹에서 열기" : "다운로드 정보 없음"}
        >
            <i className={clsx("fa-solid", icon)} />
            <span className="attachment-name">{attachment.filename}</span>
            <span className="attachment-size">{formatFileSize(attachment.size)}</span>
            {!hasLocal && hasRemote && <i className="fa-solid fa-cloud-arrow-down attachment-remote-icon" title="웹에서 로드" />}
        </div>
    );
}

function IssueCard({
    issue,
    expanded,
    launchMode,
    terminals,
    targetBlockId,
    isNew,
    skill,
    cli,
    extraPrompt,
    onToggle,
    onAnalyze,
    onOpenAttachment,
    onOpenBrowser,
    onSelectTarget,
    onSkillChange,
    onCliChange,
    onExtraPromptChange,
}: {
    issue: JiraIssue;
    expanded: boolean;
    launchMode: LaunchMode;
    terminals: TerminalOption[];
    targetBlockId: string | null;
    isNew: boolean;
    skill: string;
    cli: string;
    extraPrompt: string;
    onToggle: (key: string) => void;
    onAnalyze: (issue: JiraIssue) => void;
    onOpenAttachment: (att: JiraAttachment) => void;
    onOpenBrowser: (issue: JiraIssue) => void;
    onSelectTarget: (blockId: string | null) => void;
    onSkillChange: (v: string) => void;
    onCliChange: (v: string) => void;
    onExtraPromptChange: (v: string) => void;
}) {
    const statusClass = STATUS_CATEGORY_CLASS[issue.statusCategory] || "status-todo";
    const priorityIcon = PRIORITY_ICONS[issue.priority] || "fa-minus";
    const attachments = issue.attachments || [];
    const comments = issue.comments || [];
    const commentCount = issue.commentCount || comments.length;

    return (
        <div className={clsx("jira-issue-card", statusClass, expanded && "expanded", isNew && "is-new")}>
            <div
                className="issue-card-main"
                onClick={() => onToggle(issue.key)}
                title={`${issue.key}: ${issue.summary}${isNew ? " (신규/업데이트됨)" : ""}`}
            >
                <div className="issue-card-header">
                    <span className="issue-key">
                        <i className={clsx("fa-solid caret", expanded ? "fa-caret-down" : "fa-caret-right")} />
                        {isNew && <span className="new-dot" title="신규/업데이트됨" />}
                        {issue.key}
                    </span>
                    <span className="issue-time">{formatUpdatedTime(issue.updated)}</span>
                </div>
                <div className={clsx("issue-summary", !expanded && "truncate")}>{issue.summary}</div>
                <div className="issue-meta">
                    <span className="issue-status">
                        <span className={clsx("status-dot", statusClass)} />
                        {issue.status}
                    </span>
                    <span className="issue-priority">
                        <i className={clsx("fa-solid", priorityIcon)} />
                        {issue.priority}
                    </span>
                    <span className="issue-type">{issue.issueType}</span>
                    {attachments.length > 0 && (
                        <span className="issue-attachments">
                            <i className="fa-solid fa-paperclip" />
                            {attachments.length}
                        </span>
                    )}
                    {commentCount > 0 && (
                        <span className="issue-comments" title={`댓글 ${commentCount}개`}>
                            <i className="fa-solid fa-comment" />
                            {commentCount}
                        </span>
                    )}
                </div>
            </div>

            {expanded && (
                <div className="issue-card-expanded" onClick={(e) => e.stopPropagation()}>
                    {issue.description ? (
                        <pre className="issue-description">{issue.description}</pre>
                    ) : (
                        <div className="issue-description-empty">본문이 비어있습니다.</div>
                    )}

                    {attachments.length > 0 && (
                        <div className="issue-attachments-block">
                            <div className="section-label">
                                <i className="fa-solid fa-paperclip" />
                                첨부파일 ({attachments.length})
                            </div>
                            <div className="attachment-list">
                                {attachments.map((att) => (
                                    <AttachmentRow key={att.id} attachment={att} onOpen={onOpenAttachment} />
                                ))}
                            </div>
                        </div>
                    )}

                    {commentCount > 0 && (
                        <div className="issue-comments-block">
                            <div className="section-label">
                                <i className="fa-solid fa-comments" />
                                댓글 ({commentCount}{comments.length < commentCount ? `, 최근 ${comments.length}개 저장` : ""})
                            </div>
                            <div className="comment-list">
                                {comments.map((c) => (
                                    <div key={c.id} className="comment-item">
                                        <div className="comment-head">
                                            <span className="comment-author">{c.author}</span>
                                            <span className="comment-time" title={c.updated || c.created}>
                                                {formatUpdatedTime(c.updated || c.created)}
                                            </span>
                                        </div>
                                        <pre className="comment-body">
                                            {c.body}
                                            {c.truncated && <span className="comment-truncated"> …(잘림)</span>}
                                        </pre>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {launchMode === "current" && terminals.length > 0 && (
                        <div className="target-terminal-row">
                            <label className="section-label" htmlFor={`target-${issue.key}`}>
                                <i className="fa-solid fa-terminal" />
                                대상 터미널
                            </label>
                            <select
                                id={`target-${issue.key}`}
                                className="target-terminal-select"
                                value={
                                    targetBlockId && terminals.some((t) => t.blockId === targetBlockId)
                                        ? targetBlockId
                                        : ""
                                }
                                onChange={(e) => onSelectTarget(e.target.value || null)}
                            >
                                <option value="">자동 (포커스 · 단일 터미널)</option>
                                {terminals.map((t) => (
                                    <option key={t.blockId} value={t.blockId}>
                                        {t.label}
                                    </option>
                                ))}
                            </select>
                            <span className="terminal-count">{terminals.length}개</span>
                        </div>
                    )}

                    <div className="analyze-config">
                        <div className="analyze-config-row">
                            <label className="filter-label" htmlFor={`cli-${issue.key}`}>CLI</label>
                            <input
                                id={`cli-${issue.key}`}
                                className="analyze-input short"
                                value={cli}
                                placeholder="claude"
                                onChange={(e) => onCliChange(e.target.value)}
                                list="jiratasks-cli-list"
                            />
                            <label className="filter-label skill-label" htmlFor={`skill-${issue.key}`}>Skill</label>
                            <input
                                id={`skill-${issue.key}`}
                                className="analyze-input"
                                value={skill}
                                placeholder="/gsd-explore (선택)"
                                onChange={(e) => onSkillChange(e.target.value)}
                                list={`skills-${issue.key}`}
                            />
                            <datalist id={`skills-${issue.key}`}>
                                {SKILL_SUGGESTIONS.map((s) => (
                                    <option key={s} value={s} />
                                ))}
                            </datalist>
                        </div>
                        <textarea
                            className="analyze-extra-prompt"
                            value={extraPrompt}
                            placeholder="추가 프롬프트 (선택) — 이슈 컨텍스트 뒤에 이어서 전달됩니다."
                            rows={3}
                            onChange={(e) => onExtraPromptChange(e.target.value)}
                        />
                    </div>

                    <div className="issue-actions">
                        <button
                            className="action-btn action-analyze"
                            onClick={() => onAnalyze(issue)}
                            title={launchMode === "new" ? "새 터미널에서 분석" : "현재 터미널에서 분석"}
                        >
                            <i className="fa-solid fa-wand-magic-sparkles" />
                            분석
                        </button>
                        <button
                            className="action-btn"
                            onClick={() => onOpenBrowser(issue)}
                            title="Jira에서 열기"
                        >
                            <i className="fa-solid fa-up-right-from-square" />
                            Jira 열기
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}

function JiraTasksView({ model }: { model: JiraTasksViewModel }) {
    const allIssues = useAtomValue(model.issuesAtom);
    const issues = useAtomValue(model.filteredIssuesAtom);
    const loading = useAtomValue(model.loadingAtom);
    const error = useAtomValue(model.errorAtom);
    const refreshProgress = useAtomValue(model.refreshProgressAtom);
    const launchMode = useAtomValue(model.launchModeAtom);
    const fetchedAt = useAtomValue(model.fetchedAtAtom);
    const expandedKey = useAtomValue(model.expandedKeyAtom);
    const terminals = useAtomValue(model.terminalsAtom);
    const targetBlockId = useAtomValue(model.targetBlockIdAtom);
    const projects = useAtomValue(model.availableProjectsAtom);
    const projectFilter = useAtomValue(model.projectFilterAtom);
    const statusFilter = useAtomValue(model.statusFilterAtom);
    const dateFilter = useAtomValue(model.dateFilterAtom);
    const refreshInterval = useAtomValue(model.refreshIntervalAtom);
    const filtersOpen = useAtomValue(model.filtersOpenAtom);
    const newCount = useAtomValue(model.newIssuesCountAtom);
    const lastSeenAt = useAtomValue(model.lastSeenAtAtom);
    const analyzeSkill = useAtomValue(model.analyzeSkillAtom);
    const analyzeCli = useAtomValue(model.analyzeCliAtom);
    const extraPrompt = useAtomValue(model.extraPromptAtom);

    useEffect(() => {
        fireAndForget(() => model.loadFromCache());
    }, []);

    useEffect(() => {
        if (refreshInterval <= 0) return;
        const id = setInterval(() => {
            fireAndForget(() => model.loadFromCache());
        }, refreshInterval * 1000);
        return () => clearInterval(id);
    }, [refreshInterval, model]);

    const lastSeenMs = Date.parse(lastSeenAt || "");
    const isIssueNew = useCallback((issue: JiraIssue): boolean => {
        if (!Number.isFinite(lastSeenMs)) return false;
        const t = Date.parse(issue.updated || "");
        return Number.isFinite(t) && t > lastSeenMs;
    }, [lastSeenMs]);

    const handleToggle = useCallback((key: string) => model.toggleExpanded(key), [model]);
    const handleAnalyze = useCallback((issue: JiraIssue) => {
        fireAndForget(() => model.analyzeIssue(issue));
    }, [model]);
    const handleOpenAttachment = useCallback((att: JiraAttachment) => {
        fireAndForget(() => model.openAttachment(att));
    }, [model]);
    const handleOpenBrowser = useCallback((issue: JiraIssue) => {
        fireAndForget(() => model.openIssueInBrowser(issue));
    }, [model]);
    const handleSelectTarget = useCallback((blockId: string | null) => {
        model.setTargetBlockId(blockId);
    }, [model]);
    const handleSkillChange = useCallback((v: string) => model.setAnalyzeSkill(v), [model]);
    const handleCliChange = useCallback((v: string) => model.setAnalyzeCli(v), [model]);
    const handleExtraPromptChange = useCallback((v: string) => model.setExtraPrompt(v), [model]);

    const statusSet = new Set(statusFilter);
    const hiddenCount = allIssues.length - issues.length;

    return (
        <div className="jiratasks-view">
            <div className="jiratasks-toolbar">
                <div className="toolbar-left">
                    <span className="issue-count">
                        {issues.length}
                        {hiddenCount > 0 && <span className="hidden-count"> / {allIssues.length}</span>}
                        개 이슈
                    </span>
                    {newCount > 0 && (
                        <span
                            className="new-badge"
                            title="신규 또는 업데이트된 이슈 수 (클릭하여 확인 처리)"
                            onClick={() => model.markAllRead()}
                        >
                            <i className="fa-solid fa-bell" />
                            {newCount} new
                        </span>
                    )}
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

            {filtersOpen && (
                <div className="jiratasks-filterbar">
                    <div className="filter-row">
                        <label className="filter-label">프로젝트</label>
                        <select
                            className="filter-select"
                            value={projectFilter || ""}
                            onChange={(e) => model.setProjectFilter(e.target.value || null)}
                        >
                            <option value="">전체 ({allIssues.length})</option>
                            {projects.map((p) => (
                                <option key={p.key} value={p.key}>
                                    {p.key} · {p.name} ({p.count})
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="filter-row">
                        <label className="filter-label">상태</label>
                        <div className="filter-chips">
                            {(["new", "indeterminate", "done"] as StatusCategory[]).map((cat) => (
                                <button
                                    key={cat}
                                    className={clsx("filter-chip", `chip-${cat}`, statusSet.has(cat) && "active")}
                                    onClick={() => model.toggleStatus(cat)}
                                >
                                    {STATUS_LABELS[cat]}
                                </button>
                            ))}
                        </div>
                    </div>

                    <div className="filter-row">
                        <label className="filter-label">기간</label>
                        <select
                            className="filter-select"
                            value={dateFilter}
                            onChange={(e) => model.setDateFilter(e.target.value as DateFilter)}
                        >
                            {DATE_OPTIONS.map((o) => (
                                <option key={o.value} value={o.value}>{o.label}</option>
                            ))}
                        </select>
                    </div>

                    <div className="filter-row">
                        <label className="filter-label">자동 새로고침</label>
                        <select
                            className="filter-select"
                            value={refreshInterval}
                            onChange={(e) => model.setRefreshInterval(Number(e.target.value))}
                            title="캐시 파일을 주기적으로 다시 읽습니다 (Jira→캐시 갱신은 ☁️ 버튼으로 수동 실행)"
                        >
                            {REFRESH_OPTIONS.map((o) => (
                                <option key={o.value} value={o.value}>{o.label}</option>
                            ))}
                        </select>
                    </div>
                </div>
            )}

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
                {refreshProgress && (
                    <div className="jira-refresh-summary">
                        <i className="fa-solid fa-circle-check" />
                        <span>{refreshProgress}</span>
                    </div>
                )}
                {!loading && !error && issues.length === 0 && (
                    <div className="jiratasks-empty">
                        <i className={clsx("fa-solid", allIssues.length === 0 ? "fa-circle-check" : "fa-filter")} />
                        <span>
                            {allIssues.length === 0
                                ? "할당된 이슈가 없습니다"
                                : `필터에 일치하는 이슈가 없습니다 (전체 ${allIssues.length}개)`}
                        </span>
                    </div>
                )}
                {issues.map((issue) => (
                    <IssueCard
                        key={issue.key}
                        issue={issue}
                        expanded={expandedKey === issue.key}
                        launchMode={launchMode}
                        terminals={terminals}
                        targetBlockId={targetBlockId}
                        isNew={isIssueNew(issue)}
                        skill={analyzeSkill}
                        cli={analyzeCli}
                        extraPrompt={extraPrompt}
                        onToggle={handleToggle}
                        onAnalyze={handleAnalyze}
                        onOpenAttachment={handleOpenAttachment}
                        onOpenBrowser={handleOpenBrowser}
                        onSelectTarget={handleSelectTarget}
                        onSkillChange={handleSkillChange}
                        onCliChange={handleCliChange}
                        onExtraPromptChange={handleExtraPromptChange}
                    />
                ))}
            </div>
        </div>
    );
}

export { ATTACHMENTS_DIR };

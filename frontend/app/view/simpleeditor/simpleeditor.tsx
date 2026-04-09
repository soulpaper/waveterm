// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import type { BlockNodeModel } from "@/app/block/blocktypes";
import { CodeEditor } from "@/app/view/codeeditor/codeeditor";
import { DiffViewer } from "@/app/view/codeeditor/diffviewer";
import type { TabModel } from "@/app/store/tab-model";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { tryReinjectKey } from "@/app/store/keymodel";
import { getApi, globalStore } from "@/store/global";
import { base64ToString, fireAndForget, stringToBase64 } from "@/util/util";
import { adaptFromReactOrNativeKeyEvent } from "@/util/keyutil";
import { checkKeyPressed } from "@/util/keyutil";
import clsx from "clsx";
import { Atom, atom, PrimitiveAtom } from "jotai";
import { useAtomValue } from "jotai";
import * as React from "react";
import type * as MonacoTypes from "monaco-editor";
import * as MonacoModule from "monaco-editor";
import "./simpleeditor.scss";

type VcsType = "git" | "svn" | null;
type DiffMode = "off" | "saved" | "vcs";

const LANGUAGE_LIST = [
    "plaintext",
    "javascript",
    "typescript",
    "python",
    "go",
    "rust",
    "java",
    "c",
    "cpp",
    "csharp",
    "html",
    "css",
    "scss",
    "json",
    "yaml",
    "xml",
    "markdown",
    "sql",
    "shell",
    "powershell",
    "dockerfile",
    "ruby",
    "php",
    "swift",
    "kotlin",
    "lua",
    "perl",
    "r",
    "toml",
    "ini",
    "graphql",
];

const EXT_TO_LANG: Record<string, string> = {
    ".js": "javascript",
    ".mjs": "javascript",
    ".cjs": "javascript",
    ".jsx": "javascript",
    ".ts": "typescript",
    ".tsx": "typescript",
    ".py": "python",
    ".go": "go",
    ".rs": "rust",
    ".java": "java",
    ".c": "c",
    ".h": "c",
    ".cpp": "cpp",
    ".cxx": "cpp",
    ".cc": "cpp",
    ".hpp": "cpp",
    ".cs": "csharp",
    ".html": "html",
    ".htm": "html",
    ".css": "css",
    ".scss": "scss",
    ".sass": "scss",
    ".less": "less",
    ".json": "json",
    ".yml": "yaml",
    ".yaml": "yaml",
    ".xml": "xml",
    ".md": "markdown",
    ".mdx": "markdown",
    ".sql": "sql",
    ".sh": "shell",
    ".bash": "shell",
    ".zsh": "shell",
    ".fish": "shell",
    ".ps1": "powershell",
    ".psm1": "powershell",
    ".rb": "ruby",
    ".php": "php",
    ".swift": "swift",
    ".kt": "kotlin",
    ".kts": "kotlin",
    ".lua": "lua",
    ".pl": "perl",
    ".pm": "perl",
    ".r": "r",
    ".R": "r",
    ".toml": "toml",
    ".ini": "ini",
    ".cfg": "ini",
    ".conf": "ini",
    ".graphql": "graphql",
    ".gql": "graphql",
    ".dockerfile": "dockerfile",
    ".svg": "xml",
    ".vue": "html",
    ".svelte": "html",
};

function detectLanguage(filePath: string): string {
    if (!filePath) return "plaintext";
    const name = filePath.split("/").pop() || filePath.split("\\").pop() || filePath;
    if (name === "Dockerfile" || name === "Containerfile") return "dockerfile";
    if (name === "Makefile" || name === "GNUmakefile") return "makefile";
    if (name === "Taskfile.yml" || name === "Taskfile.yaml") return "yaml";
    if (name.endsWith(".env") || name.startsWith(".env")) return "ini";
    const dotIdx = name.lastIndexOf(".");
    if (dotIdx === -1) return "plaintext";
    const ext = name.substring(dotIdx).toLowerCase();
    return EXT_TO_LANG[ext] ?? "plaintext";
}

function extractFileName(filePath: string): string {
    if (!filePath) return "";
    return filePath.split("/").pop() || filePath.split("\\").pop() || filePath;
}

/**
 * Resolve ~ in file path to the actual home directory.
 */
function resolveHome(filePath: string): string {
    if (!filePath) return filePath;
    if (filePath.startsWith("~/") || filePath === "~") {
        return filePath.replace("~", getApi().getHomeDir());
    }
    return filePath;
}

/**
 * Get the directory containing the file.
 */
function getFileDir(filePath: string): string {
    const resolved = resolveHome(filePath);
    const lastSlash = Math.max(resolved.lastIndexOf("/"), resolved.lastIndexOf("\\"));
    if (lastSlash === -1) return ".";
    return resolved.substring(0, lastSlash);
}

/**
 * Detect VCS type for a given file path by checking for .git or .svn directories.
 */
async function detectVcsType(filePath: string): Promise<VcsType> {
    if (!filePath) return null;
    const dir = getFileDir(filePath);
    try {
        // Try git first
        await getApi().execCommand("git", ["rev-parse", "--is-inside-work-tree"], dir);
        return "git";
    } catch {
        // Not a git repo
    }
    try {
        await getApi().execCommand("svn", ["info"], dir);
        return "svn";
    } catch {
        // Not an svn repo
    }
    return null;
}

/**
 * Get the VCS (git/svn) version of a file.
 */
async function getVcsFileContent(filePath: string, vcsType: VcsType): Promise<string> {
    if (!filePath || !vcsType) return "";
    const resolved = resolveHome(filePath);
    const dir = getFileDir(filePath);

    if (vcsType === "git") {
        // Get the relative path from git root
        const { stdout: gitRoot } = await getApi().execCommand(
            "git",
            ["rev-parse", "--show-toplevel"],
            dir
        );
        const root = gitRoot.trim().replace(/\\/g, "/");
        const resolvedNorm = resolved.replace(/\\/g, "/");
        let relPath: string;
        if (resolvedNorm.startsWith(root)) {
            relPath = resolvedNorm.substring(root.length + 1);
        } else {
            relPath = resolvedNorm;
        }
        const { stdout } = await getApi().execCommand("git", ["show", `HEAD:${relPath}`], dir);
        return stdout;
    }

    if (vcsType === "svn") {
        const { stdout } = await getApi().execCommand("svn", ["cat", resolved], dir);
        return stdout;
    }

    return "";
}

export class SimpleEditorModel implements ViewModel {
    viewType = "simpleeditor";
    blockId: string;
    nodeModel: BlockNodeModel;
    tabModel: TabModel;
    viewIcon: Atom<string> = atom("file-code");
    viewName: Atom<string> = atom("Editor");
    viewComponent = SimpleEditorView;
    noPadding: Atom<boolean> = atom(true);

    filePathAtom: PrimitiveAtom<string>;
    fileContentAtom: PrimitiveAtom<string>;
    savedContentAtom: PrimitiveAtom<string>;
    languageAtom: PrimitiveAtom<string>;
    isDirtyAtom: Atom<boolean>;
    isLoadingAtom: PrimitiveAtom<boolean>;
    errorAtom: PrimitiveAtom<string | null>;
    monacoRef: React.RefObject<MonacoTypes.editor.IStandaloneCodeEditor>;

    // Diff state
    diffModeAtom: PrimitiveAtom<DiffMode>;
    diffOriginalAtom: PrimitiveAtom<string>;
    vcsTypeAtom: PrimitiveAtom<VcsType>;
    diffLoadingAtom: PrimitiveAtom<boolean>;

    viewText: Atom<HeaderElem[]>;
    endIconButtons: Atom<IconButtonDecl[]>;

    constructor({ blockId, nodeModel, tabModel }: ViewModelInitType) {
        this.blockId = blockId;
        this.nodeModel = nodeModel;
        this.tabModel = tabModel;
        this.monacoRef = React.createRef();

        this.filePathAtom = atom("");
        this.fileContentAtom = atom("");
        this.savedContentAtom = atom("");
        this.languageAtom = atom("plaintext");
        this.isLoadingAtom = atom(false);
        this.errorAtom = atom<string | null>(null) as PrimitiveAtom<string | null>;

        // Diff atoms
        this.diffModeAtom = atom<DiffMode>("off") as PrimitiveAtom<DiffMode>;
        this.diffOriginalAtom = atom("");
        this.vcsTypeAtom = atom<VcsType>(null) as PrimitiveAtom<VcsType>;
        this.diffLoadingAtom = atom(false);

        this.isDirtyAtom = atom((get) => {
            const current = get(this.fileContentAtom);
            const saved = get(this.savedContentAtom);
            return current !== saved;
        });

        this.viewText = atom((get) => {
            const filePath = get(this.filePathAtom);
            const isDirty = get(this.isDirtyAtom);
            const lang = get(this.languageAtom);
            const diffMode = get(this.diffModeAtom);
            const vcsType = get(this.vcsTypeAtom);
            const fileName = filePath ? extractFileName(filePath) : "Untitled";
            const displayName = isDirty ? `${fileName} *` : fileName;

            const elems: HeaderElem[] = [
                {
                    elemtype: "text",
                    text: displayName,
                    className: clsx("simpleeditor-filename", isDirty && "is-dirty"),
                },
                {
                    elemtype: "text",
                    text: lang,
                    className: "simpleeditor-lang-badge",
                },
            ];
            if (diffMode !== "off") {
                const diffLabel = diffMode === "vcs" ? `Diff: ${vcsType?.toUpperCase()}` : "Diff: Saved";
                elems.push({
                    elemtype: "text",
                    text: diffLabel,
                    className: "simpleeditor-diff-badge",
                });
            }
            return elems;
        });

        this.endIconButtons = atom((get) => {
            const isDirty = get(this.isDirtyAtom);
            const diffMode = get(this.diffModeAtom);
            const vcsType = get(this.vcsTypeAtom);
            const filePath = get(this.filePathAtom);
            const buttons: IconButtonDecl[] = [];

            // Diff buttons
            if (filePath && diffMode === "off") {
                if (vcsType) {
                    buttons.push({
                        elemtype: "iconbutton",
                        icon: "code-compare",
                        title: `Diff with ${vcsType.toUpperCase()} (Ctrl+D)`,
                        click: () => fireAndForget(() => this.toggleDiff("vcs")),
                    });
                }
                if (isDirty) {
                    buttons.push({
                        elemtype: "iconbutton",
                        icon: "arrows-left-right",
                        title: "Diff with saved",
                        click: () => fireAndForget(() => this.toggleDiff("saved")),
                    });
                }
            }
            if (diffMode !== "off") {
                buttons.push({
                    elemtype: "iconbutton",
                    icon: "xmark",
                    title: "Close Diff",
                    click: () => this.closeDiff(),
                });
            }

            // File operations
            buttons.push({
                elemtype: "iconbutton",
                icon: "folder-open",
                title: "Open File",
                click: () => fireAndForget(() => this.openFileDialog()),
            });
            buttons.push({
                elemtype: "iconbutton",
                icon: "file-circle-plus",
                title: "New File",
                click: () => this.newFile(),
            });
            if (isDirty) {
                buttons.unshift({
                    elemtype: "iconbutton",
                    icon: "floppy-disk",
                    title: "Save (Ctrl+S)",
                    click: () => fireAndForget(() => this.saveFile()),
                });
            }
            return buttons;
        });
    }

    newFile(): void {
        globalStore.set(this.filePathAtom, "");
        globalStore.set(this.fileContentAtom, "");
        globalStore.set(this.savedContentAtom, "");
        globalStore.set(this.languageAtom, "plaintext");
        globalStore.set(this.errorAtom, null);
        globalStore.set(this.diffModeAtom, "off");
        globalStore.set(this.vcsTypeAtom, null);
        if (this.monacoRef.current) {
            this.monacoRef.current.setValue("");
            this.monacoRef.current.focus();
        }
    }

    async openFile(filePath: string): Promise<void> {
        if (!filePath) return;
        globalStore.set(this.isLoadingAtom, true);
        globalStore.set(this.errorAtom, null);
        globalStore.set(this.diffModeAtom, "off");
        try {
            const fileData = await RpcApi.FileReadCommand(TabRpcClient, {
                info: { path: filePath },
            });
            const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
            const lang = detectLanguage(filePath);
            globalStore.set(this.filePathAtom, filePath);
            globalStore.set(this.fileContentAtom, content);
            globalStore.set(this.savedContentAtom, content);
            globalStore.set(this.languageAtom, lang);
            if (this.monacoRef.current) {
                this.monacoRef.current.setValue(content);
                this.monacoRef.current.focus();
            }
            // Detect VCS type in background
            detectVcsType(filePath).then((vcsType) => {
                globalStore.set(this.vcsTypeAtom, vcsType);
            });
        } catch (e) {
            globalStore.set(this.errorAtom, `Failed to open: ${e}`);
        } finally {
            globalStore.set(this.isLoadingAtom, false);
        }
    }

    async saveFile(): Promise<void> {
        const filePath = globalStore.get(this.filePathAtom);
        if (!filePath) {
            await this.saveAsDialog();
            return;
        }
        const content = globalStore.get(this.fileContentAtom);
        globalStore.set(this.errorAtom, null);
        try {
            await RpcApi.FileWriteCommand(TabRpcClient, {
                info: { path: filePath },
                data64: stringToBase64(content),
            });
            globalStore.set(this.savedContentAtom, content);
        } catch (e) {
            globalStore.set(this.errorAtom, `Save failed: ${e}`);
        }
    }

    async openFileDialog(): Promise<void> {
        // Placeholder - user can drag & drop files or type a path in the toolbar
    }

    async saveAsDialog(): Promise<void> {
        const content = globalStore.get(this.fileContentAtom);
        const lang = globalStore.get(this.languageAtom);
        const ext = Object.entries(EXT_TO_LANG).find(([, l]) => l === lang)?.[0] ?? ".txt";
        const defaultPath = `~/untitled${ext}`;
        globalStore.set(this.filePathAtom, defaultPath);
        try {
            await RpcApi.FileWriteCommand(TabRpcClient, {
                info: { path: defaultPath },
                data64: stringToBase64(content),
            });
            globalStore.set(this.savedContentAtom, content);
        } catch (e) {
            globalStore.set(this.errorAtom, `Save failed: ${e}`);
        }
    }

    setLanguage(lang: string): void {
        globalStore.set(this.languageAtom, lang);
    }

    // --- Diff methods ---

    async toggleDiff(mode: DiffMode): Promise<void> {
        const currentMode = globalStore.get(this.diffModeAtom);
        if (currentMode === mode) {
            this.closeDiff();
            return;
        }

        globalStore.set(this.diffLoadingAtom, true);
        globalStore.set(this.errorAtom, null);

        try {
            let original = "";
            if (mode === "saved") {
                original = globalStore.get(this.savedContentAtom);
            } else if (mode === "vcs") {
                const filePath = globalStore.get(this.filePathAtom);
                const vcsType = globalStore.get(this.vcsTypeAtom);
                if (!filePath || !vcsType) {
                    globalStore.set(this.errorAtom, "No VCS detected for this file");
                    return;
                }
                original = await getVcsFileContent(filePath, vcsType);
            }
            globalStore.set(this.diffOriginalAtom, original);
            globalStore.set(this.diffModeAtom, mode);
        } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            globalStore.set(this.errorAtom, `Diff failed: ${msg}`);
        } finally {
            globalStore.set(this.diffLoadingAtom, false);
        }
    }

    closeDiff(): void {
        globalStore.set(this.diffModeAtom, "off");
        globalStore.set(this.diffOriginalAtom, "");
    }

    giveFocus(): boolean {
        if (this.monacoRef.current) {
            this.monacoRef.current.focus();
            return true;
        }
        return false;
    }

    keyDownHandler(e: WaveKeyboardEvent): boolean {
        if (checkKeyPressed(e, "Ctrl:s") || checkKeyPressed(e, "Cmd:s")) {
            fireAndForget(() => this.saveFile());
            return true;
        }
        if (checkKeyPressed(e, "Ctrl:n") || checkKeyPressed(e, "Cmd:n")) {
            this.newFile();
            return true;
        }
        if (checkKeyPressed(e, "Ctrl:d") || checkKeyPressed(e, "Cmd:d")) {
            const diffMode = globalStore.get(this.diffModeAtom);
            if (diffMode !== "off") {
                this.closeDiff();
            } else {
                const vcsType = globalStore.get(this.vcsTypeAtom);
                if (vcsType) {
                    fireAndForget(() => this.toggleDiff("vcs"));
                } else {
                    fireAndForget(() => this.toggleDiff("saved"));
                }
            }
            return true;
        }
        if (checkKeyPressed(e, "Escape")) {
            const diffMode = globalStore.get(this.diffModeAtom);
            if (diffMode !== "off") {
                this.closeDiff();
                return true;
            }
        }
        return false;
    }
}

// --- View Components ---

function SimpleEditorToolbar({
    model,
    diffMode,
    onToggleDiff,
}: {
    model: SimpleEditorModel;
    diffMode: DiffMode;
    onToggleDiff: (mode: DiffMode) => void;
}) {
    const filePath = useAtomValue(model.filePathAtom);
    const language = useAtomValue(model.languageAtom);
    const isDirty = useAtomValue(model.isDirtyAtom);
    const error = useAtomValue(model.errorAtom);
    const vcsType = useAtomValue(model.vcsTypeAtom);
    const diffLoading = useAtomValue(model.diffLoadingAtom);
    const [showPathInput, setShowPathInput] = React.useState(false);
    const [inputPath, setInputPath] = React.useState("");
    const inputRef = React.useRef<HTMLInputElement>(null);

    React.useEffect(() => {
        if (showPathInput && inputRef.current) {
            inputRef.current.focus();
        }
    }, [showPathInput]);

    const handleOpenPath = () => {
        setShowPathInput(true);
        setInputPath(filePath || "");
    };

    const handlePathSubmit = () => {
        if (inputPath.trim()) {
            fireAndForget(() => model.openFile(inputPath.trim()));
        }
        setShowPathInput(false);
    };

    const handlePathKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === "Enter") {
            e.preventDefault();
            handlePathSubmit();
        }
        if (e.key === "Escape") {
            setShowPathInput(false);
        }
    };

    return (
        <div className="simpleeditor-toolbar">
            <div className="toolbar-left">
                {showPathInput ? (
                    <input
                        ref={inputRef}
                        type="text"
                        className="path-input"
                        value={inputPath}
                        onChange={(e) => setInputPath(e.target.value)}
                        onKeyDown={handlePathKeyDown}
                        onBlur={handlePathSubmit}
                        placeholder="Enter file path..."
                    />
                ) : (
                    <button className="path-display" onClick={handleOpenPath} title="Click to open a file path">
                        <i className="fa-solid fa-folder-open" />
                        <span>{filePath || "Untitled"}</span>
                        {isDirty && <span className="dirty-dot" />}
                    </button>
                )}
            </div>
            <div className="toolbar-right">
                {/* Diff buttons */}
                {filePath && (
                    <div className="toolbar-diff-group">
                        {vcsType && (
                            <button
                                className={clsx("toolbar-btn diff-btn", diffMode === "vcs" && "active")}
                                onClick={() => onToggleDiff("vcs")}
                                disabled={diffLoading}
                                title={`Diff with ${vcsType.toUpperCase()} HEAD (Ctrl+D)`}
                            >
                                <i className="fa-solid fa-code-compare" />
                                <span className="diff-btn-label">{vcsType.toUpperCase()}</span>
                            </button>
                        )}
                        {isDirty && (
                            <button
                                className={clsx("toolbar-btn diff-btn", diffMode === "saved" && "active")}
                                onClick={() => onToggleDiff("saved")}
                                disabled={diffLoading}
                                title="Diff with saved version"
                            >
                                <i className="fa-solid fa-arrows-left-right" />
                                <span className="diff-btn-label">Saved</span>
                            </button>
                        )}
                        {diffMode !== "off" && (
                            <button
                                className="toolbar-btn close-diff-btn"
                                onClick={() => model.closeDiff()}
                                title="Close diff (Esc)"
                            >
                                <i className="fa-solid fa-xmark" />
                            </button>
                        )}
                    </div>
                )}

                <div className="toolbar-separator" />

                <select
                    className="lang-select"
                    value={language}
                    onChange={(e) => model.setLanguage(e.target.value)}
                >
                    {LANGUAGE_LIST.map((lang) => (
                        <option key={lang} value={lang}>
                            {lang}
                        </option>
                    ))}
                </select>
                <button
                    className={clsx("toolbar-btn save-btn", isDirty && "active")}
                    onClick={() => fireAndForget(() => model.saveFile())}
                    disabled={!isDirty}
                    title="Save (Ctrl+S)"
                >
                    <i className="fa-solid fa-floppy-disk" />
                </button>
                <button
                    className="toolbar-btn"
                    onClick={() => model.newFile()}
                    title="New File (Ctrl+N)"
                >
                    <i className="fa-solid fa-file-circle-plus" />
                </button>
            </div>
            {error && (
                <div className="toolbar-error">
                    <i className="fa-solid fa-triangle-exclamation" />
                    <span>{error}</span>
                    <button onClick={() => globalStore.set(model.errorAtom, null)}>
                        <i className="fa-solid fa-xmark" />
                    </button>
                </div>
            )}
        </div>
    );
}

function SimpleEditorView({ model }: ViewComponentProps<SimpleEditorModel>) {
    const fileContent = useAtomValue(model.fileContentAtom);
    const language = useAtomValue(model.languageAtom);
    const filePath = useAtomValue(model.filePathAtom);
    const isLoading = useAtomValue(model.isLoadingAtom);
    const diffMode = useAtomValue(model.diffModeAtom);
    const diffOriginal = useAtomValue(model.diffOriginalAtom);
    const diffLoading = useAtomValue(model.diffLoadingAtom);
    const [isDragOver, setIsDragOver] = React.useState(false);

    const handleChange = React.useCallback(
        (text: string) => {
            globalStore.set(model.fileContentAtom, text);
        },
        [model]
    );

    const handleMount = React.useCallback(
        (editor: MonacoTypes.editor.IStandaloneCodeEditor, monacoApi: typeof MonacoModule): (() => void) => {
            model.monacoRef.current = editor;

            const keyDownDisposer = editor.onKeyDown((e: MonacoTypes.IKeyboardEvent) => {
                const waveEvent = adaptFromReactOrNativeKeyEvent(e.browserEvent);
                const handled = tryReinjectKey(waveEvent);
                if (handled) {
                    e.stopPropagation();
                    e.preventDefault();
                }
            });

            const isFocused = globalStore.get(model.nodeModel.isFocused);
            if (isFocused) {
                editor.focus();
            }

            return () => {
                keyDownDisposer.dispose();
                model.monacoRef.current = null;
            };
        },
        [model]
    );

    const handleToggleDiff = React.useCallback(
        (mode: DiffMode) => {
            fireAndForget(() => model.toggleDiff(mode));
        },
        [model]
    );

    // Drag & drop handlers
    const handleDragOver = React.useCallback((e: React.DragEvent) => {
        if (e.dataTransfer.types.includes("Files")) {
            e.preventDefault();
            e.stopPropagation();
            setIsDragOver(true);
        }
    }, []);

    const handleDragLeave = React.useCallback((e: React.DragEvent) => {
        e.preventDefault();
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const x = e.clientX;
        const y = e.clientY;
        if (x <= rect.left || x >= rect.right || y <= rect.top || y >= rect.bottom) {
            setIsDragOver(false);
        }
    }, []);

    const handleDrop = React.useCallback(
        (e: React.DragEvent) => {
            e.preventDefault();
            e.stopPropagation();
            setIsDragOver(false);

            if (e.dataTransfer.files.length > 0) {
                const file = e.dataTransfer.files[0];
                const path = (file as any).path;
                if (path) {
                    fireAndForget(() => model.openFile(path));
                }
            }
        },
        [model]
    );

    const renderContent = () => {
        if (isLoading || diffLoading) {
            return (
                <div className="simpleeditor-loading">
                    <i className="fa-solid fa-spinner fa-spin" />
                    <span>{diffLoading ? "Loading diff..." : "Loading..."}</span>
                </div>
            );
        }

        if (diffMode !== "off") {
            return (
                <div className="simpleeditor-editor-wrap">
                    <DiffViewer
                        blockId={model.blockId}
                        original={diffOriginal}
                        modified={fileContent}
                        language={language}
                        fileName={filePath || "untitled"}
                    />
                </div>
            );
        }

        return (
            <div className="simpleeditor-editor-wrap">
                <CodeEditor
                    blockId={model.blockId}
                    text={fileContent}
                    language={language}
                    fileName={filePath || undefined}
                    readonly={false}
                    onChange={handleChange}
                    onMount={handleMount}
                />
            </div>
        );
    };

    return (
        <div
            className={clsx("simpleeditor-view", isDragOver && "drag-over")}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
        >
            <SimpleEditorToolbar model={model} diffMode={diffMode} onToggleDiff={handleToggleDiff} />
            {renderContent()}
            {isDragOver && (
                <div className="simpleeditor-drop-overlay">
                    <i className="fa-solid fa-file-import" />
                    <span>Drop file to open</span>
                </div>
            )}
        </div>
    );
}

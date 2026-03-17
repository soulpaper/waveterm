// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

import { Modal } from "@/app/modals/modal";
import { getApi } from "@/app/store/global";
import { modalsModel } from "@/app/store/modalmodel";
import { RpcApi } from "@/app/store/wshclientapi";
import { TabRpcClient } from "@/app/store/wshrpcutil";
import { base64ToString, makeIconClass, stringToBase64 } from "@/util/util";
import { memo, useCallback, useState } from "react";

const COMMON_ICONS: { name: string; label: string }[] = [
    { name: "globe", label: "Globe" },
    { name: "bookmark", label: "Bookmark" },
    { name: "link", label: "Link" },
    { name: "newspaper", label: "News" },
    { name: "code", label: "Code" },
    { name: "envelope", label: "Email" },
    { name: "comment", label: "Chat" },
    { name: "video", label: "Video" },
    { name: "music", label: "Music" },
    { name: "image", label: "Image" },
    { name: "chart-line", label: "Chart" },
    { name: "cloud", label: "Cloud" },
    { name: "book", label: "Book" },
    { name: "graduation-cap", label: "Education" },
    { name: "calendar", label: "Calendar" },
    { name: "star", label: "Star" },
    { name: "heart", label: "Heart" },
    { name: "house", label: "Home" },
    { name: "magnifying-glass", label: "Search" },
    { name: "message-bot", label: "Bot" },
    { name: "brands@github", label: "GitHub" },
    { name: "brands@google", label: "Google" },
    { name: "brands@youtube", label: "YouTube" },
    { name: "brands@discord", label: "Discord" },
    { name: "brands@slack", label: "Slack" },
    { name: "brands@linkedin", label: "LinkedIn" },
    { name: "brands@reddit", label: "Reddit" },
    { name: "brands@twitter", label: "Twitter" },
    { name: "brands@stack-overflow", label: "StackOverflow" },
    { name: "brands@spotify", label: "Spotify" },
];

function sanitizeKey(input: string): string {
    return input
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-|-$/g, "")
        .slice(0, 30);
}

function extractDomain(url: string): string {
    try {
        const u = new URL(url.startsWith("http") ? url : "https://" + url);
        return u.hostname.replace(/^www\./, "");
    } catch {
        return "widget";
    }
}

const AddWebWidgetModal = memo(() => {
    const [url, setUrl] = useState("");
    const [label, setLabel] = useState("");
    const [icon, setIcon] = useState("globe");
    const [showIconPicker, setShowIconPicker] = useState(false);
    const [error, setError] = useState("");
    const [isSaving, setIsSaving] = useState(false);

    const handleClose = useCallback(() => {
        modalsModel.popModal();
    }, []);

    const handleSave = useCallback(async () => {
        if (!url.trim()) {
            setError("URL is required");
            return;
        }

        setIsSaving(true);
        setError("");

        try {
            let normalizedUrl = url.trim();
            if (!normalizedUrl.startsWith("http://") && !normalizedUrl.startsWith("https://")) {
                normalizedUrl = "https://" + normalizedUrl;
            }

            const displayLabel = label.trim() || extractDomain(normalizedUrl);
            const keyBase = sanitizeKey(displayLabel);
            let widgetKey = `custom@${keyBase || "web"}`;

            const configDir = getApi().getConfigDir();
            const fullPath = `${configDir}/widgets.json`;

            let widgetsConfig: Record<string, any> = {};
            try {
                const fileData = await RpcApi.FileReadCommand(TabRpcClient, { info: { path: fullPath } }, null);
                const content = fileData?.data64 ? base64ToString(fileData.data64) : "";
                if (content.trim()) {
                    widgetsConfig = JSON.parse(content);
                }
            } catch {
                // File doesn't exist yet, start with empty config
            }

            // Ensure unique key
            if (widgetsConfig[widgetKey]) {
                let suffix = 2;
                while (widgetsConfig[`${widgetKey}-${suffix}`]) {
                    suffix++;
                }
                widgetKey = `${widgetKey}-${suffix}`;
            }

            widgetsConfig[widgetKey] = {
                icon: icon || "globe",
                label: displayLabel,
                blockdef: {
                    meta: {
                        view: "web",
                        url: normalizedUrl,
                    },
                },
            };

            const formatted = JSON.stringify(widgetsConfig, null, 4);
            await RpcApi.FileWriteCommand(
                TabRpcClient,
                {
                    info: { path: fullPath },
                    data64: stringToBase64(formatted),
                },
                null
            );

            modalsModel.popModal();
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setIsSaving(false);
        }
    }, [url, label, icon]);

    const handleKeyDown = useCallback(
        (e: React.KeyboardEvent) => {
            if (e.key === "Enter" && !isSaving && url.trim()) {
                e.preventDefault();
                handleSave();
            }
        },
        [handleSave, isSaving, url]
    );

    return (
        <Modal
            className="addwebwidget-modal"
            onOk={handleSave}
            onCancel={handleClose}
            onClose={handleClose}
            okLabel={isSaving ? "Adding..." : "Add Widget"}
            cancelLabel="Cancel"
            okDisabled={!url.trim() || isSaving}
        >
            <div className="flex flex-col gap-4" style={{ minWidth: 380 }}>
                <h2 className="text-lg font-semibold">Add Web Widget</h2>

                <div className="flex flex-col gap-1">
                    <label className="text-xs font-medium text-secondary">URL *</label>
                    <input
                        type="text"
                        value={url}
                        onChange={(e) => setUrl(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="https://example.com"
                        autoFocus
                        className="px-3 py-2 text-sm rounded border border-border bg-transparent text-white focus:outline-none focus:border-accent"
                    />
                </div>

                <div className="flex flex-col gap-1">
                    <label className="text-xs font-medium text-secondary">Label</label>
                    <input
                        type="text"
                        value={label}
                        onChange={(e) => setLabel(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="My Widget"
                        className="px-3 py-2 text-sm rounded border border-border bg-transparent text-white focus:outline-none focus:border-accent"
                    />
                </div>

                <div className="flex flex-col gap-1">
                    <label className="text-xs font-medium text-secondary">Icon</label>
                    <div className="flex items-center gap-2">
                        <div className="w-9 h-9 flex items-center justify-center border border-border rounded text-lg">
                            <i className={makeIconClass(icon || "globe", false)} />
                        </div>
                        <input
                            type="text"
                            value={icon}
                            onChange={(e) => setIcon(e.target.value)}
                            onKeyDown={handleKeyDown}
                            placeholder="globe"
                            className="flex-1 px-3 py-2 text-sm rounded border border-border bg-transparent text-white focus:outline-none focus:border-accent"
                        />
                        <button
                            type="button"
                            onClick={() => setShowIconPicker(!showIconPicker)}
                            className="px-2.5 py-2 border border-border rounded hover:bg-hoverbg cursor-pointer text-secondary hover:text-white"
                        >
                            <i className="fa fa-solid fa-grid-2" />
                        </button>
                    </div>
                    {showIconPicker && (
                        <div className="grid grid-cols-6 gap-1 p-2 border border-border rounded mt-1 max-h-[200px] overflow-y-auto">
                            {COMMON_ICONS.map(({ name, label: iconLabel }) => (
                                <div
                                    key={name}
                                    onClick={() => {
                                        setIcon(name);
                                        setShowIconPicker(false);
                                    }}
                                    className={`flex flex-col items-center justify-center p-1.5 rounded cursor-pointer text-secondary hover:bg-hoverbg hover:text-white ${icon === name ? "bg-accent/20 text-accent" : ""}`}
                                    title={iconLabel}
                                >
                                    <i className={makeIconClass(name, false)} />
                                    <span className="text-[9px] mt-0.5 truncate w-full text-center">
                                        {iconLabel}
                                    </span>
                                </div>
                            ))}
                        </div>
                    )}
                </div>

                {error && <div className="text-sm text-error">{error}</div>}
            </div>
        </Modal>
    );
});

AddWebWidgetModal.displayName = "AddWebWidgetModal";

export { AddWebWidgetModal };

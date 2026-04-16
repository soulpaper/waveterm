// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

// This file defines the on-disk cache schema — the shape that
// ~/.config/waveterm/jira-cache.json holds and that the widget at
// frontend/app/view/jiratasks/jiratasks.tsx (lines 116-160) consumes.
//
// These types are deliberately kept separate from the wire-format types in
// client.go (Issue, IssueFields, Attachment, Comment) because the shapes
// diverge on several fields after ADF conversion and author flattening:
//
//   - Comment.Author is an object on the wire, a string in the cache.
//   - Description / Comment.Body are ADF JSON on the wire, markdown strings
//     in the cache.
//   - StatusCategory is a nested struct on the wire, a flat string in the
//     cache (mapped via statusCategoryFromKey; unknown → "new" per D-CACHE-08).
//   - commentCount / lastCommentAt / webUrl are synthetic — computed by
//     refresh.go's buildCacheIssue, not present on the wire.
//
// Every field and json tag here corresponds 1:1 to the TypeScript interface
// the widget declares. Changing a tag here is a widget-breaking change.

// JiraCache is the top-level JSON object at ~/.config/waveterm/jira-cache.json
// (D-CACHE-01, D-CACHE-02).
type JiraCache struct {
	CloudId   string           `json:"cloudId"`
	BaseUrl   string           `json:"baseUrl"`
	AccountId string           `json:"accountId"`
	FetchedAt string           `json:"fetchedAt"` // ISO8601 UTC, e.g. "2026-04-15T08:30:00Z"
	Issues    []JiraCacheIssue `json:"issues"`
}

// JiraCacheIssue is the cache representation of one Jira issue (D-CACHE-03).
// Every field maps to the widget's JiraIssue interface.
type JiraCacheIssue struct {
	Key            string                `json:"key"`
	ID             string                `json:"id"`
	Summary        string                `json:"summary"`
	Description    string                `json:"description"`    // ADF → markdown; "" when null (D-CACHE-04)
	Status         string                `json:"status"`         // status.name
	StatusCategory string                `json:"statusCategory"` // "new" | "indeterminate" | "done" (D-CACHE-08)
	IssueType      string                `json:"issueType"`      // issuetype.name
	Priority       string                `json:"priority"`       // priority.name, "" if none
	ProjectKey     string                `json:"projectKey"`
	ProjectName    string                `json:"projectName"`
	Updated        string                `json:"updated"`
	Created        string                `json:"created"`
	WebUrl         string                `json:"webUrl"`      // baseUrl + "/browse/" + key
	Attachments    []JiraCacheAttachment `json:"attachments"` // ALWAYS non-nil (RESEARCH Pitfall 3)
	Comments       []JiraCacheComment    `json:"comments"`    // ALWAYS non-nil
	CommentCount   int                   `json:"commentCount"`  // wire total, may exceed len(Comments) (D-CACHE-07)
	LastCommentAt  string                `json:"lastCommentAt"` // max(updated,created) across kept; "" if none
}

// JiraCacheAttachment is the cache representation of an issue attachment
// (D-CACHE-05). Metadata only — binary content is never embedded.
type JiraCacheAttachment struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
	LocalPath string `json:"localPath"` // "" default; preserved from prior cache (D-FLOW-04)
	WebUrl    string `json:"webUrl"`    // site-pattern URL (D-CACHE-05, RESEARCH A3)
}

// JiraCacheComment is the cache representation of one comment (D-CACHE-06).
// Body is markdown (ADF-flattened) and capped at 2000 chars; Truncated flags
// the cap was hit.
//
// Author is a STRING (displayName, or accountId fallback) — Jira's wire
// shape is an object; the widget reads `c.author` as a string. See
// RESEARCH Pitfall 1.
type JiraCacheComment struct {
	ID        string `json:"id"`
	Author    string `json:"author"`
	Created   string `json:"created"`
	Updated   string `json:"updated"`
	Body      string `json:"body"`
	Truncated bool   `json:"truncated,omitempty"` // omit when false (RESEARCH A2 / D-TEST-01 golden-file)
}

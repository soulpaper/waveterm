// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package jira — refresh orchestration (Phase 2).
//
// Refresh is the single entry point that Phase 3's wsh command and widget
// RPC will call. It combines Phase 1 primitives (Config, Client.SearchIssues,
// Client.GetIssue, Client.GetMyself, ADFToMarkdown) into one sequential
// operation that writes ~/.config/waveterm/jira-cache.json in the exact
// schema the widget at frontend/app/view/jiratasks/jiratasks.tsx consumes.
//
// Design per:
//   - CONTEXT.md:  D-CACHE-01..08, D-FLOW-01..05, D-PROG-01..02,
//                  D-ERR-01..03, D-CONC-01, D-TEST-01..03
//   - RESEARCH.md: §Architecture Patterns (Patterns 1-4), §Common Pitfalls 1-8

package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wavetermdev/waveterm/pkg/util/fileutil"
)

// RefreshOpts is the input to Refresh. Config is required; the other fields
// are optional.
type RefreshOpts struct {
	// Config is the loaded Jira configuration. Typically obtained via
	// LoadConfig(); callers with a pre-built Config (tests, future wsh
	// subcommands) may pass it directly.
	Config Config

	// HTTPClient, if non-nil, overrides the default 30s-timeout http.Client
	// that NewClient would use. Tests pass httptest.NewServer().Client() here
	// so the server's self-signed cert is accepted. Phase 5 will pass a
	// rate-limiting Transport via this hook. Nil → NewClient(cfg) is used.
	HTTPClient *http.Client

	// OnProgress, if non-nil, is invoked at stage transitions (D-PROG-01).
	// Stages:
	//   "search" — paginating search. current = issues discovered so far;
	//              total = 0 (enhanced search has no total per RESEARCH
	//              Pitfall 6). First call is ("search", 0, 0).
	//   "fetch"  — per-issue GetIssue. current = 1-based index of the issue
	//              whose fetch just completed (success or skip);
	//              total = len(allKeys).
	//   "write"  — cache write. First call ("write", 0, 1) before marshal;
	//              final call ("write", 1, 1) after successful rename.
	//
	// The callback runs on Refresh's goroutine; it must not block.
	OnProgress func(stage string, current, total int)
}

// RefreshReport summarizes a successful Refresh. Nil when Refresh returns a
// fatal error before the first write (D-PROG-02).
type RefreshReport struct {
	IssueCount      int           // number of issues successfully written (omits per-issue failures)
	AttachmentCount int           // total attachments across kept issues
	CommentCount    int           // total kept comments across kept issues (NOT sum of issue.CommentCount)
	Elapsed         time.Duration // wall-clock time spent in Refresh
	CachePath       string        // absolute path of the written cache file
}

// Refresh fetches the configured JQL, converts each issue to the cache
// schema, and atomically writes ~/.config/waveterm/jira-cache.json.
//
// Error semantics:
//   - /myself failure:       fatal, no cache file written (D-ERR-03)
//   - /search failure:       fatal, no cache file written (D-ERR-02)
//   - per-issue GetIssue:    logged, issue skipped, Refresh continues (D-ERR-01)
//   - cache marshal/write:   fatal, returns wrapped error
//
// Security (T-01-02 extension): never logs Config fields or response Body.
// Error formatting uses %v only — see RESEARCH Pitfall 8.
func Refresh(ctx context.Context, opts RefreshOpts) (*RefreshReport, error) {
	start := time.Now()

	var client *Client
	if opts.HTTPClient != nil {
		client = NewClientWithHTTP(opts.Config, opts.HTTPClient)
	} else {
		client = NewClient(opts.Config)
	}

	// Step 3 first — fail fast on auth before doing any search work (D-ERR-03).
	me, err := client.GetMyself(ctx)
	if err != nil {
		return nil, fmt.Errorf("jira refresh: GetMyself failed: %v", err)
	}

	// Resolve cache path + preserve-localPath snapshot BEFORE fetching so a
	// fatal search failure leaves the existing cache untouched (D-ERR-02).
	cachePath, err := cacheFilePath()
	if err != nil {
		return nil, fmt.Errorf("jira refresh: %v", err)
	}
	localPaths := loadExistingLocalPaths(cachePath) // best-effort, never errors (D-FLOW-04)

	// Step 1 — paginate search until isLast or empty nextPageToken (D-FLOW-01).
	progress(opts.OnProgress, "search", 0, 0)
	allKeys, err := paginateSearch(ctx, client, opts.Config)
	if err != nil {
		return nil, fmt.Errorf("jira refresh: search failed: %v", err)
	}
	progress(opts.OnProgress, "search", len(allKeys), 0)

	// Step 2 — fetch each issue sequentially (D-CONC-01), build cache entries.
	issueFieldList := []string{
		"summary", "description", "status", "issuetype", "priority",
		"project", "created", "updated", "attachment", "comment",
	}

	cacheIssues := make([]JiraCacheIssue, 0, len(allKeys))
	for i, key := range allKeys {
		issue, gerr := client.GetIssue(ctx, key, GetIssueOpts{Fields: issueFieldList})
		if gerr != nil {
			// D-ERR-01: log (%v only, T-01-02) and skip.
			log.Printf("jira refresh: GetIssue %s failed: %v (skipping)", key, gerr)
			progress(opts.OnProgress, "fetch", i+1, len(allKeys))
			continue
		}
		cacheIssues = append(cacheIssues, buildCacheIssue(issue, opts.Config.BaseUrl, localPaths))
		progress(opts.OnProgress, "fetch", i+1, len(allKeys))
	}

	// Step 5 — marshal + atomic write (D-FLOW-05).
	progress(opts.OnProgress, "write", 0, 1)
	cache := JiraCache{
		CloudId:   opts.Config.CloudId,
		BaseUrl:   opts.Config.BaseUrl,
		AccountId: me.AccountID,
		FetchedAt: time.Now().UTC().Format(time.RFC3339),
		Issues:    cacheIssues,
	}
	data, err := json.MarshalIndent(&cache, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("jira refresh: marshal cache: %v", err)
	}
	// Ensure parent directory exists (RESEARCH Pitfall 5 — fresh install has no ~/.config/waveterm yet).
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return nil, fmt.Errorf("jira refresh: mkdir cache parent: %v", err)
	}
	if err := fileutil.AtomicWriteFile(cachePath, data, 0o644); err != nil {
		return nil, fmt.Errorf("jira refresh: write cache: %v", err)
	}
	progress(opts.OnProgress, "write", 1, 1)

	return &RefreshReport{
		IssueCount:      len(cacheIssues),
		AttachmentCount: countAttachments(cacheIssues),
		CommentCount:    countComments(cacheIssues),
		Elapsed:         time.Since(start),
		CachePath:       cachePath,
	}, nil
}

// paginateSearch calls SearchIssues repeatedly until IsLast is true or the
// server returns an empty NextPageToken. Collects issue keys only —
// per D-FLOW-02 full-field fetching happens in Step 2 via GetIssue.
func paginateSearch(ctx context.Context, client *Client, cfg Config) ([]string, error) {
	var keys []string
	var nextToken string
	// Defensive upper bound: 1000 pages × 50/page = 50_000 issues. More than
	// a realistic personal JQL. Prevents an infinite loop if the server
	// violates its own isLast contract (T-02-13).
	const maxPages = 1000
	for page := 0; page < maxPages; page++ {
		res, err := client.SearchIssues(ctx, SearchOpts{
			JQL:           cfg.Jql,
			NextPageToken: nextToken,
			MaxResults:    cfg.PageSize,
			Fields:        []string{"summary"}, // minimal — full fetch in Step 2
		})
		if err != nil {
			return nil, err
		}
		for _, ref := range res.Issues {
			keys = append(keys, ref.Key)
		}
		if res.IsLast || res.NextPageToken == "" {
			return keys, nil
		}
		nextToken = res.NextPageToken
	}
	return keys, fmt.Errorf("jira refresh: search pagination exceeded %d pages", maxPages)
}

// buildCacheIssue converts a wire-format *Issue into a cache-format
// JiraCacheIssue. Covers D-CACHE-03..08 and the comment transforms from
// D-CACHE-06..07. See RESEARCH Pattern 2 for the decision log.
func buildCacheIssue(issue *Issue, baseUrl string, localPaths map[string]string) JiraCacheIssue {
	// Description: ADF → markdown. Malformed ADF on ONE issue must not fail
	// refresh — degrade to "" and log.
	desc, err := ADFToMarkdown(issue.Fields.Description)
	if err != nil {
		log.Printf("jira refresh: ADF description parse error on %s: %v", issue.Key, err)
		desc = ""
	}

	// Attachments: metadata only. Always non-nil slice (Pitfall 3).
	atts := make([]JiraCacheAttachment, 0, len(issue.Fields.Attachment))
	for _, a := range issue.Fields.Attachment {
		lp := localPaths[issue.Key+"::"+a.ID] // "" if unseen
		atts = append(atts, JiraCacheAttachment{
			ID:        a.ID,
			Filename:  a.Filename,
			MimeType:  a.MimeType,
			Size:      a.Size,
			LocalPath: lp,
			WebUrl:    a.Content, // Jira already returns the site-pattern URL (A3)
		})
	}

	// Comments: wire is oldest-first. Keep LAST 10 (Pitfall 2).
	raw := issue.Fields.Comment.Comments
	keep := raw
	if len(raw) > 10 {
		keep = raw[len(raw)-10:]
	}
	cmts := make([]JiraCacheComment, 0, len(keep))
	var lastAt string
	for _, c := range keep {
		body, cerr := ADFToMarkdown(c.Body)
		if cerr != nil {
			log.Printf("jira refresh: ADF comment parse error on %s comment %s: %v", issue.Key, c.ID, cerr)
			body = ""
		}
		truncated := false
		if len(body) > 2000 {
			body = body[:2000]
			truncated = true
		}
		// Author flatten (Pitfall 1). DisplayName preferred; accountId fallback.
		author := c.Author.DisplayName
		if author == "" {
			author = c.Author.AccountID
		}
		cmts = append(cmts, JiraCacheComment{
			ID:        c.ID,
			Author:    author,
			Created:   c.Created,
			Updated:   c.Updated,
			Body:      body,
			Truncated: truncated,
		})
		// lastCommentAt = max(updated, created) across kept (D-CACHE-07).
		// ISO8601 timestamps with consistent Jira timezone are lexically
		// sortable (RESEARCH A1).
		candidate := c.Updated
		if c.Created > candidate {
			candidate = c.Created
		}
		if candidate > lastAt {
			lastAt = candidate
		}
	}

	return JiraCacheIssue{
		Key:            issue.Key,
		ID:             issue.ID,
		Summary:        issue.Fields.Summary,
		Description:    desc,
		Status:         issue.Fields.Status.Name,
		StatusCategory: statusCategoryFromKey(issue.Fields.Status.StatusCategory.Key),
		IssueType:      issue.Fields.IssueType.Name,
		Priority:       issue.Fields.Priority.Name,
		ProjectKey:     issue.Fields.Project.Key,
		ProjectName:    issue.Fields.Project.Name,
		Updated:        issue.Fields.Updated,
		Created:        issue.Fields.Created,
		WebUrl:         baseUrl + "/browse/" + issue.Key,
		Attachments:    atts,
		Comments:       cmts,
		CommentCount:   issue.Fields.Comment.Total, // wire total, may exceed len(cmts)
		LastCommentAt:  lastAt,
	}
}

// statusCategoryFromKey maps Jira's statusCategory.key to the 3-value enum
// the widget consumes (D-CACHE-08). Unknown / missing → "new".
func statusCategoryFromKey(k string) string {
	switch k {
	case "new", "indeterminate", "done":
		return k
	default:
		return "new"
	}
}

// loadExistingLocalPaths reads ~/.config/waveterm/jira-cache.json if it
// exists and extracts a flat map of (issueKey + "::" + attachmentID) →
// localPath for every attachment with a non-empty LocalPath. Any error —
// file missing, permission denied, malformed JSON, schema drift — returns
// an empty map with no error surfaced (D-FLOW-04). This is deliberate:
// Refresh's job is to produce a fresh cache, not to validate the prior one.
func loadExistingLocalPaths(cachePath string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return out
	}
	var existing JiraCache
	if err := json.Unmarshal(data, &existing); err != nil {
		return out
	}
	for _, iss := range existing.Issues {
		for _, a := range iss.Attachments {
			if a.LocalPath != "" {
				out[iss.Key+"::"+a.ID] = a.LocalPath
			}
		}
	}
	return out
}

// cacheFilePath returns the literal ~/.config/waveterm/jira-cache.json path
// (D-CACHE-01). Uses os.UserHomeDir() which honors $HOME on POSIX and
// %USERPROFILE% on Windows — consistent with pkg/jira/config.go's LoadConfig.
func cacheFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %v", err)
	}
	return filepath.Join(home, ".config", "waveterm", "jira-cache.json"), nil
}

// progress invokes cb if non-nil. Centralizing the nil check keeps call
// sites clean (D-PROG-01).
func progress(cb func(string, int, int), stage string, cur, total int) {
	if cb != nil {
		cb(stage, cur, total)
	}
}

func countAttachments(issues []JiraCacheIssue) int {
	n := 0
	for _, i := range issues {
		n += len(i.Attachments)
	}
	return n
}

func countComments(issues []JiraCacheIssue) int {
	n := 0
	for _, i := range issues {
		n += len(i.Comments)
	}
	return n
}

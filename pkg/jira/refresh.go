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
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

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
	//   "build"  — building cache entries from received issues. current =
	//              1-based index of the issue just processed; total = total
	//              received (no HTTP, just local ADF conversion).
	//   "write"  — cache write. First call ("write", 0, 1) before marshal;
	//              final call ("write", 1, 1) after successful rename.
	//
	// The callback runs on Refresh's goroutine; it must not block.
	OnProgress func(stage string, current, total int)

	// ForceFull, when true, bypasses the incremental refresh path and re-fetches
	// every issue the JQL returns. Default false = auto-incremental when a valid
	// prior cache is present and recent enough.
	ForceFull bool

	// StaleAfter defines how old the existing cache can be before we auto-upgrade
	// a delta refresh to a full one. Zero = 24h default. Issues deleted upstream
	// between full refreshes remain in the cache until the next full (delta
	// can't detect deletions).
	StaleAfter time.Duration

	// StatusCategories, if non-empty, restricts the fetch to issues whose Jira
	// statusCategory is in this list. Valid values: "new", "indeterminate",
	// "done". Used by the widget to skip "done" issues server-side when the
	// user has that filter toggled off.
	StatusCategories []string
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

	// Load existing cache for delta + localPath preservation (D-FLOW-04).
	existing := loadExistingCache(cachePath) // best-effort, never errors
	localPaths := localPathsFromCache(existing)

	// Determine whether this is a delta or full refresh.
	staleAfter := opts.StaleAfter
	if staleAfter == 0 {
		staleAfter = 24 * time.Hour
	}
	isDelta := false
	var extraClauses []string
	if !opts.ForceFull && existing != nil && existing.FetchedAt != "" && len(existing.Issues) > 0 {
		if prev, perr := time.Parse(time.RFC3339, existing.FetchedAt); perr == nil {
			age := time.Since(prev)
			if age < staleAfter {
				isDelta = true
				// 60s clock-skew buffer; JQL date format = "YYYY-MM-DD HH:mm".
				// Jira treats bare quoted dates as local time; we shift to
				// UTC and shave 60s to be inclusive of boundary edits.
				cutoff := prev.UTC().Add(-60 * time.Second)
				extraClauses = append(extraClauses,
					fmt.Sprintf(`updated >= "%s"`, cutoff.Format("2006-01-02 15:04")))
			}
		}
	}
	// Status category restriction (widget filter propagation).
	// Jira's JQL does NOT accept the lowercase key strings ("new", "indeterminate",
	// "done") directly — those are API keys, not JQL tokens. Use the numeric
	// statusCategory IDs instead (locale-independent, documented in Jira REST):
	//   2 = "To Do"        (key "new")
	//   4 = "In Progress"  (key "indeterminate")
	//   3 = "Done"         (key "done")
	if len(opts.StatusCategories) > 0 {
		categoryIds := make([]string, 0, len(opts.StatusCategories))
		for _, s := range opts.StatusCategories {
			// Allow-list prevents JQL injection via unknown values.
			switch s {
			case "new":
				categoryIds = append(categoryIds, "2")
			case "indeterminate":
				categoryIds = append(categoryIds, "4")
			case "done":
				categoryIds = append(categoryIds, "3")
			}
		}
		if len(categoryIds) > 0 {
			extraClauses = append(extraClauses,
				fmt.Sprintf("statusCategory in (%s)", strings.Join(categoryIds, ", ")))
		}
	}
	extraJQL := strings.Join(extraClauses, " AND ")

	// Step 1 — paginate search WITH FULL FIELDS (collapses prior /search + N × /issue calls).
	progress(opts.OnProgress, "search", 0, 0)
	issueRefs, err := paginateSearchFull(ctx, client, opts.Config, extraJQL)
	if err != nil {
		return nil, fmt.Errorf("jira refresh: search failed: %v", err)
	}
	progress(opts.OnProgress, "search", len(issueRefs), 0)

	// Step 2 — build cache entries from the search response directly.
	fetchedSet := make(map[string]bool, len(issueRefs))
	cacheIssues := make([]JiraCacheIssue, 0, len(issueRefs))
	total := len(issueRefs)
	for i, ref := range issueRefs {
		issue := Issue{ID: ref.ID, Key: ref.Key, Self: ref.Self, Fields: ref.Fields}
		cacheIssues = append(cacheIssues, buildCacheIssue(&issue, opts.Config.BaseUrl, localPaths))
		fetchedSet[ref.Key] = true
		progress(opts.OnProgress, "build", i+1, total)
	}

	// In delta mode, append any existing issues NOT re-fetched (unchanged since last refresh).
	// When a status filter is active, drop existing issues whose category no longer
	// matches — this lets a widget-side "done" toggle take effect immediately on
	// the next delta refresh without requiring a full reconcile.
	if isDelta && existing != nil {
		allowedCategories := map[string]bool{}
		for _, s := range opts.StatusCategories {
			allowedCategories[s] = true
		}
		filterOn := len(allowedCategories) > 0
		for _, old := range existing.Issues {
			if fetchedSet[old.Key] {
				continue
			}
			if filterOn && !allowedCategories[old.StatusCategory] {
				continue
			}
			cacheIssues = append(cacheIssues, old)
		}
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
	// Mode 0o600: cache contains issue summaries, descriptions, comment bodies,
	// and accountId — match config.json's 0o600 to avoid leaking to other local
	// users on a shared workstation. (WR-02)
	if err := fileutil.AtomicWriteFile(cachePath, data, 0o600); err != nil {
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

// paginateSearchFull calls SearchIssues with all cache-relevant fields so the
// response includes full issue data — no per-issue GetIssue call needed.
// This collapses the previous ~1+N HTTP roundtrips (1 search per page + N full
// fetches) down to just the search pagination pass. If `extraJQL` is non-empty,
// it is AND'd into the base JQL (used for delta refresh).
func paginateSearchFull(ctx context.Context, client *Client, cfg Config, extraJQL string) ([]IssueRef, error) {
	jql := combineJQL(cfg.Jql, extraJQL)
	fieldList := []string{
		"summary", "description", "status", "issuetype", "priority",
		"project", "created", "updated", "attachment", "comment",
	}
	var all []IssueRef
	var nextToken string
	// Defensive upper bound: 1000 pages × 50/page = 50_000 issues (T-02-13).
	const maxPages = 1000
	for page := 0; page < maxPages; page++ {
		res, err := client.SearchIssues(ctx, SearchOpts{
			JQL:           jql,
			NextPageToken: nextToken,
			MaxResults:    cfg.PageSize,
			Fields:        fieldList,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, res.Issues...)
		if res.IsLast || res.NextPageToken == "" {
			return all, nil
		}
		nextToken = res.NextPageToken
	}
	return all, fmt.Errorf("jira refresh: search pagination exceeded %d pages", maxPages)
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
			// Truncate on a valid UTF-8 rune boundary, not a byte boundary.
			// Byte-slicing at 2000 could split a multi-byte rune (Korean/CJK/
			// emoji), producing invalid UTF-8 that json.Marshal would escape
			// or the widget would render as U+FFFD. Walk back to the start
			// of the rune that overruns the cap. (WR-01)
			cut := 2000
			for cut > 0 && !utf8.RuneStart(body[cut]) {
				cut--
			}
			body = body[:cut]
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
	// Reverse so cache stores newest-first (user-facing order).
	for i, j := 0, len(cmts)-1; i < j; i, j = i+1, j-1 {
		cmts[i], cmts[j] = cmts[j], cmts[i]
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

// orderByRe splits a JQL string on the first " ORDER BY " (case-insensitive).
// The base JQL typically ends in ` ORDER BY updated DESC`; when we AND extra
// clauses (delta / statusCategory filter) we must insert them BEFORE the
// ORDER BY, otherwise Jira returns 400.
var orderByRe = regexp.MustCompile(`(?i)\s+ORDER\s+BY\s+`)

// combineJQL merges base JQL with additional WHERE clauses. Handles the
// ORDER BY split described above. Empty extra → returns base unchanged.
func combineJQL(base, extra string) string {
	if extra == "" {
		return base
	}
	loc := orderByRe.FindStringIndex(base)
	if loc == nil {
		return "(" + base + ") AND " + extra
	}
	where := base[:loc[0]]
	orderBy := base[loc[0]:] // includes leading whitespace + "ORDER BY ..."
	return "(" + where + ") AND " + extra + orderBy
}

// loadExistingCache reads ~/.config/waveterm/jira-cache.json if present and
// returns the decoded struct. Any error (file missing, permission denied,
// malformed JSON) returns nil — refresh is free to proceed as a full fetch.
func loadExistingCache(cachePath string) *JiraCache {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	var existing JiraCache
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil
	}
	return &existing
}

// localPathsFromCache builds the (issueKey + "::" + attachmentID) → localPath
// map used by buildCacheIssue to preserve previously-downloaded attachment
// paths (D-FLOW-04). Nil cache = empty map.
func localPathsFromCache(c *JiraCache) map[string]string {
	out := map[string]string{}
	if c == nil {
		return out
	}
	for _, iss := range c.Issues {
		for _, a := range iss.Attachments {
			if a.LocalPath != "" {
				out[iss.Key+"::"+a.ID] = a.LocalPath
			}
		}
	}
	return out
}

// loadExistingLocalPaths is kept as a thin adapter for existing tests that
// still reference it directly. Prefer loadExistingCache + localPathsFromCache
// for new code paths.
func loadExistingLocalPaths(cachePath string) map[string]string {
	return localPathsFromCache(loadExistingCache(cachePath))
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

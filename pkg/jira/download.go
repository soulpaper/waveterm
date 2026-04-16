// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// download.go — on-demand attachment download (Phase 5, D-DL-01..06).
//
// DownloadAttachments is the entry point called by JiraDownloadCommand (wshserver)
// and `wsh jira download` (CLI). It streams attachment files to disk at
// ~/.config/waveterm/jira-attachments/<KEY>/<filename>, then updates the cache's
// localPath field so the widget picks up local files on next load.
//
// Security invariants:
//   - Uses the same Basic auth as all other Client methods (setCommonHeaders).
//   - Downloads via the attachment's Content URL (Jira REST API content endpoint),
//     which requires auth — NOT the anonymous public URL.
//   - Uses io.Copy for streaming (D-DL-05) — no in-memory buffer for large files.
//   - Cache updates are atomic via fileutil.AtomicWriteFile.

package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wavetermdev/waveterm/pkg/util/fileutil"
)

// cacheFilePathFn and attachmentDirFn are test seams. Production code uses the
// real implementations; tests override them to use temp directories.
var (
	cacheFilePathFn = cacheFilePath
	attachmentDirFn = attachmentDir
)

// DownloadResult describes the outcome of downloading a single attachment.
type DownloadResult struct {
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`       // bytes written
	LocalPath string `json:"localPath"`  // absolute path on disk
	Skipped   bool   `json:"skipped"`    // true if file already existed at localPath
}

// DownloadReport summarizes a DownloadAttachments call.
type DownloadReport struct {
	IssueKey   string           `json:"issueKey"`
	Downloaded []DownloadResult `json:"downloaded"`
	TotalBytes int64            `json:"totalBytes"`
}

// DownloadOpts configures a DownloadAttachments call.
type DownloadOpts struct {
	// Config is the loaded Jira configuration (for auth).
	Config Config

	// HTTPClient, if non-nil, overrides the default http.Client. Tests pass
	// httptest.NewServer().Client() here.
	HTTPClient *http.Client

	// IssueKey is the Jira issue key (e.g. "ITSM-3135"). Required.
	IssueKey string

	// Filename, if non-empty, restricts the download to a single attachment
	// matching this filename. If empty, all attachments for the issue are
	// downloaded (D-DL-01).
	Filename string
}

// DownloadAttachments fetches attachment file(s) for the given issue key from
// the Jira REST API and writes them to ~/.config/waveterm/jira-attachments/<KEY>/.
// After downloading, it updates the on-disk cache's localPath entries atomically.
//
// The function reads the existing cache to find attachment metadata (ID, Content
// URL) for the given issue. If Filename is specified, only that attachment is
// downloaded; otherwise all attachments are fetched.
//
// Error semantics:
//   - Missing cache or issue not in cache: error
//   - No matching attachment (for Filename filter): error
//   - Individual download failure: returns error (no partial commit)
//   - Cache update failure: returns error (files are on disk but cache is stale)
func DownloadAttachments(ctx context.Context, opts DownloadOpts) (*DownloadReport, error) {
	if opts.IssueKey == "" {
		return nil, fmt.Errorf("jira download: issue key is required")
	}

	// Read the existing cache to find attachment metadata.
	cachePath, err := cacheFilePathFn()
	if err != nil {
		return nil, fmt.Errorf("jira download: %v", err)
	}
	cache, err := readCache(cachePath)
	if err != nil {
		return nil, fmt.Errorf("jira download: cannot read cache: %v", err)
	}

	// Find the issue in the cache.
	issue, issueIdx := findCacheIssue(cache, opts.IssueKey)
	if issue == nil {
		return nil, fmt.Errorf("jira download: issue %s not found in cache (run `wsh jira refresh` first)", opts.IssueKey)
	}

	// Filter attachments.
	attachments := issue.Attachments
	if opts.Filename != "" {
		filtered := filterAttachments(attachments, opts.Filename)
		if len(filtered) == 0 {
			return nil, fmt.Errorf("jira download: no attachment named %q on issue %s", opts.Filename, opts.IssueKey)
		}
		attachments = filtered
	}
	if len(attachments) == 0 {
		return nil, fmt.Errorf("jira download: issue %s has no attachments", opts.IssueKey)
	}

	// Build client for downloading.
	var client *Client
	if opts.HTTPClient != nil {
		client = NewClientWithHTTP(opts.Config, opts.HTTPClient)
	} else {
		client = NewClient(opts.Config)
	}

	// Resolve the download directory.
	downloadDir, err := attachmentDirFn(opts.IssueKey)
	if err != nil {
		return nil, fmt.Errorf("jira download: %v", err)
	}
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		return nil, fmt.Errorf("jira download: mkdir %s: %v", downloadDir, err)
	}

	// Download each attachment sequentially.
	report := &DownloadReport{
		IssueKey:   opts.IssueKey,
		Downloaded: make([]DownloadResult, 0, len(attachments)),
	}

	for _, att := range attachments {
		localPath := filepath.Join(downloadDir, att.Filename)

		// Skip if already downloaded.
		if att.LocalPath != "" {
			if _, serr := os.Stat(att.LocalPath); serr == nil {
				report.Downloaded = append(report.Downloaded, DownloadResult{
					Filename:  att.Filename,
					Size:      att.Size,
					LocalPath: att.LocalPath,
					Skipped:   true,
				})
				continue
			}
		}

		// Download from the Content URL (which is the Jira REST API endpoint).
		written, derr := client.downloadFile(ctx, att.WebUrl, localPath)
		if derr != nil {
			return nil, fmt.Errorf("jira download: downloading %s: %v", att.Filename, derr)
		}

		report.Downloaded = append(report.Downloaded, DownloadResult{
			Filename:  att.Filename,
			Size:      written,
			LocalPath: localPath,
		})
		report.TotalBytes += written
	}

	// Update cache with localPath entries.
	if err := updateCacheLocalPaths(cachePath, cache, issueIdx, report); err != nil {
		return nil, fmt.Errorf("jira download: cache update failed: %v", err)
	}

	return report, nil
}

// downloadFile streams the content at url to destPath using Basic auth.
// Returns the number of bytes written.
func (c *Client) downloadFile(ctx context.Context, contentURL, destPath string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, contentURL, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %v", err)
	}
	c.setCommonHeaders(req)

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		return 0, &APIError{
			StatusCode: resp.StatusCode,
			Endpoint:   req.URL.Path,
			Method:     req.Method,
			Body:       string(body),
		}
	}

	// Write to a temp file first, then rename for atomic writes.
	tmpPath := destPath + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return 0, fmt.Errorf("create file: %v", err)
	}

	written, err := io.Copy(f, resp.Body)
	if closeErr := f.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("write file: %v", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return 0, fmt.Errorf("rename file: %v", err)
	}

	return written, nil
}

// readCache reads and parses the on-disk cache file.
func readCache(cachePath string) (*JiraCache, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}
	var cache JiraCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("parse cache: %v", err)
	}
	return &cache, nil
}

// findCacheIssue returns the issue with the given key and its index, or nil,-1.
func findCacheIssue(cache *JiraCache, key string) (*JiraCacheIssue, int) {
	for i := range cache.Issues {
		if cache.Issues[i].Key == key {
			return &cache.Issues[i], i
		}
	}
	return nil, -1
}

// filterAttachments returns attachments matching the given filename.
func filterAttachments(atts []JiraCacheAttachment, filename string) []JiraCacheAttachment {
	var out []JiraCacheAttachment
	for _, a := range atts {
		if a.Filename == filename {
			out = append(out, a)
		}
	}
	return out
}

// updateCacheLocalPaths updates the cache file with the localPath values from
// the download report. Uses atomic write to prevent corruption.
func updateCacheLocalPaths(cachePath string, cache *JiraCache, issueIdx int, report *DownloadReport) error {
	issue := &cache.Issues[issueIdx]
	for _, dl := range report.Downloaded {
		if dl.Skipped {
			continue
		}
		for j := range issue.Attachments {
			if issue.Attachments[j].Filename == dl.Filename {
				issue.Attachments[j].LocalPath = dl.LocalPath
			}
		}
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cache: %v", err)
	}
	return fileutil.AtomicWriteFile(cachePath, data, 0o600)
}

// attachmentDir returns the absolute path to the attachment download directory
// for the given issue key: ~/.config/waveterm/jira-attachments/<KEY>/
func attachmentDir(issueKey string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot resolve home directory: %v", err)
	}
	return filepath.Join(home, ".config", "waveterm", "jira-attachments", issueKey), nil
}

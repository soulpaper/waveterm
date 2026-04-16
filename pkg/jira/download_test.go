// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestDownloadAttachments_Success verifies the happy path: attachment content is
// streamed to disk, cache is updated with localPath. Uses httptest to serve
// attachment content at the expected /rest/api/3/attachment/content/{id} path.
func TestDownloadAttachments_Success(t *testing.T) {
	// Serve attachment content via httptest.
	const fileContent = "hello attachment content"
	const attID = "12345"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept any path — the Content URL in the cache points here.
		if r.Header.Get("Authorization") == "" {
			t.Errorf("expected Authorization header")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.WriteHeader(200)
		w.Write([]byte(fileContent))
	}))
	defer ts.Close()

	// Set up a temporary directory for both cache and downloads.
	tmpDir := t.TempDir()

	// Create a seed cache file.
	cache := JiraCache{
		CloudId:   "cloud1",
		BaseUrl:   ts.URL,
		AccountId: "acc1",
		FetchedAt: "2026-04-15T00:00:00Z",
		Issues: []JiraCacheIssue{
			{
				Key:     "TEST-1",
				ID:      "1001",
				Summary: "Test issue",
				Attachments: []JiraCacheAttachment{
					{
						ID:       attID,
						Filename: "report.pdf",
						MimeType: "application/pdf",
						Size:     1024,
						WebUrl:   ts.URL + "/rest/api/3/attachment/content/" + attID,
					},
				},
				Comments: []JiraCacheComment{},
			},
		},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	if err := os.WriteFile(cachePath, cacheData, 0o600); err != nil {
		t.Fatalf("write seed cache: %v", err)
	}

	// Override cacheFilePath and attachmentDir for testing.
	origCacheFP := cacheFilePathFn
	origAttDir := attachmentDirFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	attachmentDirFn = func(key string) (string, error) {
		return filepath.Join(tmpDir, "jira-attachments", key), nil
	}
	t.Cleanup(func() {
		cacheFilePathFn = origCacheFP
		attachmentDirFn = origAttDir
	})

	report, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:     Config{BaseUrl: ts.URL, Email: "u@example.com", ApiToken: "tok"},
		HTTPClient: ts.Client(),
		IssueKey:   "TEST-1",
	})
	if err != nil {
		t.Fatalf("DownloadAttachments error: %v", err)
	}

	if len(report.Downloaded) != 1 {
		t.Fatalf("expected 1 downloaded, got %d", len(report.Downloaded))
	}
	dl := report.Downloaded[0]
	if dl.Filename != "report.pdf" {
		t.Errorf("filename = %q, want report.pdf", dl.Filename)
	}
	if dl.Skipped {
		t.Errorf("expected not skipped")
	}
	if dl.Size != int64(len(fileContent)) {
		t.Errorf("size = %d, want %d", dl.Size, len(fileContent))
	}

	// Verify file on disk.
	data, err := os.ReadFile(dl.LocalPath)
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != fileContent {
		t.Errorf("file content = %q, want %q", string(data), fileContent)
	}

	// Verify cache was updated.
	updatedCache, err := readCache(cachePath)
	if err != nil {
		t.Fatalf("read updated cache: %v", err)
	}
	if updatedCache.Issues[0].Attachments[0].LocalPath != dl.LocalPath {
		t.Errorf("cache localPath = %q, want %q", updatedCache.Issues[0].Attachments[0].LocalPath, dl.LocalPath)
	}
}

// TestDownloadAttachments_FilenameFilter verifies that specifying a filename
// restricts the download to just that attachment.
func TestDownloadAttachments_FilenameFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("file-data"))
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	cache := JiraCache{
		CloudId: "c", BaseUrl: ts.URL, AccountId: "a", FetchedAt: "2026-01-01T00:00:00Z",
		Issues: []JiraCacheIssue{{
			Key: "TEST-2", ID: "2002", Summary: "Two attachments",
			Attachments: []JiraCacheAttachment{
				{ID: "a1", Filename: "first.txt", Size: 100, WebUrl: ts.URL + "/att/a1"},
				{ID: "a2", Filename: "second.txt", Size: 200, WebUrl: ts.URL + "/att/a2"},
			},
			Comments: []JiraCacheComment{},
		}},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	os.WriteFile(cachePath, cacheData, 0o600)

	origCacheFP := cacheFilePathFn
	origAttDir := attachmentDirFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	attachmentDirFn = func(key string) (string, error) {
		return filepath.Join(tmpDir, "jira-attachments", key), nil
	}
	t.Cleanup(func() {
		cacheFilePathFn = origCacheFP
		attachmentDirFn = origAttDir
	})

	report, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:     Config{BaseUrl: ts.URL, Email: "u", ApiToken: "t"},
		HTTPClient: ts.Client(),
		IssueKey:   "TEST-2",
		Filename:   "second.txt",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(report.Downloaded) != 1 {
		t.Fatalf("expected 1 downloaded, got %d", len(report.Downloaded))
	}
	if report.Downloaded[0].Filename != "second.txt" {
		t.Errorf("filename = %q, want second.txt", report.Downloaded[0].Filename)
	}
}

// TestDownloadAttachments_IssueNotInCache verifies error when issue key is not
// present in the cache.
func TestDownloadAttachments_IssueNotInCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache := JiraCache{
		CloudId: "c", BaseUrl: "https://x", AccountId: "a", FetchedAt: "2026-01-01T00:00:00Z",
		Issues: []JiraCacheIssue{{
			Key: "OTHER-1", ID: "9999", Summary: "different issue",
			Attachments: []JiraCacheAttachment{},
			Comments:    []JiraCacheComment{},
		}},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	os.WriteFile(cachePath, cacheData, 0o600)

	origCacheFP := cacheFilePathFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	t.Cleanup(func() { cacheFilePathFn = origCacheFP })

	_, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:   Config{BaseUrl: "https://x", Email: "u", ApiToken: "t"},
		IssueKey: "MISSING-1",
	})
	if err == nil {
		t.Fatal("expected error for missing issue")
	}
	if got := err.Error(); !contains(got, "not found in cache") {
		t.Errorf("error = %q, want to contain 'not found in cache'", got)
	}
}

// TestDownloadAttachments_NoFilenameMatch verifies error when a specific filename
// is requested but doesn't exist on the issue.
func TestDownloadAttachments_NoFilenameMatch(t *testing.T) {
	tmpDir := t.TempDir()
	cache := JiraCache{
		CloudId: "c", BaseUrl: "https://x", AccountId: "a", FetchedAt: "2026-01-01T00:00:00Z",
		Issues: []JiraCacheIssue{{
			Key: "TEST-3", ID: "3003", Summary: "issue with att",
			Attachments: []JiraCacheAttachment{
				{ID: "a1", Filename: "existing.txt", Size: 50, WebUrl: "https://x/att"},
			},
			Comments: []JiraCacheComment{},
		}},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	os.WriteFile(cachePath, cacheData, 0o600)

	origCacheFP := cacheFilePathFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	t.Cleanup(func() { cacheFilePathFn = origCacheFP })

	_, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:   Config{BaseUrl: "https://x", Email: "u", ApiToken: "t"},
		IssueKey: "TEST-3",
		Filename: "nonexistent.pdf",
	})
	if err == nil {
		t.Fatal("expected error for no matching filename")
	}
	if got := err.Error(); !contains(got, "no attachment named") {
		t.Errorf("error = %q, want to contain 'no attachment named'", got)
	}
}

// TestDownloadAttachments_ServerError verifies that non-2xx response from the
// download endpoint returns an error.
func TestDownloadAttachments_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", 403)
	}))
	defer ts.Close()

	tmpDir := t.TempDir()
	cache := JiraCache{
		CloudId: "c", BaseUrl: ts.URL, AccountId: "a", FetchedAt: "2026-01-01T00:00:00Z",
		Issues: []JiraCacheIssue{{
			Key: "TEST-4", ID: "4004", Summary: "issue",
			Attachments: []JiraCacheAttachment{
				{ID: "a1", Filename: "secret.pdf", Size: 100, WebUrl: ts.URL + "/att/a1"},
			},
			Comments: []JiraCacheComment{},
		}},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	os.WriteFile(cachePath, cacheData, 0o600)

	origCacheFP := cacheFilePathFn
	origAttDir := attachmentDirFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	attachmentDirFn = func(key string) (string, error) {
		return filepath.Join(tmpDir, "jira-attachments", key), nil
	}
	t.Cleanup(func() {
		cacheFilePathFn = origCacheFP
		attachmentDirFn = origAttDir
	})

	_, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:     Config{BaseUrl: ts.URL, Email: "u", ApiToken: "t"},
		HTTPClient: ts.Client(),
		IssueKey:   "TEST-4",
	})
	if err == nil {
		t.Fatal("expected error for server 403")
	}
	if got := err.Error(); !contains(got, "downloading secret.pdf") {
		t.Errorf("error = %q, want to contain 'downloading secret.pdf'", got)
	}
}

// TestDownloadAttachments_EmptyIssueKey verifies the validation.
func TestDownloadAttachments_EmptyIssueKey(t *testing.T) {
	_, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:   Config{BaseUrl: "https://x", Email: "u", ApiToken: "t"},
		IssueKey: "",
	})
	if err == nil {
		t.Fatal("expected error for empty issue key")
	}
}

// TestDownloadAttachments_SkipsExisting verifies that already-downloaded files
// (where localPath exists on disk) are skipped.
func TestDownloadAttachments_SkipsExisting(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-create the downloaded file.
	attDir := filepath.Join(tmpDir, "jira-attachments", "TEST-5")
	os.MkdirAll(attDir, 0o755)
	existingPath := filepath.Join(attDir, "already.txt")
	os.WriteFile(existingPath, []byte("old content"), 0o600)

	cache := JiraCache{
		CloudId: "c", BaseUrl: "https://x", AccountId: "a", FetchedAt: "2026-01-01T00:00:00Z",
		Issues: []JiraCacheIssue{{
			Key: "TEST-5", ID: "5005", Summary: "issue",
			Attachments: []JiraCacheAttachment{
				{ID: "a1", Filename: "already.txt", Size: 11, LocalPath: existingPath, WebUrl: "https://x/att/a1"},
			},
			Comments: []JiraCacheComment{},
		}},
	}
	cacheData, _ := json.MarshalIndent(&cache, "", "  ")
	cachePath := filepath.Join(tmpDir, "jira-cache.json")
	os.WriteFile(cachePath, cacheData, 0o600)

	origCacheFP := cacheFilePathFn
	origAttDir := attachmentDirFn
	cacheFilePathFn = func() (string, error) { return cachePath, nil }
	attachmentDirFn = func(key string) (string, error) {
		return filepath.Join(tmpDir, "jira-attachments", key), nil
	}
	t.Cleanup(func() {
		cacheFilePathFn = origCacheFP
		attachmentDirFn = origAttDir
	})

	report, err := DownloadAttachments(context.Background(), DownloadOpts{
		Config:   Config{BaseUrl: "https://x", Email: "u", ApiToken: "t"},
		IssueKey: "TEST-5",
	})
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(report.Downloaded) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Downloaded))
	}
	if !report.Downloaded[0].Skipped {
		t.Error("expected Skipped=true for existing file")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	return fmt.Sprintf("%s", s) != "" && len(s) > 0 && findSubstr(s, substr)
}

func findSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

// refresh_test.go — Nyquist RED tests for the Phase 2 cache orchestrator.
//
// Every test in this file is authored BEFORE refresh.go exists. They will
// fail to compile until Plan 02 defines Refresh, RefreshOpts, and
// RefreshReport. That compile failure is the intended signal of the RED
// phase — Plan 02 turns them GREEN by implementing the cache orchestrator.
//
// White-box test (package jira) matching the Phase 1 convention so the
// tests can touch the cache_types.go structs (JiraCache, JiraCacheIssue,
// ...) directly without going through an exported decoder.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// setFakeHome sets HOME and USERPROFILE to t.TempDir() so Refresh writes
// into the test's isolated directory instead of the real user profile.
// Returns the tmpdir. Does NOT pre-create ~/.config/waveterm/ — Refresh
// itself must MkdirAll it (RESEARCH Pitfall 5).
func setFakeHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // Windows: os.UserHomeDir() reads USERPROFILE first
	return tmp
}

// readFixture returns the bytes of a file under pkg/jira/testdata/.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

// newRefreshTestServer returns an httptest.Server that answers /myself,
// /search/jql (single-page), and /issue/{key}. issueFixtures maps issue
// key → JSON body. Unknown keys return 404. Used by most TestRefresh_*
// cases.
func newRefreshTestServer(t *testing.T, issueKeys []string, issueFixtures map[string]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/3/myself":
			fmt.Fprint(w, `{"accountId":"acc-dev-os","displayName":"Dev","emailAddress":"dev@example.com"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/rest/api/3/search/jql":
			_, _ = io.ReadAll(r.Body)
			refs := make([]string, 0, len(issueKeys))
			for i, k := range issueKeys {
				refs = append(refs, fmt.Sprintf(`{"id":"%d","key":"%s"}`, 10000+i, k))
			}
			fmt.Fprintf(w, `{"issues":[%s],"isLast":true,"nextPageToken":""}`, strings.Join(refs, ","))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/rest/api/3/issue/"):
			key := strings.TrimPrefix(r.URL.Path, "/rest/api/3/issue/")
			// Strip query string (fields=...)
			if i := strings.Index(key, "?"); i >= 0 {
				key = key[:i]
			}
			body, ok := issueFixtures[key]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"errorMessages":["not found"]}`)
				return
			}
			fmt.Fprint(w, body)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
}

// baseConfig returns a Config pointed at srv with dev-style synthetic
// values. No real credentials are ever used.
func baseConfig(srv *httptest.Server) Config {
	return Config{
		BaseUrl:  srv.URL,
		CloudId:  "cloud-xxx",
		Email:    "dev@example.com",
		ApiToken: "tok",
		Jql:      "assignee = currentUser()",
		PageSize: 50,
	}
}

// fetchedAtRe matches the fetchedAt JSON field so tests can normalize the
// timestamp before a byte-identical golden-file comparison.
var fetchedAtRe = regexp.MustCompile(`"fetchedAt":\s*"[^"]+"`)

func normalizeFetchedAt(b []byte) []byte {
	return fetchedAtRe.ReplaceAll(b, []byte(`"fetchedAt": "NORMALIZED"`))
}

// runRefreshOneIssue is a small helper: start a test server with a single
// issue fixture and call Refresh. Used by many of the simpler tests.
func runRefreshOneIssue(t *testing.T, issueKey, fixtureFile string) (*RefreshReport, error) {
	t.Helper()
	srv := newRefreshTestServer(t,
		[]string{issueKey},
		map[string]string{issueKey: string(readFixture(t, fixtureFile))},
	)
	t.Cleanup(srv.Close)
	return Refresh(context.Background(), RefreshOpts{
		Config:     baseConfig(srv),
		HTTPClient: srv.Client(),
	})
}

// ---------------------------------------------------------------------
// Test 1: Golden-file byte-identical output (JIRA-07 Success #1, D-TEST-01)
// ---------------------------------------------------------------------

func TestRefresh_GoldenFile(t *testing.T) {
	setFakeHome(t)
	srv := newRefreshTestServer(t,
		[]string{"ITSM-1"},
		map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
	)
	defer srv.Close()

	report, err := Refresh(context.Background(), RefreshOpts{
		Config:     baseConfig(srv),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if report == nil || report.IssueCount != 1 {
		t.Fatalf("report: got %v, want IssueCount=1", report)
	}

	got, err := os.ReadFile(report.CachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	want := readFixture(t, "cache.golden.json")
	want = bytes.ReplaceAll(want, []byte("<<REPLACED-AT-TEST>>"), []byte(srv.URL))

	gotN := normalizeFetchedAt(got)
	wantN := normalizeFetchedAt(want)
	if !bytes.Equal(gotN, wantN) {
		t.Errorf("cache mismatch:\n--- want ---\n%s\n--- got ---\n%s", wantN, gotN)
	}
}

// ---------------------------------------------------------------------
// Test 2: Preserve localPath across refresh (JIRA-07 Success #4, D-TEST-02, D-FLOW-04)
// ---------------------------------------------------------------------

func TestRefresh_PreserveLocalPath(t *testing.T) {
	tmp := setFakeHome(t)
	dir := filepath.Join(tmp, ".config", "waveterm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := readFixture(t, "cache-with-localpath.seed.json")
	if err := os.WriteFile(filepath.Join(dir, "jira-cache.json"), seed, 0o644); err != nil {
		t.Fatal(err)
	}

	srv := newRefreshTestServer(t,
		[]string{"ITSM-1"},
		map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
	)
	defer srv.Close()

	report, err := Refresh(context.Background(), RefreshOpts{
		Config:     baseConfig(srv),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data, err := os.ReadFile(report.CachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal cache: %v", err)
	}
	if len(out.Issues) != 1 || len(out.Issues[0].Attachments) != 1 {
		t.Fatalf("expected 1 issue with 1 attachment, got %d issues", len(out.Issues))
	}
	got := out.Issues[0].Attachments[0].LocalPath
	want := "C:/Users/dev/downloaded/att1-screenshot.png"
	if got != want {
		t.Errorf("LocalPath: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------
// Test 3: Comment cap (10) + body truncation (2000) + kept = LAST 10
// (JIRA-06, D-CACHE-06, D-CACHE-07, D-TEST-03, RESEARCH Pitfall 2)
// ---------------------------------------------------------------------

func TestRefresh_CommentCapAndTruncation(t *testing.T) {
	setFakeHome(t)
	report, err := runRefreshOneIssue(t, "ITSM-CAP-1", "issue-comment-cap.json")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	data, err := os.ReadFile(report.CachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out.Issues))
	}
	iss := out.Issues[0]
	if iss.CommentCount != 15 {
		t.Errorf("CommentCount: got %d, want 15", iss.CommentCount)
	}
	if len(iss.Comments) != 10 {
		t.Fatalf("Comments: got %d, want 10", len(iss.Comments))
	}
	// Last-10 rule: oldest-5 dropped = c01..c05; kept = c06..c15 in ascending order.
	if iss.Comments[0].ID != "c06" {
		t.Errorf("first kept comment id: got %q, want c06", iss.Comments[0].ID)
	}
	if iss.Comments[9].ID != "c15" {
		t.Errorf("last kept comment id: got %q, want c15", iss.Comments[9].ID)
	}
	// c06 has the 2500-char body → truncated to 2000, Truncated=true.
	c06 := iss.Comments[0]
	if len(c06.Body) != 2000 {
		t.Errorf("c06 body len: got %d, want 2000", len(c06.Body))
	}
	if !c06.Truncated {
		t.Errorf("c06 Truncated: got false, want true")
	}
	// c07 has a short body → not truncated, field must be omitted in JSON.
	c07 := iss.Comments[1]
	if c07.Truncated {
		t.Errorf("c07 Truncated: got true, want false")
	}
	// Golden JSON bytes must not contain truncated:true for the short-body comments.
	// Spot-check: exactly one "truncated":true occurrence expected (c06).
	if n := bytes.Count(data, []byte(`"truncated": true`)); n != 1 {
		t.Errorf("truncated:true occurrences in cache: got %d, want 1", n)
	}
}

// ---------------------------------------------------------------------
// Test 4: lastCommentAt = max(updated, created) across kept comments
// (D-CACHE-07)
// ---------------------------------------------------------------------

func TestRefresh_LastCommentAt(t *testing.T) {
	setFakeHome(t)
	report, err := runRefreshOneIssue(t, "ITSM-1", "issue-itsm1.json")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	data, _ := os.ReadFile(report.CachePath)
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out.Issues))
	}
	// c2.updated=2026-04-03T10:30:00 is the max across (c1.created, c1.updated,
	// c2.created, c2.updated); see testdata/issue-itsm1.json.
	want := "2026-04-03T10:30:00.000+0000"
	if out.Issues[0].LastCommentAt != want {
		t.Errorf("LastCommentAt: got %q, want %q", out.Issues[0].LastCommentAt, want)
	}
}

// ---------------------------------------------------------------------
// Test 5: description: null → "" (D-CACHE-04)
// ---------------------------------------------------------------------

func TestRefresh_NullDescription(t *testing.T) {
	setFakeHome(t)
	report, err := runRefreshOneIssue(t, "ITSM-NULL-1", "issue-null-description.json")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	data, _ := os.ReadFile(report.CachePath)
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(out.Issues))
	}
	if out.Issues[0].Description != "" {
		t.Errorf("Description: got %q, want empty string", out.Issues[0].Description)
	}
}

// ---------------------------------------------------------------------
// Test 6: statusCategory.key mapping (new/indeterminate/done + unknown → new)
// (D-CACHE-08)
// ---------------------------------------------------------------------

func TestRefresh_StatusCategoryMapping(t *testing.T) {
	cases := []struct {
		name       string
		wireKey    string // value to put at fields.status.statusCategory.key; empty means omit
		wantCache  string
	}{
		{"new", "new", "new"},
		{"indeterminate", "indeterminate", "indeterminate"},
		{"done", "done", "done"},
		{"undefined", "undefined", "new"}, // unknown → new (D-CACHE-08)
		{"missing", "", "new"},            // no statusCategory at all → new
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setFakeHome(t)

			var status string
			if tc.wireKey == "" {
				status = `{"name":"X","id":"1"}`
			} else {
				status = fmt.Sprintf(`{"name":"X","id":"1","statusCategory":{"key":%q}}`, tc.wireKey)
			}
			fixture := fmt.Sprintf(`{
				"id":"20000","key":"ITSM-SC-1",
				"fields":{
					"summary":"s","description":null,
					"status":%s,
					"issuetype":{"name":"Task","id":"3","subtask":false},
					"priority":{"name":"Low","id":"4"},
					"project":{"key":"ITSM","name":"IT Service Management","id":"100"},
					"created":"2026-03-01T00:00:00.000+0000",
					"updated":"2026-04-15T00:00:00.000+0000",
					"attachment":[],
					"comment":{"total":0,"maxResults":0,"startAt":0,"comments":[]}
				}
			}`, status)

			srv := newRefreshTestServer(t,
				[]string{"ITSM-SC-1"},
				map[string]string{"ITSM-SC-1": fixture},
			)
			defer srv.Close()

			report, err := Refresh(context.Background(), RefreshOpts{
				Config:     baseConfig(srv),
				HTTPClient: srv.Client(),
			})
			if err != nil {
				t.Fatalf("refresh: %v", err)
			}
			data, _ := os.ReadFile(report.CachePath)
			var out JiraCache
			if err := json.Unmarshal(data, &out); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if len(out.Issues) != 1 {
				t.Fatalf("expected 1 issue, got %d", len(out.Issues))
			}
			if out.Issues[0].StatusCategory != tc.wantCache {
				t.Errorf("StatusCategory: got %q, want %q", out.Issues[0].StatusCategory, tc.wantCache)
			}
		})
	}
}

// ---------------------------------------------------------------------
// Test 7: Error classification (D-ERR-01..03)
//   (a) /myself 401  → fatal error, no cache file written
//   (b) /search 500  → fatal error, no cache file written
//   (c) one /issue/K 404, others succeed → nil error, failed issue omitted
// ---------------------------------------------------------------------

func TestRefresh_ErrorClassification(t *testing.T) {
	t.Run("MyselfUnauthorizedIsFatal", func(t *testing.T) {
		tmp := setFakeHome(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/rest/api/3/myself" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"errorMessages":["unauth"]}`)
				return
			}
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := Refresh(context.Background(), RefreshOpts{
			Config:     baseConfig(srv),
			HTTPClient: srv.Client(),
		})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		// Cache file must NOT exist (no partial writes on fatal error).
		if _, statErr := os.Stat(filepath.Join(tmp, ".config", "waveterm", "jira-cache.json")); !os.IsNotExist(statErr) {
			t.Errorf("cache file should not exist after fatal error, stat err: %v", statErr)
		}
	})

	t.Run("SearchFailureIsFatal", func(t *testing.T) {
		tmp := setFakeHome(t)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/rest/api/3/myself" {
				fmt.Fprint(w, `{"accountId":"acc-dev-os","displayName":"Dev","emailAddress":"dev@example.com"}`)
				return
			}
			if r.URL.Path == "/rest/api/3/search/jql" {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, `{"errorMessages":["boom"]}`)
				return
			}
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := Refresh(context.Background(), RefreshOpts{
			Config:     baseConfig(srv),
			HTTPClient: srv.Client(),
		})
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if _, statErr := os.Stat(filepath.Join(tmp, ".config", "waveterm", "jira-cache.json")); !os.IsNotExist(statErr) {
			t.Errorf("cache file should not exist after fatal search error, stat err: %v", statErr)
		}
	})

	t.Run("PerIssueFailureIsSkipped", func(t *testing.T) {
		setFakeHome(t)
		// Two issues listed in search; only ITSM-1 has a fixture. ITSM-MISSING → 404.
		srv := newRefreshTestServer(t,
			[]string{"ITSM-1", "ITSM-MISSING"},
			map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
		)
		defer srv.Close()

		report, err := Refresh(context.Background(), RefreshOpts{
			Config:     baseConfig(srv),
			HTTPClient: srv.Client(),
		})
		if err != nil {
			t.Fatalf("refresh: unexpected error: %v", err)
		}
		if report == nil || report.IssueCount != 1 {
			t.Fatalf("IssueCount: got %v, want 1 (one fixture missing)", report)
		}
		data, _ := os.ReadFile(report.CachePath)
		var out JiraCache
		if err := json.Unmarshal(data, &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out.Issues) != 1 {
			t.Fatalf("expected 1 issue in cache, got %d", len(out.Issues))
		}
		if out.Issues[0].Key != "ITSM-1" {
			t.Errorf("surviving issue: got %q, want ITSM-1", out.Issues[0].Key)
		}
	})
}

// ---------------------------------------------------------------------
// Test 8: Progress callback invocation sequence (D-PROG-01)
// ---------------------------------------------------------------------

func TestRefresh_ProgressCallback(t *testing.T) {
	setFakeHome(t)
	srv := newRefreshTestServer(t,
		[]string{"ITSM-1"},
		map[string]string{"ITSM-1": string(readFixture(t, "issue-itsm1.json"))},
	)
	defer srv.Close()

	type rec struct {
		stage            string
		current, total   int
	}
	var calls []rec
	_, err := Refresh(context.Background(), RefreshOpts{
		Config:     baseConfig(srv),
		HTTPClient: srv.Client(),
		OnProgress: func(stage string, current, total int) {
			calls = append(calls, rec{stage, current, total})
		},
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(calls) == 0 {
		t.Fatal("OnProgress was never invoked")
	}
	has := func(stage string, cur, tot int) bool {
		for _, c := range calls {
			if c.stage == stage && c.current == cur && c.total == tot {
				return true
			}
		}
		return false
	}
	if !has("fetch", 1, 1) {
		t.Errorf("missing ('fetch', 1, 1) in %v", calls)
	}
	if !has("write", 1, 1) {
		t.Errorf("missing ('write', 1, 1) in %v", calls)
	}
	// "search" stage must appear at least once; total is 0 because the
	// token-based search API does not return a total (RESEARCH Pitfall 6).
	sawSearch := false
	for _, c := range calls {
		if c.stage == "search" {
			sawSearch = true
			break
		}
	}
	if !sawSearch {
		t.Errorf("missing 'search' stage in %v", calls)
	}
}

// ---------------------------------------------------------------------
// Test 9: Attachment webUrl passes through wire content field unchanged
// (D-CACHE-05, RESEARCH A3)
// ---------------------------------------------------------------------

func TestRefresh_AttachmentWebUrlPassthrough(t *testing.T) {
	setFakeHome(t)
	report, err := runRefreshOneIssue(t, "ITSM-1", "issue-itsm1.json")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	data, _ := os.ReadFile(report.CachePath)
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Issues) != 1 || len(out.Issues[0].Attachments) != 1 {
		t.Fatalf("expected 1 issue with 1 attachment, got %d issues", len(out.Issues))
	}
	got := out.Issues[0].Attachments[0].WebUrl
	want := "https://example.atlassian.net/rest/api/3/attachment/content/att1"
	if got != want {
		t.Errorf("WebUrl: got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------
// Test 10: Multi-byte UTF-8 comment body truncates on a rune boundary
// (WR-01 regression guard — Korean/CJK users)
//
// Build a body from "한" (U+D55C, 3 bytes in UTF-8). 700 runes × 3 bytes =
// 2100 bytes, which is > the 2000-byte cap. A naive byte-slice at 2000
// would land in the middle of the 667th rune (2000 / 3 = 666 remainder 2)
// and produce invalid UTF-8. The correct behavior is to walk back to the
// rune boundary at byte 1998, yielding 666 complete runes.
// ---------------------------------------------------------------------

func TestRefresh_CommentBodyTruncationIsUTF8Safe(t *testing.T) {
	setFakeHome(t)

	// 700 copies of "한" → 2100 bytes.
	bigBody := strings.Repeat("한", 700)
	if len(bigBody) != 2100 {
		t.Fatalf("fixture setup: expected 2100 bytes, got %d", len(bigBody))
	}

	fixture := fmt.Sprintf(`{
		"id":"30000","key":"ITSM-UTF8-1",
		"fields":{
			"summary":"utf8 truncation","description":null,
			"status":{"name":"Open","id":"1","statusCategory":{"key":"new"}},
			"issuetype":{"name":"Task","id":"3","subtask":false},
			"priority":{"name":"Medium","id":"3"},
			"project":{"key":"ITSM","name":"IT Service Management","id":"100"},
			"created":"2026-03-01T00:00:00.000+0000",
			"updated":"2026-04-15T00:00:00.000+0000",
			"attachment":[],
			"comment":{"total":1,"maxResults":1,"startAt":0,"comments":[
				{"id":"cutf8","author":{"accountId":"acc-u1","displayName":"User 1"},
				 "body":{"type":"doc","version":1,"content":[{"type":"paragraph","content":[
					{"type":"text","text":%q}
				 ]}]},
				 "created":"2026-03-01T00:00:00.000+0000",
				 "updated":"2026-03-01T00:00:00.000+0000"}
			]}
		}
	}`, bigBody)

	srv := newRefreshTestServer(t,
		[]string{"ITSM-UTF8-1"},
		map[string]string{"ITSM-UTF8-1": fixture},
	)
	defer srv.Close()

	report, err := Refresh(context.Background(), RefreshOpts{
		Config:     baseConfig(srv),
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	data, err := os.ReadFile(report.CachePath)
	if err != nil {
		t.Fatalf("read cache: %v", err)
	}
	var out JiraCache
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Issues) != 1 || len(out.Issues[0].Comments) != 1 {
		t.Fatalf("expected 1 issue with 1 comment, got %d issues", len(out.Issues))
	}
	got := out.Issues[0].Comments[0].Body
	if !out.Issues[0].Comments[0].Truncated {
		t.Errorf("Truncated: got false, want true")
	}
	// Must be valid UTF-8 after truncation (core WR-01 invariant).
	if !utf8.ValidString(got) {
		t.Errorf("truncated body is not valid UTF-8: %q", got)
	}
	// Cut should be at byte 1998 (666 runes × 3 bytes), NOT 2000.
	// - 1998 % 3 == 0  → rune-aligned
	// - len(got) must be <= 2000 (the cap) but > 1997 (we didn't over-walk).
	if len(got) != 1998 {
		t.Errorf("truncated body length: got %d bytes, want 1998 (666 × 3-byte runes)", len(got))
	}
	// Every rune in the output must be "한" — nothing corrupted.
	if runeCount := utf8.RuneCountInString(got); runeCount != 666 {
		t.Errorf("rune count: got %d, want 666", runeCount)
	}
	// The raw cache JSON must also be valid UTF-8 (json.Marshal would otherwise
	// escape invalid bytes, but we want to see no escapes for valid Korean text).
	if !utf8.Valid(data) {
		t.Errorf("cache file bytes are not valid UTF-8")
	}
	// Confirm the JSON does not contain the Unicode replacement character U+FFFD.
	if bytes.Contains(data, []byte("\ufffd")) {
		t.Errorf("cache contains U+FFFD replacement character — truncation split a rune")
	}
}

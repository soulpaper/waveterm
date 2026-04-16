// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestClient returns a Client pointed at srv.URL using srv.Client().
// All HTTP tests funnel through this helper so auth/header assertions are
// consistent.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	cfg := Config{
		BaseUrl:  srv.URL,
		CloudId:  "test-cloud-id",
		Email:    "user@example.com",
		ApiToken: "test-token",
		Jql:      "assignee = currentUser()",
		PageSize: 50,
	}
	return NewClientWithHTTP(cfg, srv.Client())
}

func TestAuthHeader_ExactBasicBase64(t *testing.T) {
	// D-06: Authorization: Basic base64("user@example.com:test-token")
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("user@example.com:test-token"))
	var gotAuth, gotAccept, gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"issues":[],"isLast":true,"nextPageToken":""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != expected {
		t.Errorf("Authorization header: got %q want %q", gotAuth, expected)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept header: got %q", gotAccept)
	}
	if !strings.HasPrefix(gotUA, "waveterm-jira/") {
		t.Errorf("User-Agent: got %q want prefix waveterm-jira/", gotUA)
	}
}

func TestSearchIssues_Pagination(t *testing.T) {
	// First call: no token → server returns nextPageToken=tok2, isLast=false.
	// Second call: token=tok2 → server returns isLast=true.
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("want POST, got %s", r.Method)
		}
		if r.URL.Path != "/rest/api/3/search/jql" {
			t.Errorf("path: got %q", r.URL.Path)
		}
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		if calls == 1 {
			fmt.Fprint(w, `{"issues":[{"id":"1","key":"ITSM-1"}],"isLast":false,"nextPageToken":"tok2"}`)
		} else {
			fmt.Fprint(w, `{"issues":[{"id":"2","key":"ITSM-2"}],"isLast":true,"nextPageToken":""}`)
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	page1, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X"})
	if err != nil {
		t.Fatalf("page1 error: %v", err)
	}
	if page1.IsLast {
		t.Error("page1.IsLast should be false")
	}
	if page1.NextPageToken != "tok2" {
		t.Errorf("page1.NextPageToken: got %q want tok2", page1.NextPageToken)
	}
	if len(page1.Issues) != 1 || page1.Issues[0].Key != "ITSM-1" {
		t.Errorf("page1 issues wrong: %+v", page1.Issues)
	}

	page2, err := c.SearchIssues(context.Background(), SearchOpts{JQL: "project=X", NextPageToken: "tok2"})
	if err != nil {
		t.Fatalf("page2 error: %v", err)
	}
	if !page2.IsLast {
		t.Error("page2.IsLast should be true")
	}
	if len(page2.Issues) != 1 || page2.Issues[0].Key != "ITSM-2" {
		t.Errorf("page2 issues wrong: %+v", page2.Issues)
	}
}

func TestSearchIssues_RequestBodyShape(t *testing.T) {
	// D-01: nextPageToken is in the request BODY, not query string.
	// D-03: MaxResults=0 → defaults to 50.
	var bodyStr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		bodyStr = string(buf[:n])
		w.WriteHeader(200)
		fmt.Fprint(w, `{"issues":[],"isLast":true,"nextPageToken":""}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.SearchIssues(context.Background(), SearchOpts{
		JQL:           "project = ITSM",
		NextPageToken: "abc123",
		Fields:        []string{"summary", "status"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		`"jql"`, `"project = ITSM"`,
		`"nextPageToken"`, `"abc123"`,
		`"fields"`, `"summary"`, `"status"`,
		`"maxResults"`, `50`, // default applied
	} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("request body missing %q; full body: %s", want, bodyStr)
		}
	}
	// Must NOT send nextPageToken as a URL query param.
	if strings.Contains(bodyStr, "?nextPageToken=") {
		t.Error("nextPageToken must be in body, not URL")
	}
}

func TestGetIssue_FieldsAsCSV(t *testing.T) {
	// D-02: GET /rest/api/3/issue/{key}?fields=a,b,c (comma-joined)
	var gotQuery string
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query().Get("fields")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"10001","key":"ITSM-3135","fields":{"summary":"Test"}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	iss, err := c.GetIssue(context.Background(), "ITSM-3135", GetIssueOpts{
		Fields: []string{"summary", "status", "comment"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/rest/api/3/issue/ITSM-3135" {
		t.Errorf("path: got %q", gotPath)
	}
	if gotQuery != "summary,status,comment" {
		t.Errorf("fields query: got %q want summary,status,comment", gotQuery)
	}
	if iss.Key != "ITSM-3135" {
		t.Errorf("Key: got %q", iss.Key)
	}
	if iss.Fields.Summary != "Test" {
		t.Errorf("Summary: got %q", iss.Fields.Summary)
	}
}

func TestGetIssue_NilFieldsOmitsQueryParam(t *testing.T) {
	// When opts.Fields is nil/empty, do NOT send ?fields= (let Jira return defaults).
	var rawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rawQuery = r.URL.RawQuery
		w.WriteHeader(200)
		fmt.Fprint(w, `{"id":"1","key":"K","fields":{}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	_, err := c.GetIssue(context.Background(), "K", GetIssueOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(rawQuery, "fields=") {
		t.Errorf("expected no fields= param, got query %q", rawQuery)
	}
}

func TestErrorPaths(t *testing.T) {
	cases := []struct {
		name           string
		status         int
		body           string
		retryAfter     string
		wantSentinel   error
		wantRetryAfter time.Duration
	}{
		{"401 unauthorized", 401, `{"errorMessages":["bad token"]}`, "", ErrUnauthorized, 0},
		{"403 forbidden", 403, `{"errorMessages":["forbidden"]}`, "", ErrForbidden, 0},
		{"404 not found", 404, `{"errorMessages":["no issue"]}`, "", ErrNotFound, 0},
		{"429 rate limited with Retry-After", 429, `{"errorMessages":["slow down"]}`, "7", ErrRateLimited, 7 * time.Second},
		{"500 server error", 500, `internal error`, "", ErrServerError, 0},
		{"503 server error", 503, `gateway busy`, "", ErrServerError, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.retryAfter != "" {
					w.Header().Set("Retry-After", tc.retryAfter)
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := newTestClient(t, srv)
			_, err := c.GetIssue(context.Background(), "KEY-1", GetIssueOpts{})
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.wantSentinel) {
				t.Errorf("errors.Is failed: err=%v want sentinel=%v", err, tc.wantSentinel)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("errors.As(*APIError) failed for %v", err)
			}
			if apiErr.StatusCode != tc.status {
				t.Errorf("StatusCode: got %d want %d", apiErr.StatusCode, tc.status)
			}
			if apiErr.RetryAfter != tc.wantRetryAfter {
				t.Errorf("RetryAfter: got %v want %v", apiErr.RetryAfter, tc.wantRetryAfter)
			}
		})
	}
}

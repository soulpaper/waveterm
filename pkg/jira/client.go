// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

// defaultTimeout is the http.Client timeout for a single Jira request (D-05).
// Jira's P99 for /search is well under 5s; 30s is generous headroom without
// hanging user-visible flows.
const defaultTimeout = 30 * time.Second

// errorBodyLimit caps how much of a non-2xx response body we retain in
// APIError.Body (per D-18). 1 KB is enough for a stack trace or JSON error
// envelope without letting a hostile server balloon memory.
const errorBodyLimit = 1024

// Client is a stateless-ish Jira Cloud REST v3 client. It holds a Config
// snapshot captured at construction time AND a private http.Client. No
// global singleton, no init() side effects — per CONTEXT §Code Context.
type Client struct {
	cfg Config
	hc  *http.Client
}

// NewClient constructs a Client with a 30s-timeout http.Client. This is the
// normal production entry point. Callers that need a custom http.Client
// (tests, or future phases that inject a rate-limiting Transport) use
// NewClientWithHTTP.
func NewClient(cfg Config) *Client {
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: defaultTimeout},
	}
}

// NewClientWithHTTP is the test seam (D-05). client_test.go passes
// httptest.NewServer().Client() here so that the test server's self-signed
// certificate is accepted.
func NewClientWithHTTP(cfg Config, hc *http.Client) *Client {
	return &Client{cfg: cfg, hc: hc}
}

// SearchOpts is the input to SearchIssues (D-04 options-struct style).
type SearchOpts struct {
	JQL           string   // required
	NextPageToken string   // "" for first page
	Fields        []string // nil/empty → server default; else sent as JSON array
	MaxResults    int      // 0 → DefaultPageSize (50 per D-03)
}

// GetIssueOpts is the input to GetIssue.
type GetIssueOpts struct {
	Fields []string // nil/empty → server default (fields query param omitted)
}

// SearchResult is the response from POST /rest/api/3/search/jql. Per
// RESEARCH Pitfall 2, Atlassian's enhanced search response has NO total
// count — progress tracking is caller's job via fetched-so-far counting.
type SearchResult struct {
	Issues        []IssueRef `json:"issues"`
	NextPageToken string     `json:"nextPageToken"`
	IsLast        bool       `json:"isLast"`
}

// IssueRef is the shallow issue shape returned by search. Fields populated
// depend on what the caller requested via SearchOpts.Fields.
type IssueRef struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

// Issue is the full issue shape returned by GetIssue.
type Issue struct {
	ID     string      `json:"id"`
	Key    string      `json:"key"`
	Self   string      `json:"self"`
	Fields IssueFields `json:"fields"`
}

// IssueFields is a best-effort decode of the Jira "fields" object. Fields
// not requested (or not present on the issue type) decode as zero values.
// Description and Comment.Body are kept as json.RawMessage so the caller can
// feed them into ADFToMarkdown when they need markdown output.
type IssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
	Status      struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"status"`
	IssueType struct {
		Name    string `json:"name"`
		ID      string `json:"id"`
		Subtask bool   `json:"subtask"`
	} `json:"issuetype"`
	Priority struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"priority"`
	Project struct {
		Key  string `json:"key"`
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"project"`
	Created    string       `json:"created"`
	Updated    string       `json:"updated"`
	Attachment []Attachment `json:"attachment"`
	Comment    CommentPage  `json:"comment"`
}

// Attachment is an element of IssueFields.Attachment. Content is the direct
// download URL; Phase 5 will GET it with auth to stream to disk.
type Attachment struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	MimeType  string `json:"mimeType"`
	Size      int64  `json:"size"`
	Created   string `json:"created"`
	Content   string `json:"content"`
	Thumbnail string `json:"thumbnail"`
	Author    struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
}

// CommentPage is the wrapper object Jira returns in fields.comment. Per
// RESEARCH Pitfall 3 the comment field is an OBJECT (with total + comments),
// NOT a bare array. Unmarshaling into []Comment would fail decode.
type CommentPage struct {
	Total    int       `json:"total"`
	Comments []Comment `json:"comments"`
}

// Comment is a single issue comment. Body is ADF (json.RawMessage) so
// callers can pass it to ADFToMarkdown.
type Comment struct {
	ID     string `json:"id"`
	Author struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Body    json.RawMessage `json:"body"`
	Created string          `json:"created"`
	Updated string          `json:"updated"`
}

// searchRequest is the JSON body for POST /rest/api/3/search/jql. Kept
// private; SearchOpts is the caller-facing shape.
type searchRequest struct {
	JQL           string   `json:"jql"`
	MaxResults    int      `json:"maxResults"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
	Fields        []string `json:"fields,omitempty"`
}

// SearchIssues calls POST /rest/api/3/search/jql with the caller's JQL and
// pagination cursor. Per D-01/D-03/RESEARCH Pitfall 1, nextPageToken is in
// the BODY (not a query string) and MaxResults defaults to 50 when zero.
func (c *Client) SearchIssues(ctx context.Context, opts SearchOpts) (*SearchResult, error) {
	maxResults := opts.MaxResults
	if maxResults == 0 {
		maxResults = DefaultPageSize
	}
	reqBody := searchRequest{
		JQL:           opts.JQL,
		MaxResults:    maxResults,
		NextPageToken: opts.NextPageToken,
		Fields:        opts.Fields,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("jira: marshal search request: %w", err)
	}

	endpoint := c.cfg.BaseUrl + "/rest/api/3/search/jql"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("jira: build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	c.setCommonHeaders(req)

	var result SearchResult
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetIssue calls GET /rest/api/3/issue/{key}. Per D-02 the fields param is
// a single comma-joined query value, and if opts.Fields is nil/empty we
// OMIT the param entirely (Jira returns its default field set).
func (c *Client) GetIssue(ctx context.Context, key string, opts GetIssueOpts) (*Issue, error) {
	if key == "" {
		return nil, fmt.Errorf("jira: issue key is required")
	}
	// PathEscape the key — keys are ASCII (ITSM-3135) in practice but be
	// defensive against future custom project prefixes.
	endpoint := c.cfg.BaseUrl + "/rest/api/3/issue/" + url.PathEscape(key)
	if len(opts.Fields) > 0 {
		// Comma-joined single param per RESEARCH §GET Issue Request and
		// §Open Question 1 (dominant Jira client library convention).
		endpoint += "?fields=" + url.QueryEscape(strings.Join(opts.Fields, ","))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("jira: build getissue request: %w", err)
	}
	c.setCommonHeaders(req)

	var issue Issue
	if err := c.doJSON(req, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// setCommonHeaders applies Authorization, Accept, and User-Agent to every
// request (D-06). Content-Type is set by the caller when there is a body.
func (c *Client) setCommonHeaders(req *http.Request) {
	// Basic auth: base64(email + ":" + apiToken) per Atlassian docs.
	raw := c.cfg.Email + ":" + c.cfg.ApiToken
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(raw)))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "waveterm-jira/"+wavebase.WaveVersion)
}

// doJSON executes req, categorizes non-2xx into *APIError, decodes the
// success body into out. Body is always closed.
func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.hc.Do(req)
	if err != nil {
		return fmt.Errorf("jira: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, errorBodyLimit))
		apiErr := &APIError{
			StatusCode: resp.StatusCode,
			Endpoint:   req.URL.Path,
			Method:     req.Method,
			Body:       string(body),
		}
		if resp.StatusCode == 429 {
			apiErr.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
		}
		return apiErr
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("jira: decode response: %w", err)
	}
	return nil
}

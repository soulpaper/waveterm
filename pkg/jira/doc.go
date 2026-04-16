// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package jira provides a Go client for Atlassian Jira Cloud REST API v3.
// It authenticates via HTTP Basic (email + API token), supports cursor-based
// search (POST /rest/api/3/search/jql), single-issue retrieval, and a minimal
// ADF → markdown converter for issue descriptions and comment bodies.
//
// The package is stdlib-only and has no init() side effects. Construct via
// NewClient(cfg) or NewClientWithHTTP(cfg, hc) for tests.
package jira

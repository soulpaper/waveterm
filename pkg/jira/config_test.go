// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// writeConfigFile is a helper used across config tests. It writes `contents`
// to a file named jira.json inside t.TempDir() and returns the full path.
func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "jira.json")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfig_Happy(t *testing.T) {
	path := writeConfigFile(t, `{
		"baseUrl":  "https://kakaovx.atlassian.net",
		"cloudId":  "280eeb13-4c6a-4dc3-aec5-c5f9385c7a7d",
		"email":    "spike@kakaovx.com",
		"apiToken": "ATATT-xxx",
		"jql":      "project = ITSM",
		"pageSize": 25
	}`)
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.BaseUrl != "https://kakaovx.atlassian.net" {
		t.Errorf("BaseUrl: got %q", cfg.BaseUrl)
	}
	if cfg.Email != "spike@kakaovx.com" {
		t.Errorf("Email: got %q", cfg.Email)
	}
	if cfg.ApiToken != "ATATT-xxx" {
		t.Errorf("ApiToken: got %q", cfg.ApiToken)
	}
	if cfg.Jql != "project = ITSM" {
		t.Errorf("Jql: got %q", cfg.Jql)
	}
	if cfg.PageSize != 25 {
		t.Errorf("PageSize: got %d want 25", cfg.PageSize)
	}
}

func TestLoadConfig_DefaultsFill(t *testing.T) {
	// Omit jql and pageSize; loader must fill them per D-03.
	path := writeConfigFile(t, `{
		"baseUrl":  "https://kakaovx.atlassian.net",
		"email":    "spike@kakaovx.com",
		"apiToken": "ATATT-xxx"
	}`)
	cfg, err := LoadConfigFromPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Jql != "assignee = currentUser() ORDER BY updated DESC" {
		t.Errorf("Jql default: got %q", cfg.Jql)
	}
	if cfg.PageSize != 50 {
		t.Errorf("PageSize default: got %d want 50", cfg.PageSize)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	// Path points into a temp dir but the file is never created.
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("want ErrConfigNotFound, got %v", err)
	}
}

func TestLoadConfig_MalformedJSON(t *testing.T) {
	path := writeConfigFile(t, `{this is not valid json`)
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigInvalid) {
		t.Fatalf("want ErrConfigInvalid, got %v", err)
	}
}

func TestLoadConfig_Incomplete(t *testing.T) {
	// Missing baseUrl, email, apiToken → must name all three in error message.
	path := writeConfigFile(t, `{"cloudId":"abc"}`)
	_, err := LoadConfigFromPath(path)
	if !errors.Is(err, ErrConfigIncomplete) {
		t.Fatalf("want ErrConfigIncomplete, got %v", err)
	}
	msg := err.Error()
	for _, field := range []string{"baseUrl", "email", "apiToken"} {
		if !stringsContains(msg, field) {
			t.Errorf("error %q does not mention missing field %q", msg, field)
		}
	}
}

// stringsContains is a local helper to avoid importing "strings" inside tests
// that already do enough with stdlib. Replace with strings.Contains if preferred
// during implementation.
func stringsContains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

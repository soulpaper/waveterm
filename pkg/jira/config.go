// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is the on-disk representation of ~/.config/waveterm/jira.json.
// Per D-07 the path is literal — we do NOT route through the Wave config-dir
// helper because that resolves to a platform-specific directory (e.g.
// %LOCALAPPDATA%/waveterm on Windows) which is NOT the contract the existing
// Jira widget and jira-cache.json already use.
type Config struct {
	BaseUrl  string `json:"baseUrl"`  // required, e.g. "https://kakaovx.atlassian.net"
	CloudId  string `json:"cloudId"`  // required for cache output, e.g. "280eeb13-..."
	Email    string `json:"email"`    // required, Basic auth username
	ApiToken string `json:"apiToken"` // required, Basic auth password
	Jql      string `json:"jql"`      // optional, default = "assignee = currentUser() ORDER BY updated DESC"
	PageSize int    `json:"pageSize"` // optional, default = 50
}

// Default values applied by LoadConfig / LoadConfigFromPath when fields are
// missing or zero. Keep these as exported constants so Phase 2's refresh
// orchestrator can reference the same literals.
const (
	DefaultJQL      = "assignee = currentUser() ORDER BY updated DESC"
	DefaultPageSize = 50
)

// configFileName is the literal filename per D-07. The directory components
// (".config", "waveterm") are inlined into filepath.Join so the path is
// assembled with platform-correct separators on every call.
const configFileName = "jira.json"

// LoadConfig reads ~/.config/waveterm/jira.json, fills defaults, validates
// required fields. Per D-12 this is called on every refresh (no in-process
// cache) so edits to jira.json take effect without a Waveterm restart.
//
// Errors:
//   - ErrConfigNotFound  : file does not exist
//   - ErrConfigInvalid   : file exists but JSON is malformed (wraps json error)
//   - ErrConfigIncomplete: required field (baseUrl/email/apiToken) missing
func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("jira: cannot resolve home directory: %w", err)
	}
	// filepath.Join normalizes separators per platform (D-23 Windows-safe).
	path := filepath.Join(home, ".config", "waveterm", configFileName)
	return LoadConfigFromPath(path)
}

// LoadConfigFromPath is the test seam used by config_test.go. It performs all
// the actual work; LoadConfig is a thin wrapper that resolves the home path.
//
// Threat T-01-02-02: this function MUST NOT log file contents or any field of
// the decoded Config — the file holds the raw API token. Errors are returned
// to the caller; logging policy belongs to the caller.
func LoadConfigFromPath(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return Config{}, ErrConfigNotFound
		}
		// Other read failures (permission denied, etc.) are treated as
		// "not found" from the caller's perspective — Phase 4's empty-state
		// UX handles both the same way.
		return Config{}, fmt.Errorf("%w: %v", ErrConfigNotFound, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Wrap the json error via %w so tooling (errors.As) can inspect it,
		// while errors.Is(err, ErrConfigInvalid) remains true via the %w chain.
		return Config{}, fmt.Errorf("%w: %w", ErrConfigInvalid, err)
	}

	// Fill defaults (D-03 / D-08). Silent — no warning, no error.
	if cfg.Jql == "" {
		cfg.Jql = DefaultJQL
	}
	if cfg.PageSize == 0 {
		cfg.PageSize = DefaultPageSize
	}

	// Validate required fields (D-11). The error message NAMES each missing
	// field so the user knows exactly what to fix in jira.json.
	var missing []string
	if cfg.BaseUrl == "" {
		missing = append(missing, "baseUrl")
	}
	if cfg.Email == "" {
		missing = append(missing, "email")
	}
	if cfg.ApiToken == "" {
		missing = append(missing, "apiToken")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("%w: missing required fields: %s",
			ErrConfigIncomplete, strings.Join(missing, ", "))
	}
	return cfg, nil
}

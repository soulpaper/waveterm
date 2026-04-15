// Copyright 2026, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// wshcmd-jira.go — `wsh jira` parent command + `refresh` subcommand.
//
// Exit-code handling note: this repo has no `exitCodeError` sentinel pattern
// (verified via grep of cmd/wsh for `exitCodeError` / `os.Exit(`). The
// established convention is to set the package-level `WshExitCode` variable
// (see cmd/wsh/cmd/wshcmd-getvar.go) and return nil; cmd/wsh/cmd/wshcmd-root.go
// Execute() forwards that value to wshutil.DoShutdown. We follow that
// convention instead of calling os.Exit directly.

package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wshrpc/wshclient"
	"github.com/wavetermdev/waveterm/pkg/wshutil"
)

var jiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Interact with Jira (PAT-authenticated)",
	Long:  "Jira commands. Configure ~/.config/waveterm/jira.json first. See README.",
}

var jiraRefreshCmd = &cobra.Command{
	Use:     "refresh",
	Short:   "Fetch latest Jira issues into the cache",
	RunE:    jiraRefreshRun,
	PreRunE: preRunSetupRpcClient,
}

var (
	jiraRefreshJSON    bool
	jiraRefreshTimeout int
)

func init() {
	rootCmd.AddCommand(jiraCmd)
	jiraCmd.AddCommand(jiraRefreshCmd)
	jiraRefreshCmd.Flags().BoolVar(&jiraRefreshJSON, "json", false, "emit JSON instead of human-readable summary")
	jiraRefreshCmd.Flags().IntVar(&jiraRefreshTimeout, "timeout", 60, "RPC timeout in seconds")
}

// jiraRefreshRun invokes wshclient.JiraRefreshCommand and prints either a
// human-readable summary or JSON. On RPC error it prints the error to stderr
// and sets WshExitCode per D-ERR-04.
func jiraRefreshRun(cmd *cobra.Command, args []string) (rtnErr error) {
	defer func() {
		sendActivity("jira-refresh", rtnErr == nil && WshExitCode == 0)
	}()

	opts := &wshrpc.RpcOpts{Timeout: int64(jiraRefreshTimeout) * 1000}
	// Route: when run inside a Wave tab, use the tab route so the handler sees
	// the caller's context. When run from a standalone terminal (no
	// WAVETERM_TABID), leave Route empty — the default wavesrv route handles
	// the refresh since it doesn't need tab state.
	if tabId := os.Getenv("WAVETERM_TABID"); tabId != "" {
		opts.Route = wshutil.MakeTabRouteId(tabId)
	}

	rtn, err := wshclient.JiraRefreshCommand(RpcClient, wshrpc.CommandJiraRefreshData{}, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		WshExitCode = exitCodeForError(err)
		return nil
	}

	if jiraRefreshJSON {
		out, marshalErr := json.MarshalIndent(rtn, "", "  ")
		if marshalErr != nil {
			return fmt.Errorf("marshal rtn: %w", marshalErr)
		}
		fmt.Println(string(out))
		return nil
	}

	fmt.Println(formatRefreshSummary(rtn))
	return nil
}

// exitCodeForError maps a JiraRefreshCommand RPC error to a shell exit code
// per D-ERR-04. The Korean prefixes are emitted verbatim by Plan 01's
// mapJiraError (pkg/wshrpc/wshserver/wshserver-jira.go) so prefix-matching is
// the contract. Unknown errors fall through to 3.
func exitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "인증 실패"):
		return 1
	case strings.HasPrefix(msg, "설정 파일이 없습니다"):
		return 2
	default:
		return 3
	}
}

// formatRefreshSummary renders the human-readable success line per D-CLI-02.
// Elapsed is printed with one decimal second; counts are integers. The arrow
// character (U+2192) is literal.
func formatRefreshSummary(rtn wshrpc.CommandJiraRefreshRtnData) string {
	elapsed := time.Duration(rtn.ElapsedMs) * time.Millisecond
	return fmt.Sprintf("Fetched %d issues (%d attachments, %d comments) in %.1fs → %s",
		rtn.IssueCount, rtn.AttachmentCount, rtn.CommentCount,
		elapsed.Seconds(), rtn.CachePath)
}

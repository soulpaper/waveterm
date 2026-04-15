# Requirements — Milestone v1.0 Jira Team Integration

## Active Requirements

### Jira Backend (Waveterm-native)

- [ ] **JIRA-01**: User can issue an HTTP request to Atlassian Jira Cloud from the Waveterm Go backend, authenticated with an Atlassian API token (PAT + email via HTTP Basic).
- [ ] **JIRA-02**: User can configure Jira credentials (baseUrl, cloudId, email, apiToken, JQL) via a plain JSON file at `~/.config/waveterm/jira.json`. Config is read on every refresh (no restart needed after edit).
- [ ] **JIRA-06**: User gets comments fetched for each issue, with the latest 10 kept and each body truncated to 2000 chars. `commentCount` reflects total, `truncated: true` marked where applicable.
- [ ] **JIRA-07**: User gets cache file (`~/.config/waveterm/jira-cache.json`) written atomically in the existing schema (`JiraIssue` with description, attachments[], comments[], commentCount, lastCommentAt). ADF descriptions and comment bodies are converted to plain/markdown text.

### Refresh Flow

- [ ] **JIRA-03**: User can click the widget's ☁️ (cloud-arrow-down) button and the Waveterm backend itself fetches fresh data from Jira. No AI CLI is involved. Progress (issue N/total, writing cache) is surfaced to the widget.
- [ ] **JIRA-08**: User can invoke `wsh jira refresh` from any terminal and trigger the same flow as the widget button.

### Setup & Error UX

- [ ] **JIRA-04**: User who launches the widget without a valid `jira.json` sees an actionable empty state (with a copy-pastable Claude prompt that asks Claude to create the config) instead of a cryptic error. Invalid token / 401 from Jira shows a distinct "인증 실패 — PAT 재발급 필요" message.
- [ ] **JIRA-09**: User can follow a one-page README to set up their own copy: where to get a PAT, what `jira.json` must contain, how to verify with a quick `wsh jira refresh`.

### On-demand Attachments

- [ ] **JIRA-05**: User can run `wsh jira download <ISSUE-KEY> [filename]` to download specific attachments into `~/.config/waveterm/jira-attachments/<KEY>/`. The cache entry's `localPath` is updated and subsequent widget opens use the local file instead of webview.

### Hardening

- [ ] **JIRA-10**: Refresh handles Atlassian rate limits (respects `Retry-After`, max 10 req/s per client) and retries transient 5xx with exponential backoff (max 3 retries).

## Future Requirements (deferred, tracked for later milestones)

- **JIRA-F-01**: Electron `safeStorage` encrypts `apiToken` in jira.json. Migration from plain → encrypted on first launch after upgrade.
- **JIRA-F-02**: React settings modal to edit jira.json via form. Launched from widget empty state.
- **JIRA-F-03**: OAuth 2.0 (PKCE) flow as alternative to PAT.
- **JIRA-F-04**: Automatic attachment pre-download with size cap + per-mime filter.
- **JIRA-F-05**: Jira notifications API integration for accurate unread comment count.
- **JIRA-F-06**: Multi-site support (team members across different Jira Cloud instances).
- **JIRA-F-07**: Upstream PR preparation (plugin architecture, generalized external-service widget framework).

## Out of Scope (explicit — this milestone)

- **Jira Server / Data Center** — we only target Jira Cloud
- **Custom fields exposure** — stick to core fields (summary/description/status/comments/attachments)
- **Write operations** — no commenting, no status transitions, no assignment changes. Read-only.
- **Multi-account per user** — one PAT per jira.json

## Traceability

Filled during roadmap creation — see `ROADMAP.md` for which phase owns which requirement.

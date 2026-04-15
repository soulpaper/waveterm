# Project: Waveterm (KakaoVX fork)

## What This Is

Customized fork of [wavetermdev/waveterm](https://github.com/wavetermdev/waveterm) for KakaoVX internal team use. Adds Jira-integrated workflow widgets on top of upstream Waveterm capabilities (terminal blocks, preview, web views, AI chat, Claude sessions).

**Platform:** Windows 11 (primary target). macOS/Linux kept compatible but not tested.

**Distribution:** Internal binary build shared with KakaoVX team. No upstream PR planned.

## Core Value

Let engineers browse and act on their Jira work directly from Waveterm without leaving the terminal workspace — list, filter, read, and hand off to AI analysis, all from one panel.

## Current Milestone: v1.0 Jira Team Integration

**Goal:** Remove Claude dependency from the Jira Tasks widget's core data path so teammates can use it with just Waveterm + their own Jira PAT. Analysis (AI-powered) stays optional per user.

**Target features:**
- Go backend Jira HTTP client (PAT auth) populates the widget cache
- `wsh jira refresh` RPC callable from widget button and terminal
- Error UX when credentials missing/invalid, with setup guidance
- On-demand attachment download via wsh command
- Windows-first, cross-platform compatible

**Non-goals (this milestone):**
- OAuth 2.0 / PKCE (PAT only)
- Electron `safeStorage` token encryption (plain JSON at `~/.config/waveterm/jira.json`)
- React settings modal UI (plain JSON edit + Claude-assisted setup)
- Atlassian notifications API
- Jira Server (on-prem) support
- Attachment pre-download on refresh

## Validated Requirements (already built — pre-GSD)

- [x] Jira Tasks widget reads cache file and displays issues
- [x] Expandable cards with description, attachments, comments
- [x] Filters: project, status, date range
- [x] Auto-refresh interval (re-reads cache file)
- [x] New-issue badge via `lastSeenAt` tracking
- [x] Analyze button sends prompt to configurable CLI (`claude`, `gemini`, etc.)
- [x] Terminal target picker with cwd/display-name labels
- [x] Skill selector (slash-command datalist) + extra prompt textarea
- [x] Per-terminal blockId tag in block header

## Active Requirements

See `REQUIREMENTS.md` — tracked via `JIRA-*` IDs.

## Key Decisions

| Decision | Rationale | Date |
|---|---|---|
| PAT over OAuth | Single internal site (kakaovx.atlassian.net), all teammates can generate tokens, OAuth adds redirect/refresh complexity not justified for internal fork | 2026-04-15 |
| Plain JSON config | safeStorage adds cross-platform test burden; home-dir JSON is sufficient for trusted internal machines; safeStorage deferred to post-MVP | 2026-04-15 |
| Cache schema unchanged | Widget already consumes `{description, attachments[], comments[], commentCount, lastCommentAt}` — backend must produce same shape so widget code is untouched | 2026-04-15 |
| No pre-download of attachments | Size/bandwidth concerns; webview fallback works via site URL + browser session cookies; on-demand via `wsh jira download` | 2026-04-15 |
| Windows-primary | All current teammates on Windows; Go HTTP + file I/O is cross-platform by nature, so macOS/Linux should work but isn't UAT-tested | 2026-04-15 |
| Internal fork only | No upstream PR for now — fork stays in `soulpaper/waveterm`, rebased periodically from upstream main | 2026-04-15 |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---

Last updated: 2026-04-15 — Milestone v1.0 started

# Milestones

## Active

### v1.0 Jira Team Integration — 2026-04-15 → in progress

Remove Claude dependency from Jira Tasks widget. Waveterm Go backend talks directly to Jira Cloud via PAT. Teammates install the fork + create `jira.json` and have a working widget without Claude.

See `ROADMAP.md` for phases.

## Completed

_(Pre-GSD; recorded for context)_

### Jira Tasks widget MVP — 2026-04-14

Cache-file-reading widget shipped with filters (project/status/date), auto-refresh, new-issue badge, expandable cards with description/attachments/comments, analyze-CLI picker, terminal target dropdown, skill selector, extra prompt. Refresh path relied on Claude + Atlassian MCP (replaced in v1.0).

Key commits: `b547ab1d`, `a5b58530`.

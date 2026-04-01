# rel.ai — AI Session Manager

A CLI tool that captures, indexes, and resumes abandoned Claude Code and OpenAI Codex sessions. Never lose context from an interrupted AI coding session again.

## Problem

You start an AI coding session, get distracted or close the terminal, and later can't remember what you were working on or where you left off. Both Claude Code and Codex CLI store rich session data locally, but provide no way to browse, search, or resume past sessions.

**rel.ai** is your safety net — it scans local session data, builds a searchable index, and generates context blobs you can paste into a new session to pick up exactly where you left off.

## Install

```bash
go install github.com/timae/rel.ai/cmd/ses@latest
```

> Requires `~/go/bin` in your `$PATH`. Add `export PATH="$HOME/go/bin:$PATH"` to your `~/.zshrc` or `~/.bashrc` if not already set.

Or build from source:

```bash
git clone https://github.com/timae/rel.ai.git
cd rel.ai
go build -o ses ./cmd/ses
```

## Quick Start

```bash
# Import all sessions from ~/.claude/ and ~/.codex/
ses scan

# Browse recent sessions
ses list

# Filter by source, project, date, or tag
ses list --source claude --project myapp --since 2026-03-01

# View full session details
ses show a3f2

# Search across all session content
ses search "authentication bug"

# Tag sessions for later
ses tag a3f2 "auth,bug,urgent"

# Generate a resume context blob and copy to clipboard
ses resume a3f2 | pbcopy
```

Then paste the resume output into a new Claude Code or Codex session — the AI immediately understands your prior context.

Or skip the clipboard entirely:

```bash
# Launch a new Claude session with context pre-loaded
ses resume a3f2 --inject

# Watch for new sessions in real-time
ses watch

# See your AI coding analytics
ses stats

# Show what code changed during a session
ses diff a3f2 --stat

# Chain related sessions together
ses link a3f2 b5c6 --reason "same feature"
ses resume a3f2 --chain    # includes linked session context
```

## Commands

| Command | Description |
|---|---|
| `ses scan [--full]` | Import sessions from `~/.claude/` and `~/.codex/` into a local SQLite index |
| `ses list [flags]` | Browse sessions with filters (`--since`, `--until`, `--project`, `--source`, `--tag`, `--limit`) |
| `ses show <id>` | Display session details: metadata, conversation summary, files touched, linked sessions |
| `ses search <query>` | Full-text search (FTS5) across session content |
| `ses tag <id> <tags>` | Add/remove comma-separated tags (`--remove` to delete) |
| `ses resume <id>` | Generate markdown context blob for resuming a session |
| `ses resume <id> --inject` | Launch a new Claude/Codex session pre-loaded with context |
| `ses resume <id> --chain` | Include all linked sessions in the resume context |
| `ses watch` | Auto-scan daemon — watches for new sessions and indexes them in real-time |
| `ses stats` | Analytics dashboard — session counts, durations, models, projects, activity heatmap |
| `ses diff <id>` | Show the git diff produced during a session's time window |
| `ses link <id1> <id2>` | Chain related sessions together (with optional `--reason`) |

## What It Captures

From **Claude Code** (`~/.claude/`):
- Session metadata (UUID, PID, working directory, start time)
- Full conversation transcripts with tool calls
- Git branch at time of session
- Files read, written, and edited
- Model used (e.g. `claude-opus-4-6`)

From **Codex CLI** (`~/.codex/`):
- Session metadata (UUID, working directory, git branch/commit)
- User prompts and agent responses
- Function calls (commands executed)
- Model provider info

## Resume Output

The `ses resume` command generates structured markdown like this:

```markdown
# Session Resume: Fix authentication bug in login flow

## Context
- **Project**: /Users/you/myapp
- **Git branch**: feature/auth-fix (at a1b2c3d4)
- **When**: 2026-03-20T13:22:16Z (2h15m)
- **Source**: claude (claude-opus-4-6)
- **Messages**: 48, **Tool calls**: 23

## Original Goal
The login endpoint returns 401 even with valid credentials after the session migration...

## Key Prompts During Session
1. Can you check the session middleware?
2. The token validation is failing on refresh tokens specifically
3. Try running the integration tests

## What Was Accomplished
Identified the root cause in the token refresh logic...

## Where It Left Off
I've updated the refresh token validation but haven't run the full test suite yet...

## Files Touched
- [edit] src/auth/middleware.ts
- [edit] src/auth/tokens.ts
- [read] src/auth/tests/integration.test.ts

## Resume Instructions
Continue working on this task. The session was interrupted.
Pick up where the previous assistant left off.
Review the files listed above for current state.
```

## Storage

- **Index**: SQLite database at `~/.ses/index.db` (configurable via `--db`)
- **Transcripts**: Referenced from original locations, not copied
- **Incremental scan**: Only re-parses changed files (by mtime + size)
- **Full rescan**: `ses scan --full` rebuilds the entire index

The database is a disposable cache — delete it and `ses scan` rebuilds everything from source files.

## Architecture

```
ses/
  cmd/ses/main.go       # Entry point
  cmd/                  # Cobra CLI commands (scan, list, show, search, tag, resume, watch, stats, diff, link)
  internal/
    db/                 # SQLite + FTS5 schema, queries, stats, links
    scanner/            # Claude Code + Codex CLI parsers
    model/              # Unified session data types
    resume/             # Context blob generator (full + brief for chains)
    display/            # Terminal formatting + stats dashboard
    gitutil/            # Git diff/log utilities
```

Built with:
- [cobra](https://github.com/spf13/cobra) — CLI framework
- [modernc.org/sqlite](https://modernc.org/sqlite) — Pure Go SQLite (no CGo)
- [fatih/color](https://github.com/fatih/color) — Terminal colors
- [fsnotify](https://github.com/fsnotify/fsnotify) — File system watcher

## License

MIT

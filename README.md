# rel.ai — AI Session Manager

A CLI tool that captures, indexes, and resumes abandoned Claude Code and OpenAI Codex sessions. Never lose context from an interrupted AI coding session again.

## Problem

You start an AI coding session, get distracted or close the terminal, and later can't remember what you were working on or where you left off. Both Claude Code and Codex CLI store rich session data locally, but provide no way to browse, search, or resume past sessions.

**rel.ai** is your safety net — it scans local session data, builds a searchable index, and generates context blobs you can paste into a new session to pick up exactly where you left off.

## Install

### CLI

```bash
go install github.com/timae/rel.ai/cmd/ses@latest
```

> Requires `~/go/bin` in your `$PATH`. Add `export PATH="$HOME/go/bin:$PATH"` to your `~/.zshrc` or `~/.bashrc` if not already set.

### Menu Bar App (macOS)

```bash
go install github.com/timae/rel.ai/cmd/ses-menu@latest
```

> Requires CGo (`CGO_ENABLED=1`) and Xcode Command Line Tools.

Or build both from source:

```bash
git clone https://github.com/timae/rel.ai.git
cd rel.ai
go build -o ses ./cmd/ses
CGO_ENABLED=1 go build -o ses-menu ./cmd/ses-menu
```

## Quick Start

```bash
# 1. Import all sessions from ~/.claude/ and ~/.codex/
ses scan

# 2. Install the background daemon (recommended — never run scan again)
ses watch --install

# 3. Browse recent sessions
ses list

# 4. Resume a session directly into Claude Code or Codex
ses resume a3f2 --inject
```

## Commands

### Core

| Command | Description |
|---|---|
| `ses scan [--full]` | Import sessions from `~/.claude/` and `~/.codex/` into a local SQLite index |
| `ses list [flags]` | Browse sessions with filters (`--since`, `--until`, `--project`, `--source`, `--tag`, `--limit`) |
| `ses show <id>` | Display session details: metadata, conversation summary, files touched, linked sessions |
| `ses search <query>` | Full-text search (FTS5) across session content |
| `ses tag <id> <tags>` | Add/remove comma-separated tags (`--remove` to delete) |

```bash
# Filter by source, project, date, or tag
ses list --source claude --project myapp --since 2026-03-01

# Search across all session content
ses search "authentication bug"

# Tag sessions for later
ses tag a3f2 "auth,bug,urgent"
```

### Resume & Inject

| Command | Description |
|---|---|
| `ses resume <id>` | Generate markdown context blob for resuming a session |
| `ses resume <id> --inject` | Launch a new Claude/Codex session pre-loaded with context |
| `ses resume <id> --chain` | Include all linked sessions in the resume context |
| `ses resume <id> --target codex` | Override which CLI to launch (default: matches session source) |

The `--inject` flag writes the context to a temp file and launches the appropriate CLI:
- **Claude Code**: Changes to the project directory, then runs `claude --append-system-prompt-file <context>`
- **Codex CLI**: `codex --cd <project> <context>`

```bash
# Print resume context to stdout (pipe or copy manually)
ses resume a3f2 | pbcopy

# Launch directly into a new CLI session with context
ses resume a3f2 --inject

# Include context from linked sessions
ses resume a3f2 --chain --inject
```

### Watch (Background Daemon)

| Command | Description |
|---|---|
| `ses watch` | Watch for new sessions in the foreground |
| `ses watch --install` | Install as a macOS LaunchAgent (starts on login, auto-restarts) |
| `ses watch --uninstall` | Remove the LaunchAgent |
| `ses watch --status` | Check if the daemon is running |

The daemon monitors `~/.claude/projects/` and `~/.codex/sessions/` for new or modified transcript files. When a session ends (or while it's in progress), the index is updated automatically. No more manual `ses scan`.

```bash
$ ses watch --install
Daemon installed and started.
  Binary:  ~/go/bin/ses
  Plist:   ~/Library/LaunchAgents/ai.rel.ses.watch.plist
  Log:     ~/.ses/watch.log

Sessions will be indexed automatically from now on.
The daemon starts on login and restarts if it crashes.

$ ses watch --status
Daemon is running.
  PID = 41418
  Log: ~/.ses/watch.log (09:52)
```

### Menu Bar App (macOS)

A companion menu bar app that puts a robot (🤖) in your status bar.

| Command | Description |
|---|---|
| `ses menu` | Launch the menu bar app |
| `ses menu --install` | Install as a macOS LaunchAgent (starts on login) |
| `ses menu --uninstall` | Remove the LaunchAgent |

**What the robot shows:**
- Daemon status (running/stopped)
- Last 5 sessions — click to copy `ses resume <id> --inject` to clipboard
- Quick stats (total sessions, this week)
- Scan Now / Restart Daemon actions

```bash
# Install both the daemon and menu bar app
ses watch --install
ses menu --install
```

### Stats (Analytics Dashboard)

```bash
ses stats [--since DATE] [--until DATE] [--project PATH] [--source claude|codex]
```

```
╔══════════════════════════════════════════╗
║            SESSION STATISTICS            ║
╠══════════════════════════════════════════╣
║ Total sessions                       142 ║
║ This week                             18 ║
║ Avg duration                     1h 34m ║
║ Avg messages/session                  36 ║
║ Total tool calls                   4,291 ║
╠══════════════════════════════════════════╣
║                BY SOURCE                 ║
║                                          ║
║   claude                       98  (69%) ║
║   codex                        44  (31%) ║
╠══════════════════════════════════════════╣
║                 BY MODEL                 ║
║                                          ║
║   claude-opus-4-6                     52 ║
║   claude-sonnet-4-6                   46 ║
║   openai                              44 ║
╠══════════════════════════════════════════╣
║               TOP PROJECTS               ║
║                                          ║
║   ~/projects/webapp                   38 ║
║   ~/projects/api-server               27 ║
║   ~/projects/mobile-app               19 ║
║   ~/projects/infra                    14 ║
║   ~/projects/docs                      9 ║
╠══════════════════════════════════════════╣
║          ACTIVITY (last 7 days)          ║
║                                          ║
║  Mon 03/24  ████████████  6              ║
║  Tue 03/25  ██████░░░░░░  3              ║
║  Wed 03/26  ████████████  6              ║
║  Thu 03/27  ████░░░░░░░░  2              ║
║  Fri 03/28  ░░░░░░░░░░░░  0              ║
║  Sat 03/29  ██░░░░░░░░░░  1              ║
║ *Sun 03/30  ░░░░░░░░░░░░  0              ║
╠══════════════════════════════════════════╣
║               TOP TAGS                   ║
║                                          ║
║  #bug (12)  #refactor (8)  #deploy (5)   ║
╚══════════════════════════════════════════╝
```

### Diff (Git Changes)

Show the code changes made during a session by finding commits within the session's time window.

```bash
# Full diff
ses diff a3f2

# File-level summary
ses diff a3f2 --stat

# Just file names
ses diff a3f2 --files-only
```

```
$ ses diff a3f2 --stat
 src/auth/middleware.ts       | 45 ++++++++---
 src/auth/tokens.ts          | 23 +++--
 src/auth/tests/auth.test.ts | 67 ++++++++++++++
 3 files changed, 112 insertions(+), 23 deletions(-)
```

If the session recorded a starting git commit (Codex sessions do this), the diff shows changes from that commit to HEAD. Otherwise, it finds all commits within the session's start/end time range.

### Link (Chain Sessions)

Chain related sessions that are part of the same task across multiple sittings.

```bash
# Link two sessions
ses link a3f2 b5c6 --reason "continuing auth fix"

# View the chain
ses link a3f2 --list
Sessions linked to a3f2:
  → b5c6d7e8 claude — Run the full test suite (continuing auth fix)
  ← 1a2b3c4d codex  — Initial investigation (same bug)

# Resume with full chain context
ses resume a3f2 --chain
```

The `--chain` flag on resume generates a combined context blob that includes summaries from all linked sessions in chronological order, giving the AI the full history of your multi-session task.

Linked sessions also appear in `ses show`:

```
$ ses show a3f2
Session a3f2b5c6
────────────────────────────────────────────────────────────
  Source:        claude (claude-opus-4-6)
  ...

Linked Sessions
  → b5c6d7e8 claude — Run the full test suite (continuing auth fix)
  ← 1a2b3c4d codex  — Initial investigation (same bug)
```

## Running in Containers / Sandboxed Environments

`ses` reads session data from `~/.claude/` and `~/.codex/` on the local filesystem. If you run Claude Code or Codex inside a container, DevContainer, or remote VM, you need to make the session data accessible.

### Option 1: Volume mount (recommended)

Mount the container's session directories to persistent storage on the host:

```bash
docker run \
  -v ~/.claude:/root/.claude \
  -v ~/.codex:/root/.codex \
  your-image
```

For Docker Compose:

```yaml
volumes:
  - ~/.claude:/root/.claude
  - ~/.codex:/root/.codex
```

Then run `ses` on the host as normal — the daemon picks up sessions automatically.

### Option 2: Install ses inside the container

Add `ses` to your container image:

```dockerfile
RUN go install github.com/timae/rel.ai/cmd/ses@latest
```

Run `ses scan` and `ses list` inside the container. Note that the background daemon and menu bar app are host-only features.

### Option 3: Copy session data out

If you can't mount volumes, copy the session directories after your work:

```bash
docker cp <container>:/root/.claude/ ~/.claude/
docker cp <container>:/root/.codex/ ~/.codex/
ses scan
```

### DevContainers / GitHub Codespaces

Add the volume mount to your `.devcontainer/devcontainer.json`:

```json
{
  "mounts": [
    "source=${localEnv:HOME}/.claude,target=/root/.claude,type=bind",
    "source=${localEnv:HOME}/.codex,target=/root/.codex,type=bind"
  ]
}
```

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
- **Project**: /home/user/myapp
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
- **Daemon log**: `~/.ses/watch.log` when running as a LaunchAgent
- **Menu log**: `~/.ses/menu.log` when running as a LaunchAgent

The database is a disposable cache — delete it and `ses scan` rebuilds everything from source files.

## Architecture

```
ses/
  cmd/
    ses/main.go         # CLI entry point
    ses-menu/main.go    # Menu bar app entry point
    scan.go             # Import sessions
    list.go             # Browse with filters
    show.go             # Session details
    search.go           # Full-text search
    tag.go              # Session tagging
    resume.go           # Context generation + inject + chain
    watch.go            # File watcher + daemon management
    stats.go            # Analytics dashboard
    diff.go             # Git diff integration
    link.go             # Session chaining
    menu.go             # Menu bar launcher + LaunchAgent
  internal/
    db/                 # SQLite + FTS5 schema, queries, stats, links
    scanner/            # Claude Code + Codex CLI parsers
    model/              # Unified session data types
    resume/             # Context blob generator (full + brief for chains)
    display/            # Terminal formatting + stats dashboard
    tray/               # Menu bar app (macOS, robot icon)
    gitutil/            # Git diff/log utilities
```

Built with:
- [cobra](https://github.com/spf13/cobra) — CLI framework
- [modernc.org/sqlite](https://modernc.org/sqlite) — Pure Go SQLite (no CGo)
- [fatih/color](https://github.com/fatih/color) — Terminal colors
- [fsnotify](https://github.com/fsnotify/fsnotify) — File system watcher
- [menuet](https://github.com/caseymrm/menuet) — macOS menu bar (CGo)

## License

MIT

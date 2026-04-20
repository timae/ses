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

### Share (Preview)

`ses share <id>` redacts a session transcript, uploads it to a share server you run, and returns a time-limited URL you can paste into Slack or a PR. Point it at your own instance — the hosted server is a pure-Go binary at `cmd/share-server` with a Dockerfile for deploio / any container platform. See [Running the share server](#running-the-share-server) below.

```bash
# One-time: configure the CLI against your share server
ses share login --url https://share.example.com --token <bearer>

# Preview what would be uploaded (no network)
ses share a3f2 --dry-run

# Upload for real — prints the URL on stdout, so `| pbcopy` works
ses share a3f2 --expires 7d --name "auth bug repro"

# Stricter redaction (also scrubs emails and URL paths)
ses share a3f2 --dry-run --redact=strict

# See what you've shared from this machine (expires + label)
ses share list

# Pull a share down early
ses share revoke <share-id>
```

Example output header:

```
# Share preview: a3f2b5c6

Project:   /Users/you/myapp
Source:    claude
Messages:  48
Redaction: default

## Redaction report
  git-remote     1
  path           12
  secret         2
  total:         486 chars removed

## Scrubbed transcript
...
```

**What the default mode scrubs:**

| Rule | Catches |
|---|---|
| `path` | Replaces your home directory with `~` everywhere (content + file paths) |
| `git-remote` | URLs like `https://user:token@host/…` and `ssh://user@host/…` |
| `secret` | OpenAI (`sk-…`), Anthropic (`sk-ant-…`), GitHub PATs (`ghp_…`, `github_pat_…`), AWS keys (`AKIA…`), bearer tokens, JWTs, `-----BEGIN PRIVATE KEY-----` blocks, env assignments like `FOO_TOKEN=…` |
| `long-paste` | Truncates any single message over 8 KB (2 KB in strict) |

Strict mode additionally scrubs email addresses and the path/query portion of URLs.

**How to help**: run `ses share <id> --dry-run` against your real sessions and [file an issue](https://github.com/timae/rel.ai/issues) if you see:

- A secret shape that wasn't caught (tell us the *shape*, not the actual secret — e.g. "Stripe `sk_live_…` keys aren't matched")
- A false positive that's scrubbing something harmless
- A long paste that should have been kept intact

No data leaves your machine when you run `--dry-run`. The output only goes to your terminal.

#### Running the share server

The server is a small HTTP service with four endpoints (upload, HTML view, raw download, revoke) and a background sweeper that deletes expired shares. Storage is filesystem-backed — persist a single directory and you keep state across restarts.

```bash
# Local run
SHARE_BEARER=$(openssl rand -hex 24) \
SHARE_PUBLIC_URL=http://localhost:8080 \
SHARE_DATA_DIR=./data/shares \
go run ./cmd/share-server

# Container image
docker build -f cmd/share-server/Dockerfile -t ses-share-server .
docker run --rm \
  -e SHARE_BEARER=$(openssl rand -hex 24) \
  -e SHARE_PUBLIC_URL=https://share.example.com \
  -v ses-share-data:/data \
  -p 8080:8080 \
  ses-share-server
```

| Env | Required | Default | Purpose |
|---|---|---|---|
| `SHARE_BEARER` | yes | — | Bearer token the CLI sends on upload + revoke |
| `SHARE_PUBLIC_URL` | yes | — | External base URL, used when building share URLs |
| `SHARE_DATA_DIR` | no | `/data/shares` (container) / `./data/shares` (local) | Storage directory |
| `SHARE_ADDR` | no | `:8080` | Listen address |
| `SHARE_SWEEP_INTERVAL` | no | `15m` | How often expired shares are deleted |
| `SHARE_MAX_BODY_BYTES` | no | `10485760` | Upload cap (10 MB) |

Deploying on [deploio](https://deplo.io) (Nine's PaaS) or any container platform: push the image, set the four env vars, mount a persistent volume at `/data`, done. Shares are single-file-per-upload so a nightly volume snapshot is sufficient backup.

### Handoff (Mid-Session)

Static share is read-only. If you're in the middle of a session and need to *hand it off* to a teammate so they can continue the task, use `ses handoff`:

```bash
# Sender (mid-stuck)
ses handoff a3f2 \
  --note "token refresh works, test flaky on refresh_rotation — look at tokens_spec.rb:142" \
  --expires 24h \
  --chain               # also bundle linked prior sessions
# → prints: single-use handoff URL
```

```bash
# Recipient, on their own machine
cd ~/code/myapp                      # their copy of the project
ses resume --from https://share.example.com/s/abc
# → consumes the URL (deletes it server-side), saves a scrubbed local copy
#   to ~/.ses/handoffs/, launches Claude Code with the note, transcript
#   summary, linked-session chain, and files-touched manifest all injected
#   as the system prompt.
```

`--project <path>` overrides which directory Claude Code launches in — defaults to the recipient's current working directory.

**How this differs from static share:**

- **Single-use by default.** The first `ses resume --from <url>` deletes the share server-side. Forwarding the URL after that returns 404.
- **HTML view doesn't leak the transcript.** The claim page shows the note, metadata, files list, and the exact CLI command to run — but the messages are only returned to `POST /v1/shares/{id}/consume`, which the browser never calls.
- **Shorter default expiry.** `ses handoff` defaults to 24h, not 7d — handoffs are meant to be picked up soon.
- **Full context blob.** The recipient's Claude Code gets: the sender's note pinned at the top, the original session's metadata, the files-touched manifest, linked-session briefs, and a resume-style summary of the transcript — all redacted the same way static shares are.

Because the claim URL is the capability, **only share the handoff URL with the intended recipient** (DM, not a public channel). Anyone with the URL who reaches `consume` first wins the claim.

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
    share-server/       # HTTP service hosting expiring share links (Dockerfile)
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
    share.go            # ses share (snapshot upload + preview)
    handoff.go          # ses handoff (single-use claim link for mid-session handoff)
    resume_from.go      # ses resume --from <url> (claim a handoff)
  internal/
    db/                 # SQLite + FTS5 schema, queries, stats, links
    scanner/            # Claude Code + Codex CLI parsers
    model/              # Unified session data types
    resume/             # Context blob generator (full + brief for chains)
    redact/             # Pre-share transcript scrubbing (paths, secrets, creds)
    shareserver/        # HTTP handlers, storage, HTML template for the share service
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

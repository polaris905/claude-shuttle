# claude-shuttle

**Shuttle your Claude Code sessions across devices. Pick up exactly where you left off.**

---

## The Problem

It's 6 PM. You've spent the last three hours on your **office desktop**, deep in a Claude Code session — debugging a tricky race condition, building up context, refining your approach through dozens of prompts. Claude knows your codebase, understands the bug, and has a plan for the fix. Time to shuttle home and keep going.

You run `claude-shuttle push` on your office PC, grab your bag, and head out. At home, you sit down at your **home PC** — a completely different machine. `claude-shuttle pull`, then `/resume`. Claude picks up mid-thought — same context, same plan, same momentum. Different computer, same session.

Without claude-shuttle? You open Claude Code on your home PC and get a fresh session. Empty context. Claude has no idea what you were working on an hour ago. That session is locked inside `~/.claude/` on your office desktop — a machine you can't reach until Monday.

**Three hours of accumulated context, stranded on another computer.**

### Why `git commit` isn't enough

You might think: "I'll just commit my code changes and pull them at home." But a Claude Code session is far more than your code diff:

- **Conversation history** — dozens of back-and-forth messages where you explained the problem, explored approaches, and narrowed down the root cause. This context is what makes Claude effective on your next prompt.
- **Prompt history** — your up/down arrow key history, so you can recall and refine previous prompts.
- **Task tracking** — in-progress tasks, what's done, what's blocked.
- **Plan files** — architectural decisions and implementation plans you built collaboratively.
- **Memory files** — project-specific knowledge Claude accumulated across sessions.
- **File edit history** — version snapshots of every file Claude touched, enabling undo/rollback.
- **Subagent context** — research and exploration done by background agents.

All of this lives in `~/.claude/` on your local machine. None of it goes into `git`. When you switch devices, **you lose everything except the code itself** — and the code is often the least valuable part of what Claude was holding for you.

### The hybrid work reality

Many of us split time between office desktops, home PCs, and laptops. Every device switch is a context reset. You end up re-explaining problems, re-establishing conventions, and re-doing exploration that Claude already completed on another machine.

**claude-shuttle** solves this. One command to push your session to the cloud. One command to pull it on another device. `/resume` just works — as if you never left.

---

## Why claude-shuttle?

**Your data never leaves infrastructure you already control.**

Your Claude Code session history isn't just metadata — it contains the substance of your engineering work: code snippets, debugging strategies, architectural decisions, and sometimes credentials or internal URLs that appeared in tool output.

claude-shuttle keeps all of this on storage you already own and trust:

- **Bring Your Own Storage** — Sessions travel through OneDrive, Dropbox, Google Drive, or any cloud folder you control. No third-party servers, no new accounts, no API keys.
- **Zero infrastructure** — No backend service to depend on. Works offline, folder-to-folder. If your cloud storage client is running, claude-shuttle works.
- **Free forever** — Uses storage you already pay for. No subscriptions, no usage tiers.
- **Enterprise-ready** — Routes through IT-approved storage (OneDrive/SharePoint). No new vendors to approve, no new data processors to evaluate, no new security reviews.
- **Pluggable providers** — Not locked into any single cloud service. Swap backends freely.
- **True path rewriting** — Rewrites paths *inside* the session JSONL itself — `cwd`, `project`, file paths in tool calls — so Claude Code sees native local paths. `/resume` works transparently, as if the session was always local.

---

## How It Works

```
Office PC (6 PM, heading out):             Home PC (6:30 PM, picking up):
  claude-shuttle push                       claude-shuttle pull
  ├─ Collects session history               ├─ Downloads session from OneDrive
  ├─ Bundles file edits, tasks, plans       ├─ Detects different file paths
  ├─ Uploads to OneDrive                    ├─ Rewrites paths for this machine
  └─ Done in seconds                        └─ /resume — right where you left off
```

claude-shuttle copies all session metadata through your existing cloud storage. It handles the tricky parts automatically — like the fact that your project lives at `C:\Users\alice\repos\MyApp` at the office but `C:\Users\alice\projects\MyApp` at home.

### Your storage, your choice

claude-shuttle doesn't lock you into any particular cloud service. It uses a **pluggable storage provider** model — your session data flows through whatever cloud storage you already trust and use:

| Provider | Status | How it works |
|---|---|---|
| **OneDrive** | Supported | Transfers via your local OneDrive folder |
| **Google Drive** | Planned | Same folder-based approach |
| **Dropbox** | Planned | Same folder-based approach |
| **iCloud Drive** | Planned | Same folder-based approach |

Adding a new backend is a matter of implementing 5 methods in the Provider interface — see [Pluggable storage providers](#pluggable-storage-providers) below.

No data touches third-party servers beyond what your chosen cloud storage already does. Your sessions, your infrastructure.

---

## Prerequisites

claude-shuttle doesn't talk to cloud APIs or handle authentication itself. Instead, it writes to a **local folder managed by your cloud storage desktop client** — OneDrive, Dropbox, Google Drive, etc. The desktop client handles authentication, encryption, and upload/download in the background.

**You need:**
- A cloud storage desktop client (e.g., OneDrive, Dropbox, Google Drive) **installed and signed in** on each device
- A folder within that client's managed area designated for claude-shuttle (e.g., `OneDrive/ClaudeShuttle`)

That's it. No API keys, no OAuth flows, no accounts with claude-shuttle itself.

> **How push actually works:** When you run `claude-shuttle push`, session files are copied into your cloud storage folder on the local filesystem. The cloud storage client then uploads them to the cloud in the background. claude-shuttle does not wait for the cloud upload to complete — it finishes as soon as the local copy is done. In practice, small session bundles upload within seconds, but you should allow a moment for your cloud client to finish before shutting down your machine.

---

## Quick Start

### 1. Download

Grab the binary for your platform from [Releases](https://github.com/polaris905/claude-shuttle/releases):

| Platform | Archive |
|---|---|
| Windows (x64) | `claude-shuttle-windows-amd64.zip` |
| Linux (x64) | `claude-shuttle-linux-amd64.tar.gz` |
| macOS (Apple Silicon) | `claude-shuttle-darwin-arm64.tar.gz` |
| macOS (Intel) | `claude-shuttle-darwin-amd64.tar.gz` |

Extract the archive to get `claude-shuttle` (or `claude-shuttle.exe` on Windows). Single binary, zero dependencies. No runtime to install.

### 2. Configure

Create a folder inside your cloud storage area for claude-shuttle, then point the tool at it:

```bash
# Create the folder first — claude-shuttle requires it to already exist
mkdir "C:\Users\me\OneDrive\ClaudeShuttle"

# Then configure
claude-shuttle config --storage onedrive --remote-path "C:\Users\me\OneDrive\ClaudeShuttle"
```

Run this on each device. The `--remote-path` must point to an existing directory within your cloud storage client's managed area.

### 3. Push & Pull

```bash
# 6 PM — shuttle your session before heading out:
claude-shuttle push

# At home — land the session on your home machine:
claude-shuttle pull

# Continue right where you left off:
/resume
```

That's it. Your context shuttles with you.

---

## Commands

| Command | Description |
|---|---|
| `claude-shuttle config --storage onedrive --remote-path <path>` | Configure cloud storage |
| `claude-shuttle config` | Show current config |
| `claude-shuttle push` | Select and push a session |
| `claude-shuttle push -s <session-id>` | Push a specific session |
| `claude-shuttle pull` | Pull most recent cloud session |
| `claude-shuttle pull -s <session-id>` | Pull a specific session |
| `claude-shuttle list` | List all transferred sessions |
| `claude-shuttle version` | Show version |
| `claude-shuttle help` | Show help |

---

## Key Design Decisions

### Smart path detection & rewriting

Different devices have different paths. Your project might be at:
- `C:\Users\alice\repos\MyApp` (office)
- `/home/alice/projects/MyApp` (home Linux box)
- `/Users/alice/code/MyApp` (MacBook)

When you pull a session, claude-shuttle **scans the conversation history for all file paths** referenced in tool calls (Read, Edit, Write, Bash commands). It then:

1. Checks if each path exists on the local machine
2. Checks saved path mappings from previous pulls
3. Asks you for any unresolved paths
4. Offers to save new mappings for next time

All paths inside the session JSONL are rewritten so Claude Code sees local paths — `/resume` works transparently.

### What gets transferred

| Data | Transfer behavior |
|---|---|
| Conversation history | Overwritten (session-scoped) |
| Subagents & tool results | Overwritten (session-scoped) |
| File edit history | Overwritten (session-scoped) |
| Task state | Overwritten (session-scoped) |
| Plan files | Asks before overwriting (shared across sessions) |
| Memory files | Per-file merge: adds new files, asks before overwriting existing ones (compares timestamps) |
| History index | Append-only (never overwrites existing entries) |


### Pluggable storage providers

The storage layer is an interface:

```go
type Provider interface {
    TestConnection() error
    PushSession(sessionID string, bundleDir string) error
    PullSession(sessionID string, destDir string) error
    ListSessions() ([]SessionInfo, error)
    RemoveSession(sessionID string) error
}
```

Currently ships with an **OneDrive provider** (folder-based — works with any folder that your cloud storage client manages). Adding Google Drive, Dropbox, or any other backend is a matter of implementing these 5 methods.

### Overwrite-only, no merge

claude-shuttle does **not** attempt to merge divergent session histories. If you work on the same session from two devices without transferring, the push/pull model is simple:

- **Push** always overwrites the cloud copy (with confirmation)
- **Pull** always overwrites the local copy (with confirmation)
- The most recently pushed version wins

This is a deliberate simplicity trade-off. Session divergence is rare in practice — you typically work on one device at a time and transfer when switching. A full version-control system for session files would add significant complexity for an edge case.

---

## Limitations

- **Claude Code internal format** — Session files are not a public API. Format changes in future Claude Code versions could require updates to claude-shuttle.
- **No real-time transfer** — This is a manual push/pull workflow, not a background daemon. You push when you leave, pull when you arrive.
- **No multi-device merge** — If two devices diverge on the same session, the last push wins. There is no three-way merge.
- **Large sessions** — Session JSONL files can be 10MB+ for long conversations. Push/pull time depends on your cloud storage upload speed.
- **Path rewriting is string-based** — Works well for project paths in structured fields. Paths embedded in code snippets or freeform text within the conversation are not rewritten (they don't affect Claude Code's functionality).
- **Machine-specific configs are not transferred** — MCP server configs, custom slash commands, Claude settings, and global CLAUDE.md are per-device and not included in the session bundle. If your session uses MCP tools or custom commands, set those up independently on each device. Project-level CLAUDE.md lives in your repo — use git.

---

## Building from Source

Requires Go 1.24+.

```bash
# Build for current platform
make build

# Build with specific version
make build VERSION=1.0.0

# Cross-compile for all platforms
make all

# Binaries output to dist/
```

## Project Structure

```
claude-shuttle/
├── cmd/claude-shuttle/main.go           # CLI entry point
├── internal/
│   ├── config/config.go                 # Config management
│   ├── session/session.go               # Session discovery & bundling
│   ├── rewriter/
│   │   ├── rewriter.go                  # JSONL path rewriting
│   │   └── detector.go                  # Smart path detection
│   ├── manifest/manifest.go             # Transfer manifest
│   └── provider/
│       ├── provider.go                  # Storage provider interface
│       └── onedrive.go                  # OneDrive implementation
├── Makefile
└── LICENSE
```

---

## License

MIT License. Copyright (c) 2026 Cong Li
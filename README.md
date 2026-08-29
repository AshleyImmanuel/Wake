<div align="center">
  <img src="assets/banner.jpg" alt="Wake: AI State Recovery Engine Banner" width="100%" />
  
  <br/>
  
  <h1>Wake</h1>
  <p><b>The missing "Save State" engine for autonomous AI coding agents.</b></p>
  
  [![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-v0.3--alpha-orange.svg)]()
</div>

> **[ALPHA RELEASE NOTICE]:** Wake is currently in early alpha (v0.3). The core checkpoint, reconciliation, and resume pipeline is functional. MCP server and IDE integrations are under active development. If you find bugs or want to contribute, please reach out: **immanuelashley77@gmail.com**

<br/>

## The Problem

By 2026, AI coding agents (like Claude Code, Aider, and Cursor) are incredibly capable, but they suffer from **Amnesia and Context Drift**. 

When you close your laptop, restart your IDE, or run into a token limit, the AI session dies. When you restart it, you either:
1. Burn thousands of tokens re-feeding it the entire chat transcript.
2. Watch it hallucinate because it forgot its original constraints.
3. **Worst of all:** If *you* (the human) manually edit code while the AI is asleep, the AI wakes up totally oblivious to your changes, leading to massive merge conflicts and broken features.

While tools like LangGraph save state, and Devin runs in expensive persistent cloud VMs, **neither actively diffs the AI's brain against the physical Git repository to catch human interference.**

## The Solution

**Wake** is a local-first CLI tool that acts as a referee between your AI agent's "brain" and the actual Git repository. 

It anchors the AI's memory to the physical codebase using an event-sourced SQLite ledger. When a new AI session boots up, Wake diffs the repository against the AI's last memory and generates a compact **Recovery Packet** telling the AI exactly what it completed, what is blocked, and which files changed while it was asleep.

## What Wake Does Today

- **Local-First SQLite Engine**: Tracks task objectives, completed milestones, and blockers without uploading your repo to a cloud VM. Uses WAL mode with serialized connection pooling for safe concurrent access.
- **Git Drift Reconciliation**: Compares the AI's last known state against the live Git repository. Flags `[SAFE]`, `[STALE]`, or `[CONFLICT]` verdicts.
- **Constraint Enforcement**: If you tell the AI "Do not modify auth.go", and you manually modify `auth.go` while it sleeps, Wake throws a hard `[CONFLICT]` to prevent the AI from overwriting your work.
- **Pre-Checkpoint Guard**: Blocks blind checkpoints when unreviewed human modifications or untracked files exist in the working tree.
- **Feature Pivot Support**: Run `wake objective "New Goal"` to safely pivot the AI's memory without a full reset.
- **17-Event State Engine**: Tracks task lifecycle through 17 distinct event types with a deterministic reducer and 3-tier dynamic confidence scoring (High/Low/None).

## Quickstart

### Build from Source

Ensure you have Go 1.27+ installed:

```bash
git clone https://github.com/AshleyImmanuel/Wake.git
cd Wake
go build -o wake .
```

### Basic Workflow

Wake is designed to wrap *any* AI coding agent. 

**1. Initialize Wake in your repo:**
```bash
wake init
```

**2. Set the initial goal (or have your AI run this):**
```bash
wake checkpoint --objective "Migrate the database to PostgreSQL"
```

**3. Check the Status (The AI's brain vs Reality):**
```bash
wake status
```
*Output: `[STALE] Repository has 2 uncommitted changed file(s).`*

**4. Wake the AI back up:**
Feed this output directly to your new agent session:
```bash
wake resume
```

**5. View event history:**
```bash
wake history
```

### CLI Reference

| Command | Description |
|---------|-------------|
| `wake init` | Initialize a `.wake/` workspace in the current directory |
| `wake checkpoint` | Create a versioned state snapshot |
| `wake status` | Show reconciliation status (SAFE/STALE/CONFLICT) |
| `wake resume` | Generate a compact recovery packet for a new AI session |
| `wake history` | View the event history of the active task |
| `wake objective "..."` | Pivot the task objective without resetting state |

### Checkpoint Flags

| Flag | Description |
|------|-------------|
| `--task-id` | Specify a task UUID |
| `--objective` | Set the task objective |
| `--dir` | Target repository directory |
| `-f, --force` | Force checkpoint even with unreviewed changes |
| `--tracked-files` | Comma-separated list of files tracked by the current task |

## Synergy with Existing Tools

Wake is built to **complement**, not replace, existing AI coding tools:

| Tool | Core Strength | How Wake Synergizes |
|------|---------------|---------------------|
| **LangGraph / Checkpointers** | Python state-graph execution | Wake adds Git-level physical file reconciliation to internal memory states |
| **Cursor / Aider** | IDE and codebase semantic search | Wake acts as a background "save state" layer to persist constraints across terminal reboots |
| **Devin / Cloud Agents** | Autonomous execution in persistent cloud environments | Wake provides a local-first alternative for developers who want state-persistence on their local machine |

## Architecture

Wake is built entirely in Go for cross-platform binary distribution. 

```
[CLI Commands]
(checkpoint, status, resume, history, objective, init)
        |
        v
[Application Service Facade]
(internal/service: TaskService)
        |
   +---------+---------+
   v         v         v
[State]   [Git]    [SQLite]
(events,  (client, (db: WAL mode,
 state,    parser)  migrations,
 guard,             indices)
 reconcile)
```

- `internal/events/`: 17 core Event types with deep cloning for thread safety
- `internal/state/`: Deterministic reducer that collapses the event log into a point-in-time snapshot with dynamic confidence scoring
- `internal/reconcile/`: Diffs the SQLite checkpoint against live Git status with path traversal protection
- `internal/guard/`: Pre-checkpoint validation that blocks blind checkpoints on dirty working trees
- `internal/service/`: Application facade that unifies all operations
- `internal/db/`: SQLite persistence with transactional migrations and composite indices

## Roadmap

- **MCP Server (in progress):** Official Model Context Protocol (JSON-RPC 2.0 stdio) server for universal IDE integration (Cursor, VS Code, JetBrains, Claude Desktop)
- **IDE Configuration Generators:** Auto-generate `.cursor/mcp.json`, `.vscode/mcp.json`, and Claude Desktop configs
- **Git-less File Hashing:** Decouple from Git via SHA-256 file hashing for developers who don't use Git
- **Comprehensive Test Suite:** Full unit, integration, and adversarial test coverage

## Copyright & License

**Copyright (c) 2026 Ashley Immanuel. All Rights Reserved.**

This software is proprietary. You may not use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software without express written permission. See the [LICENSE](LICENSE) file for more information.

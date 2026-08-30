<div align="center">
  <img src="assets/banner.jpg" alt="Wake: AI State Recovery Engine Banner" width="100%" />
  
  <br/>
  
  <h1>Wake</h1>
  <p><b>The missing "Save State" engine for autonomous AI coding agents.</b></p>
  
  [![License: Proprietary](https://img.shields.io/badge/License-Proprietary-red.svg)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-v1.0--beta-orange.svg)]()
</div>

> **Tags:** `wake`, `mcp-server`, `ai-agents`, `state-machine`, `local-first`

> **[BETA RELEASE NOTICE]:** Wake is currently in v1.0 beta. The core checkpoint, reconciliation, resume pipeline, MCP server, and IDE integrations are fully functional. If you find bugs or want to contribute, please reach out: **immanuelashley77@gmail.com**

<br/>

## The Problem

By 2026, AI coding agents (like Claude Code, Aider, and Cursor) are incredibly capable, but they suffer from **Amnesia and Context Drift**. 

When you close your laptop, restart your IDE, or run into a token limit, the AI session dies. When you restart it, you either:
1. Burn thousands of tokens re-feeding it the entire chat transcript.
2. Watch it hallucinate because it forgot its original constraints.
3. **Worst of all:** If *you* (the human) or *another AI tool* manually edit code while the AI is asleep, the AI wakes up totally oblivious to your changes, leading to massive merge conflicts and broken features.

While tools like LangGraph save state, and Devin runs in expensive persistent cloud VMs, **neither actively diffs the AI's brain against the physical repository to catch human or cross-agent interference.**

## The Solution

**Wake** is a local-first CLI tool and middleware that acts as a referee between your AI agent's "brain" and the actual repository. 

It anchors the AI's memory to the physical codebase using an event-sourced SQLite ledger. When a new AI session boots up, Wake diffs the repository against the AI's last memory and generates a highly condensed, **token-efficient ~150-token Recovery Packet**. This packet tells the AI exactly what it completed, what is blocked, and which files changed while it was asleep.

## What Wake Does Today

- **Multi-Tool & AI Handoff Support**: Run multiple agents (e.g., Aider, Claude Code, custom scripts) in the same workspace. Wake handles the state synchronization so they don't step on each other's toes.
- **AI-Interference Resilience**: If a human or another AI edits a file while the primary agent is offline, Wake detects the drift. It flags `[SAFE]`, `[STALE]`, or `[CONFLICT]` verdicts to ensure the agent is aware of the exact changes before it resumes coding.
- **Token-Efficient Recovery**: Instead of injecting thousands of lines of chat history to restart a session, Wake compiles the task context into a dense ~150-token recovery packet.
- **Git-Aware State Tracking**: Wake anchors state to your Git repository, tracking commits, branches, and working tree changes. Non-Git mode with internal file hashing is planned for v1.1.
- **Local-First SQLite Engine**: Tracks task objectives, completed milestones, and blockers locally. Uses WAL mode with serialized connection pooling for safe concurrent access by multiple tools.
- **Constraint Enforcement**: If you tell the AI "Do not modify auth.go", and you manually modify `auth.go` while it sleeps, Wake throws a hard `[CONFLICT]` to prevent the AI from overwriting your work.
- **Pre-Checkpoint Guard**: Blocks blind checkpoints when unreviewed modifications or untracked files exist in the working tree.
- **Feature Pivot Support**: Run `wake objective "New Goal"` to safely pivot the AI's memory without a full reset.
- **Universal MCP Server**: Built-in Model Context Protocol support (`wake mcp`). Seamlessly plug Wake into cloud-based AI agents (Claude Desktop, Cursor) or 100% local, free, air-gapped models (Ollama, LM Studio) for maximum privacy—with zero extra configuration.
## Quickstart

### Build from Source

Ensure you have Go 1.24+ installed:

```bash
git clone https://github.com/AshleyImmanuel/Wake.git
cd Wake
go mod tidy
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

### Integrate via MCP (No Clone Required)

If you just want to plug Wake into your AI client (like Claude Desktop, Cursor, or Antigravity) without cloning the source code, you can run it on-the-fly! Just add this to your client's MCP configuration (requires Go to be installed):

```json
{
  "mcpServers": {
    "wake": {
      "command": "go",
      "args": ["run", "github.com/AshleyImmanuel/Wake@latest", "mcp"]
    }
  }
}
```

### CLI Reference

| Command | Description |
|---------|-------------|
| `wake init` | Initialize a `.wake/` workspace in the current directory |
| `wake checkpoint` | Create a versioned state snapshot |
| `wake status` | Show reconciliation status (SAFE/STALE/CONFLICT) |
| `wake resume` | Generate a compact ~150 token recovery packet for a new AI session |
| `wake history` | View the event history of the active task |
| `wake objective "..."` | Pivot the task objective without resetting state |
| `wake mcp` | Start the MCP (Model Context Protocol) server for IDE integration |
| `wake setup <ide>` | Generate IDE configuration for Cursor, VS Code, or Claude Code |

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
| **LangGraph / Checkpointers** | Python state-graph execution | Wake adds physical file reconciliation to internal memory states |
| **Cursor / Aider** | IDE and codebase semantic search | Wake acts as a background "save state" layer to persist constraints across terminal reboots and handoffs |
| **Devin / Cloud Agents** | Autonomous execution in persistent cloud environments | Wake provides a local-first alternative for developers who want state-persistence on their local machine |

## Security & Compliance (DPDP Act Ready)

Wake is built for high-risk, commercial, and enterprise-grade environments with **Privacy by Design**:
- **0-Vulnerability Codebase**: Fully audited with `gosec` yielding zero vulnerabilities. Safe against path traversal, integer overflows, and command injections.
- **DPDP Act Compliant**: Wake is 100% local. It collects **zero telemetry**, makes **no external HTTP calls**, and strictly minimizes data collection to local UUIDs/Agent Names for tracking modifications. 
- **Absolute Local Sovereignty**: All state is persisted in a local SQLite ledger (`.wake/`). You maintain absolute control and erasure rights over your data.
- **Strict File Guards**: Enforces strict `0600`/`0750` permissions to prevent unauthorized tampering of internal ledger and config files.

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
[State]   [Git / FS] [SQLite]
(events,  (client,   (db: WAL mode,
 state,    parser)    migrations,
 guard,               indices)
 reconcile)
```

- `internal/events/`: 17 core Event types with deep cloning for thread safety
- `internal/state/`: Deterministic reducer that collapses the event log into a point-in-time snapshot with dynamic confidence scoring
- `internal/reconcile/`: Diffs the SQLite checkpoint against the file system to identify AI-interference. Stable and robust.
- `internal/guard/`: Pre-checkpoint validation that blocks blind checkpoints on dirty working trees
- `internal/service/`: Application facade that unifies all operations
- `internal/db/`: SQLite persistence with transactional migrations and composite indices

## Copyright & License

**Copyright (c) 2026 Ashley Immanuel. All Rights Reserved.**

This software is proprietary. You may not use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of the Software without express written permission. See the [LICENSE](LICENSE) file for more information.

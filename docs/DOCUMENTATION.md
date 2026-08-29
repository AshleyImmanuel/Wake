# Wake: Complete User Documentation

Welcome to the official documentation for **Wake**, the local-first state recovery and reconciliation engine for autonomous AI coding agents.

---

## 1. Core Philosophy

When humans code, they take breaks, switch context, and come back the next day. When they return, they use their brain to remember what they were doing, and they use Git to see what changed. 

AI agents do not have persistent brains. When an AI session ends, its memory dies. To solve this, developers historically force the AI to read thousands of lines of chat history to "remember" what it was doing. This is expensive, slow, and brittle.

**Wake solves this.** Wake acts as a synthetic hippocampus for your AI. It continuously saves the AI's internal state (goals, constraints, decisions) to a local SQLite database. When the AI wakes up, Wake diffs the AI's SQLite memory against the physical Git repository to ensure no human has secretly modified the code, and then injects a dense, 150-token **Recovery Packet** into the AI to instantly resume its task.

---

## 2. Installation

Wake ships as a single, cross-platform native binary with **zero runtime dependencies**.

**Install via NPM (Recommended -- All Platforms)**
```bash
npm install -g wake-engine
```
This auto-detects your OS (Windows, macOS, or Linux), downloads the correct pre-compiled Go binary from GitHub Releases, and places it in your system path. Node.js is only used during installation -- Wake itself runs as a native binary with zero overhead.

**Install via Direct Binary Download**

Download the latest release for your platform from the [GitHub Releases Page](https://github.com/AshleyImmanuel/Wake/releases).

**Build from Source**
```bash
git clone https://github.com/AshleyImmanuel/Wake.git
cd Wake
go build -o wake .
```

**Verify Installation**
```bash
wake --help
```

---

## 3. Core Concepts

### The Event Ledger (`.wake/state.db`)
Wake does not save raw conversation logs. Instead, it uses **Event Sourcing**. Every time the AI makes a decision or completes a milestone, a JSON event is logged to the SQLite database (e.g., `TASK_STARTED`, `CONSTRAINT_ADDED`, `FILE_CHANGED`).

### The Reducer
When you run a command, Wake takes the hundreds of raw events in the ledger and "reduces" them into a single, cohesive **Point-in-Time State Snapshot**. 

### The Reconciliation Engine
This is the heart of Wake. Before the AI resumes, Wake compares the Point-in-Time State against the live Git repository. It outputs one of three states:
1. `[SAFE]`: The repository exactly matches the AI's memory. Safe to resume.
2. `[STALE]`: The repository has drifted (e.g., a human added a new file), but no AI constraints were violated. 
3. `[CONFLICT]`: A human modified a file that the AI explicitly marked as "Do not touch", or deleted a file the AI needed. Manual review is required.

---

## 4. CLI Reference

### `wake checkpoint`
Forces Wake to evaluate the current event ledger, bundle the current repository Git hash, and save a concrete Point-in-Time checkpoint to the database.
- **Usage:** `wake checkpoint [--objective "Optional new objective"]`
- **When to use:** Run this automatically after every major file edit, or at the end of a coding session.

### `wake status`
Evaluates the latest checkpoint against the current physical Git directory and outputs a Reconciliation Report.
- **Usage:** `wake status`
- **Output:** Returns `SAFE`, `STALE`, or `CONFLICT` with a list of modified files.

### `wake resume`
Generates the highly-condensed **Recovery Packet** designed to be fed directly into an AI agent's system prompt to wake it up.
- **Usage:** `wake resume`
- **Behavior:** Automatically runs `wake status` internally. If the state is a `[CONFLICT]`, it injects a critical warning into the packet.

### `wake history`
Dumps the raw, un-reduced JSON event ledger. Useful for debugging exactly what the AI was thinking.
- **Usage:** `wake history`

### `wake objective`
Manually overrides the core objective of the task. 
- **Usage:** `wake objective "Pivot: Build PayPal instead of Stripe"`
- **When to use:** Use this if the business requirements change mid-project and you want the AI to pivot without losing its memory of the codebase.

### `wake check-conflict`
Optimistic concurrency check that prevents an AI from silently overwriting recent human edits.
- **Usage:** `wake check-conflict --file <path>`
- **Behavior:** If the file was modified by a human in the last 10 seconds, returns `{"decision": "deny"}` and blocks the AI's write tool. Otherwise returns `{"decision": "allow"}`.

### `wake mark`
Records author attribution for a file modification.
- **Usage:** `wake mark --file <path> --author <AI|HUMAN>`
- **Behavior:** Appends a timestamped entry to `.wake/attribution.log`, allowing the reconciliation engine to distinguish between AI and human edits.

### `wake setup`
Auto-generates IDE MCP configurations and lifecycle hooks.
- **Usage:** `wake setup [--cursor] [--vscode] [--claude] [--antigravity]` (Run with no flags for all).
- **Behavior:** Generates the MCP connection JSON for the specified IDEs. For **Cursor** and **Antigravity**, it also auto-generates native `hooks.json` files that run PreToolUse conflict checks, PostToolUse attribution markers, and auto-checkpointing.

---

## 5. Integrating with AI Agents

Wake is designed to be agent-agnostic. It can be wrapped around Aider, Claude Code, Antigravity, or custom Python agents.

### The Lifecycle Pattern
To integrate Wake into your agent, implement this exact lifecycle:

1. **On Boot (Pre-Invocation):**
   Execute `wake resume`. Capture the stdout. Inject this text directly as the very first System Message in the AI's context window.
   
2. **On Tool Execution (Post-Action):**
   Every time the AI executes a `WriteFile` or `RunCommand` tool, execute `wake checkpoint` in the background.

### 4. Zero-Config Embedded File Watcher
Wake's `mcp` server natively embeds a lightweight, debounced file watcher. When an IDE connects to the MCP server, this background goroutine automatically runs. **If your IDE does not support native hooks** (like VS Code or Windsurf), the daemon automatically watches for file modifications. When you or the AI edit files and go idle for a few seconds, the watcher automatically synthesizes a background checkpoint.

### 5. Native IDE Hooks (Cursor & Antigravity)
If your IDE supports native lifecycle hooks (such as Cursor or Antigravity), Wake can intercept tool usage *before* and *after* execution. 

Run `wake setup --cursor` or `wake setup --antigravity` to generate these hooks automatically. For example, the Antigravity hook looks like this:

```json
{
  "wake-autosave": {
    "PreToolUse": [
      {
        "matcher": "write_to_file|replace_file_content|multi_replace_file_content",
        "hooks": [
          {
            "type": "command",
            "command": "wake check-conflict"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "write_to_file|replace_file_content|multi_replace_file_content",
        "hooks": [
          {
            "type": "command",
            "command": "wake mark --author AI"
          },
          {
            "type": "command",
            "command": "wake checkpoint --objective 'Antigravity Auto-Save'"
          }
        ]
      }
    ]
  },
  "wake-resume": {
    "PreInvocation": [
      {
        "type": "command",
        "command": "wake resume > .agents/last_resume.txt"
      }
    ]
  }
}
```

---

## 6. v1.1 Features

### Continuous Recovery Stashing
Wake's background daemon proactively stashes human-modified files into `.wake/recovery_stash/` before an AI can overwrite them. This is 100% IDE-agnostic and runs as part of the `wake mcp` daemon.

### Git-less File Hashing Fallback
Wake does not require Git. If run outside a Git repository, it seamlessly falls back to a custom `hashfs` engine that computes SHA-256 hashes of all files and stores them in `.wake/hash_index.json` to track state changes.

### Write-Write Conflict Detection
Optimistic concurrency controls detect if a human and AI edit the same file simultaneously. If a human modified a file less than 10 seconds ago, Wake blocks the AI's write tool to prevent silent data destruction.

### Author Attribution Markers
Every AI file modification is logged to `.wake/attribution.log` with a timestamp, allowing the reconciliation engine to precisely differentiate between `AI_MODIFIED` and `HUMAN_MODIFIED` code.

---

## 7. Advanced Configuration

### SQLite WAL Mode
Wake is compiled with SQLite `WAL` (Write-Ahead Logging) enabled by default. This ensures that multiple concurrent AI agents (e.g., a Frontend Agent and a Backend Agent) can read and write to `.wake/state.db` simultaneously without encountering locking errors. 

### Custom Task IDs
By default, Wake assumes there is one active task per directory. If you are running multiple isolated tasks in the same repository, you can segment them using UUIDs:
```bash
wake checkpoint --task-id "123e4567-e89b-12d3-a456-426614174000"
wake status --task-id "123e4567-e89b-12d3-a456-426614174000"
```

---
*Generated for Wake v1.1.0.*

# Wake: Complete User Documentation

Welcome to the official documentation for **Wake**, the local-first state recovery and reconciliation engine for autonomous AI coding agents.

---

## 1. Core Philosophy

When humans code, they take breaks, switch context, and come back the next day. When they return, they use their brain to remember what they were doing, and they use Git to see what changed. 

AI agents do not have persistent brains. When an AI session ends, its memory dies. To solve this, developers historically force the AI to read thousands of lines of chat history to "remember" what it was doing. This is expensive, slow, and brittle.

**Wake solves this.** Wake acts as a synthetic hippocampus for your AI. It continuously saves the AI's internal state (goals, constraints, decisions) to a local SQLite database. When the AI wakes up, Wake diffs the AI's SQLite memory against the physical Git repository to ensure no human has secretly modified the code, and then injects a dense, 150-token **Recovery Packet** into the AI to instantly resume its task.

---

## 2. Installation

Wake is distributed as a single, cross-platform binary.

**Prerequisites:**
- Go 1.27+
- Git (must be installed and available in your `$PATH`)

**Install via Go:**
```bash
go install github.com/AshleyImmanuel/Wake@latest
```

Ensure your `~/go/bin` directory is in your system `$PATH`. You can verify the installation by running:
```bash
wake --version
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
Wake's `mcp` server natively embeds a lightweight, debounced file watcher. When an IDE (Cursor, Claude Desktop, Antigravity) connects to the MCP server, this background goroutine automatically runs. If you (or an AI without hooks) modify files and then go idle for a few seconds, the daemon automatically synthesizes a background checkpoint. 

### Native Antigravity Integration
If you are using the Antigravity IDE, you do not need to write code or configure hooks manually. Simply run:

```bash
wake setup --antigravity
```

This will auto-generate `.agents/mcp_config.json` to register the MCP server, and an `.agents/hooks.json` file which forces the IDE to auto-checkpoint upon every file write. The hook looks like this:

```json
{
  "wake-autosave": {
    "PostToolUse": [
      {
        "matcher": "write_to_file|replace_file_content",
        "hooks": [
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

## 6. Advanced Configuration

### SQLite WAL Mode
Wake is compiled with SQLite `WAL` (Write-Ahead Logging) enabled by default. This ensures that multiple concurrent AI agents (e.g., a Frontend Agent and a Backend Agent) can read and write to `.wake/state.db` simultaneously without encountering locking errors. 

### Custom Task IDs
By default, Wake assumes there is one active task per directory. If you are running multiple isolated tasks in the same repository, you can segment them using UUIDs:
```bash
wake checkpoint --task-id "123e4567-e89b-12d3-a456-426614174000"
wake status --task-id "123e4567-e89b-12d3-a456-426614174000"
```

---
*Generated for Wake v1.0 Beta.*

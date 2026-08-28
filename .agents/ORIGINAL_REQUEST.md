# Original User Request

## Initial Request — 2026-08-28T16:52:54Z

# Teamwork Project Prompt — Draft

> Status: Launched
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: [none — teamwork routes from the description]

Implement Phase 2 (Reconciliation) of the Sentinel MVP. This involves building the Git comparison utilities and a reconciliation engine that checks a saved task state checkpoint against the current live Git repository to determine if the state is SAFE, STALE, or CONFLICT.

Working directory: C:/Users/USER/Desktop/Sentinel
Integrity mode: development

## Requirements

### R1. Git CLI Wrapper
Build a utility layer that shells out to the local `git` binary to retrieve current repository information. It must be able to retrieve the current commit hash, list modified files, and list uncommitted changes.

### R2. Reconciliation Engine
Implement a comparison function that takes a saved `Checkpoint` object (from the SQLite database layer) and the current Git repository state. It must evaluate the differences and return a status of SAFE, STALE, or CONFLICT.

## Acceptance Criteria

### Verification Suite
- [ ] A Go test suite exists that uses a temporary Git repository to simulate SAFE, STALE, and CONFLICT scenarios.
- [ ] The test suite runs automatically via `go test` and passes without human intervention.
- [ ] The reconciliation engine correctly returns SAFE when the simulated repository exactly matches the checkpoint commit with no uncommitted changes.
- [ ] The reconciliation engine correctly returns CONFLICT or STALE when simulated task-related files have been manually modified since the checkpoint.

## Follow-up — 2026-08-28T20:12:45Z

# Teamwork Project Prompt — Draft

> Status: Launched
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: [none — teamwork routes from the description]

Perform a comprehensive codebase review of the Wake project. Modularize the existing code, optimize the core logic (events, state reduction, and reconciliation), and significantly expand the test coverage with a large suite of unit and integration tests.

Working directory: C:/Users/USER/Desktop/Sentinel
Integrity mode: development

## Requirements

### R1. Codebase Review & Optimization
Analyze all packages (`internal/state`, `internal/git`, `internal/reconcile`, `internal/db`, `cmd/`). Optimize the state reduction and reconciliation logic for performance and readability.

### R2. Modularization
Refactor tightly coupled code into distinct, single-responsibility modules. Ensure the interfaces between the CLI, the State Engine, and the SQLite Database are clean.

### R3. Comprehensive Testing
Write extensive Go tests covering standard execution paths, edge cases, and error handling across all packages.

## Acceptance Criteria

### Verification Suite
- [ ] A comprehensive test suite exists and is verifiable by running `go test -v ./...`.
- [ ] The test suite passes 100% without human intervention.
- [ ] `go vet ./...` runs without outputting any warnings, proving structural soundness.
- [ ] Core behavior (creating Checkpoints and identifying SAFE/STALE/CONFLICT states) remains fully intact and proven by tests.

## Follow-up — 2026-08-28T21:03:18Z

# Teamwork Project Prompt — Draft

> Status: Launched
> Goal: Craft prompt → get user approval → delegate to teamwork_preview
> Requested team: Full team

Conduct a comprehensive security and bug audit of the entire Wake codebase. Review all Go source files, identify potential vulnerabilities or functional bugs, and directly implement fixes for any definitive issues found.

Working directory: C:/Users/USER/Desktop/Wake
Integrity mode: development

## Requirements

### R1. Security & Bug Audit
Thoroughly review all Go code in the repository to identify security vulnerabilities (e.g., path traversal, injection, database locking, race conditions) and functional bugs.

### R2. Direct Remediation
Directly implement patches for all definitive security issues and bugs discovered during the audit without altering the core CLI workflow or feature set.

### R3. Token Optimization
The agent team must activate and use the `caveman` skill to heavily compress their communication and output tokens throughout the duration of this massive audit.

### R4. Universal IDE & MCP Integration
Design and implement integration wrappers (such as an official MCP server, generic IDE plugins, or lifecycle hooks) so that developers can seamlessly use Wake inside any modern IDE (Cursor, VS Code, JetBrains) or with any MCP-compatible agent in their comfort zone.

## Acceptance Criteria

### Security Verification
- [ ] An independent auditing agent has reviewed the implemented fixes against a strict security rubric and confirmed that the vulnerabilities are resolved and no new regressions were introduced.

### Integration Verification
- [ ] A functioning MCP (Model Context Protocol) server for Wake is implemented and successfully exposes the Wake API to external IDEs/agents.

## Follow-up — 2026-08-28T21:35:00Z

[URGENT ZERO-DAY DISCOVERY from User]: The user just exploited a critical architectural vulnerability in the Wake wrapper. 

Vulnerability: If a human modifies a file in the working tree *while* the AI is running a tool, and the AI subsequently runs `wake checkpoint`, Wake currently accepts the checkpoint and locks the human's unreviewed code into the baseline memory. The wrapper fails to protect the state from the AI's own blind commits.

Directive: You must immediately implement a "Pre-Checkpoint Guard" in the core Go engine. When `wake checkpoint` is called, it MUST verify that no un-tracked or human-modified files are being blindly scooped into the checkpoint. If it detects unreviewed changes, it must return a fatal error and refuse to checkpoint. Implement this as part of Milestone 2.



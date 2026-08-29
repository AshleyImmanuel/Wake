# Project: Wake Security & Bug Audit, Remediation, and Universal IDE / MCP Integration

## Architecture
Wake is an autonomous context checkpointing, repository state reconciliation, and universal IDE/agent integration platform. It records fine-grained development activity into an append-only event ledger, calculates deterministic task state via a 17-event reduction engine, persists snapshots into an indexed SQLite store, reconciles repository state against live Git trees, protects repository state via a Pre-Checkpoint Guard, and exposes all capabilities through both a CLI and an official Model Context Protocol (MCP) JSON-RPC 2.0 stdio server.

```
[External IDEs & AI Agents]
(Cursor, VS Code, JetBrains, Claude Desktop, Antigravity)
          │
          │ stdio (JSON-RPC 2.0)
          ▼
[Official MCP Server] ◄──────────────► [CLI Presentation Layer]
(cmd/mcp, internal/mcp)                 (cmd/: checkpoint, status, resume, history)
          │                                      │
          └──────────────────┬───────────────────┘
                             ▼
               [Application Service Facade]
               (internal/service: TaskService)
                             │
       ┌─────────────────────┼─────────────────────┐
       ▼                     ▼                     ▼
[State & Event Engine]  [Git Wrapper]      [Persistence Store]
(internal/state, events)(internal/git)      (internal/db Store)
       │                     │
       └──────────┬──────────┘
                  ▼
       [Reconciliation Engine & Pre-Checkpoint Guard]
       (internal/reconcile, internal/guard)
                  │
                  ▼
       [Unified Test & E2E Harness]
       (internal/testutil, e2e)
```

## Feature & Vulnerability Inventory
| # | Item | Category | Description | Milestone | Source |
|---|------|----------|-------------|-----------|--------|
| 1 | Test Harness & Build Fixes | Build / Test | Fix `e2e/harness_test.go` pointer return/dereference errors and `testutil_test.go` path mismatch (`.wake` vs `.sentinel`) | M1 | Survey (SEC-14, BUG-08) [DONE] |
| 2 | Path Traversal & Safe Path Validation | Security | Prevent path traversal in `internal/reconcile/engine.go` via `filepath.Rel` containment and rejecting escape paths | M1 | Survey (SEC-01) [DONE] |
| 3 | False CONFLICT & Wildcard Sanity | Bug / Logic | Prevent DoS false CONFLICTs on wildcards (`*.go`), version numbers (`v2.0`), step counters, URLs in `internal/reconcile` | M1 | Survey (SEC-02, BUG-03) [DONE] |
| 4 | Git CLI Flag Injection & Ref Validation | Security | Validate git refs against `^[a-zA-Z0-9_\.\/\-]+$`, disallow leading `-`, append `--` to git diff commands in `internal/git/client.go` | M1 | Survey (SEC-05, SEC-06) [DONE] |
| 5 | Git Status Rename & Octal Parser Fixes | Bug / Logic | Restrict `parseRenamePath` strictly to rename/copy statuses; parse unescaped raw-byte octal/C-quoted UTF-8 paths; fix whitespace handling in `internal/git/parser.go` | M1 | Survey (BUG-04) [DONE] |
| 6 | SQLite Concurrency & Pool Locking | Security / Concurrency | Set `MaxOpenConns(1)` / `MaxIdleConns(1)` to eliminate `SQLITE_BUSY` errors under WAL mode in `internal/db/db.go` | M2 | Survey (SEC-07) |
| 7 | Transactional Migrations & Composite Indices | Database / Performance | Execute migrations in atomic transactions; add composite indices `(task_id, timestamp)` on `events` and `checkpoints` tables | M2 | Survey (SEC-08, SEC-09) |
| 8 | Checkpoint State Version Uniqueness | Concurrency / Integrity | Add `UNIQUE(task_id, state_version)` constraint to prevent state version collisions | M2 | Survey (SEC-10) |
| 9 | SQLite Deserialization Error Handling | Reliability / Integrity | Propagate explicit errors on `uuid.Parse` and `time.Parse` failure instead of swallowing errors in `internal/db/db.go` | M2 | Survey (SEC-12, BUG-07) |
| 10 | 17-Event Type State Reducer | Logic / Completeness | Full event folding across all 17 event types; populate `State.TaskID`; compute dynamic confidence in `internal/state/engine.go` | M2 | Survey (BUG-01, BUG-02) |
| 11 | Thread-Safe Event Payloads | Concurrency / Safety | Prevent concurrent map access race conditions on `Event.Payload` in `internal/events` and `internal/state` | M2 | Survey (SEC-11) |
| 12 | Reconciler Token Matching & Ancestry Error Handling | Logic / Reliability | Refine constraint matching to avoid single-word path false positives; propagate git ancestry check errors in `internal/reconcile` | M2 | Survey (BUG-05, BUG-06) |
| 13 | Pre-Checkpoint Human-Modification Guard | Safety / Core Engine | Enforce pre-checkpoint validation to verify no untracked or human-modified files are blindly scooped into checkpoints; return fatal error if unreviewed changes detected | M2 | User Urgent Directive |
| 14 | Application Service Facade | Architecture | Implement `internal/service.TaskService` unifying checkpoint, status, history, and resume operations across CLI and MCP | M3 | Survey (MCP-07) |
| 15 | CLI Context & Formatting Fixes | Bug / CLI | Propagate `cmd.Context()` for clean cancellation; fix `cmd/resume.go:121` branch output format; decouple CLI to thin service delegates | M3 | Survey (SEC-13, BUG-09) |
| 16 | Official MCP JSON-RPC 2.0 Stdio Server | MCP Integration | Pure Go zero-dependency JSON-RPC 2.0 stdio server implementing 2024-11-05 MCP specification in `internal/mcp` and `cmd/mcp.go` | M4 | Survey (MCP-01, MCP-02) |
| 17 | 7 Standard MCP Tools | MCP Integration | Implement `wake_checkpoint`, `wake_status`, `wake_resume`, `wake_history`, `wake_update_objective`, `wake_record_event`, `wake_init` | M4 | Survey (MCP-03) |
| 18 | 4 MCP Resources & 3 MCP Prompts | MCP Integration | Implement state/event resources and AI workflow prompts (`wake_session_start`, `wake_pre_commit_audit`, `wake_conflict_resolution`) | M4 | Survey (MCP-04, MCP-05) |
| 19 | Universal IDE Configs & Lifecycle Hooks | IDE Integration | Generate configurations for Cursor (`.cursor/mcp.json`, `.cursorrules`), VS Code (`.vscode/mcp.json`), Claude/JetBrains, and Antigravity hooks | M4 | Survey (MCP-06) |
| 20 | Comprehensive Verification & Soundness Suite | Verification | 100% test pass on `go test -v ./...`, `go vet ./...` 0 warnings, and MCP stdio integration tests | M5 | Original Request & Acceptance Criteria |
| 21 | Independent Forensic Security & Integrity Audit | Security Audit | Forensic integrity review by `teamwork_preview_auditor` against strict security rubric | M6 | Acceptance Criteria |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Harness Fixes, Security Path & Git Hardening | Fix test compilation; remediate path traversal (SEC-01, SEC-02, BUG-03); remediate git flag injection & parser bugs (SEC-05, SEC-06, BUG-04); raw-byte octal UTF-8 unescaping | none | DONE |
| M2 | SQLite Concurrency, Data Integrity, State Engine & Pre-Checkpoint Guard | Remediate SQLite connection locks (SEC-07), migrations & indexes (SEC-08, SEC-09), version uniqueness (SEC-10), error propagation (SEC-12); complete 17-event reducer & dynamic confidence (BUG-01, BUG-02, SEC-11); optimize reconciler matching (BUG-05, BUG-06); implement Pre-Checkpoint Guard | M1 | DONE |
| M3 | Application Service Facade & CLI Decoupling | Implement `internal/service.TaskService`; fix CLI context cancellation (SEC-13) and formatting bugs (BUG-09); refactor `cmd/` | M2 | DONE |
| M4 | Universal IDE & Official MCP Server Integration | Implement `internal/mcp` and `cmd/mcp.go`; 7 MCP tools, 4 resources, 3 prompts; IDE configs (Cursor, VS Code, JetBrains, Claude, hooks); unit & protocol tests | M3 | PLANNED |
| M5 | Comprehensive E2E, MCP Integration & Adversarial Verification | Full test suite validation (`go test -v ./...`, `go vet ./...`); MCP stdio subprocess tests; Tier 5 adversarial tests | M4 | PLANNED |
| M6 | Independent Forensic Security & Integrity Audit | Strict security rubric audit by `teamwork_preview_auditor` (zero tolerance, binary veto) | M5 | PLANNED |

## Interface Contracts

### `internal/events` ↔ `internal/state`
```go
package events

type EventType string

const (
    TaskStarted        EventType = "TASK_STARTED"
    RequirementAdded   EventType = "REQUIREMENT_ADDED"
    ConstraintAdded    EventType = "CONSTRAINT_ADDED"
    UserApproval       EventType = "USER_APPROVAL"
    UserRejection      EventType = "USER_REJECTION"
    DecisionMade       EventType = "DECISION_MADE"
    FileChanged        EventType = "FILE_CHANGED"
    CommandExecuted    EventType = "COMMAND_EXECUTED"
    TestStarted        EventType = "TEST_STARTED"
    TestPassed         EventType = "TEST_PASSED"
    TestFailed         EventType = "TEST_FAILED"
    BlockerCreated     EventType = "BLOCKER_CREATED"
    BlockerResolved    EventType = "BLOCKER_RESOLVED"
    MilestoneCompleted EventType = "MILESTONE_COMPLETED"
    GitCommit          EventType = "GIT_COMMIT"
    SessionInterrupted EventType = "SESSION_INTERRUPTED"
    SessionResumed     EventType = "SESSION_RESUMED"
)

type Event struct {
    ID        uuid.UUID              `json:"id"`
    TaskID    uuid.UUID              `json:"task_id"`
    Type      EventType              `json:"type"`
    Timestamp time.Time              `json:"timestamp"`
    Payload   map[string]interface{} `json:"payload"`
}
```

### `internal/service` (Application Service & Pre-Checkpoint Guard)
```go
package service

type TaskService interface {
    CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
    GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
    GetHistory(ctx context.Context, taskID string, limit int) ([]events.Event, error)
    ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
    UpdateObjective(ctx context.Context, taskID string, objective string) error
    RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error)
    InitWorkspace(ctx context.Context, dir string) error
}

type CheckpointRequest struct {
    TaskID       string
    Objective    string
    Dir          string
    TrackedFiles []string
    Force        bool
}
```

## Code Layout
```
C:/Users/USER/Desktop/Sentinel/
├── main.go
├── go.mod
├── go.sum
├── PROJECT.md
├── TEST_INFRA.md
├── cmd/
│   ├── root.go
│   ├── checkpoint.go
│   ├── checkpoint_test.go
│   ├── status.go
│   ├── status_test.go
│   ├── history.go
│   ├── history_test.go
│   ├── objective.go
│   ├── objective_test.go
│   ├── resume.go
│   ├── resume_test.go
│   ├── mcp.go
│   └── mcp_test.go
├── internal/
│   ├── events/
│   │   ├── models.go
│   │   └── models_test.go
│   ├── state/
│   │   ├── models.go
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── db/
│   │   ├── db.go
│   │   ├── store.go
│   │   └── db_test.go
│   ├── git/
│   │   ├── models.go
│   │   ├── runner.go
│   │   ├── parser.go
│   │   ├── parser_test.go
│   │   ├── client.go
│   │   ├── client_test.go
│   │   ├── errors.go
│   │   └── adversarial_test.go
│   ├── reconcile/
│   │   ├── models.go
│   │   ├── engine.go
│   │   └── engine_test.go
│   ├── service/
│   │   ├── service.go
│   │   ├── models.go
│   │   └── service_test.go
│   ├── mcp/
│   │   ├── protocol.go
│   │   ├── server.go
│   │   ├── tools.go
│   │   ├── resources.go
│   │   ├── prompts.go
│   │   ├── server_test.go
│   │   └── tools_test.go
│   └── testutil/
│       ├── git.go
│       ├── db.go
│       ├── fixtures.go
│       └── testutil_test.go
└── e2e/
    ├── harness_test.go
    ├── mcp_test.go
    └── e2e_test.go
```

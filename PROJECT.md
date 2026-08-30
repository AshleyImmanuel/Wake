# Project: Wake Codebase Review, Modularization, Optimization, and Comprehensive Testing

## Architecture
Wake is an autonomous context checkpointing and repository state reconciliation platform. It tracks development tasks via an append-only event stream, computes current task state via deterministic state reduction, persists snapshots into SQLite, inspects live Git repository status, and reconciles saved checkpoints against the working tree to determine SAFE, STALE, or CONFLICT states.

```
[CLI / Presentation Layer]
(cmd/: root, checkpoint, status, history, resume)
          │
          ▼
[Application Service Facade]
(internal/service: TaskService, CheckpointService, StatusService, HistoryService, ResumeService)
          │
    ┌─────┴──────────────────┬─────────────────┐
    ▼                        ▼                 ▼
[State & Event Engine]  [Git Wrapper]   [Persistence Store]
(internal/state, events)(internal/git)   (internal/db Store)
    │                        │
    └───────────┬────────────┘
                ▼
      [Reconciliation Engine]
      (internal/reconcile)
                │
                ▼
      [Unified Test Harness]
      (internal/testutil)
```

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Shared Test Harness & Fixture Utility | Unified Git repository simulator and SQLite test fixture (`internal/testutil`) | M1 | Survey & Test Gap |
| 2 | UTF-8 Byte Sort Order Fix | Fix inverted UTF-8 string sort order in `internal/git/adversarial_test.go:206` | M1 | Survey & Test Fix |
| 3 | Strongly-Typed Event Payloads | Concrete payload structs, constructors, and JSON serialization in `internal/events` | M2 | Survey & Modularization |
| 4 | Comprehensive 17-Event State Reducer | Full event reduction across all 17 event types, task ID parsing, and dynamic confidence | M2 | Survey & Logic Optimization |
| 5 | Incremental Event Folding | Efficient folding function (`Fold`) for delta state evaluation | M2 | Survey & Optimization |
| 6 | Database Store Abstraction & PRAGMAs | `Store` interface, WAL journal mode, busy timeout, and composite B-tree indexes in `internal/db` | M3 | Survey & Modularization |
| 7 | Atomic Multi-Entity Transactions | Transactional boundaries (`WithTx`) for multi-entity event and checkpoint persistence | M3 | Survey & DB Integrity |
| 8 | Git Invocations Consolidation | Optimized branch and status extraction via `--porcelain=v1 --branch` | M4 | Survey & Performance |
| 9 | Reconciler Delimiter & Matching Optimization | Precompiled token regex, `strings.FieldsFunc`, and indexed membership matching | M4 | Survey & Performance |
| 10 | Filesystem Check Decoupling in Reconciler | Abstract file existence check interface in `internal/reconcile` | M4 | Survey & Modularization |
| 11 | Application Service Facade | `internal/service` package implementing `TaskService` interface for all core workflows | M5 | Survey & Architecture |
| 12 | CLI Decoupling & Expanded Command Coverage | Refactor `cmd/` to thin delegates; implement comprehensive `cmd/history_test.go` and `cmd/resume_test.go` | M5 | Survey & Coverage |
| 13 | Comprehensive Verification Suite | 100% passing tests across `go test -v ./...` and 0 warnings on `go vet ./...` | M6 | Original Request Criteria |
| 14 | Adversarial Coverage Hardening (Tier 5) | White-box stress tests, concurrency torture tests, and edge case resilience | M6 | E2E Hardening |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Test Infrastructure & Shared Harness | `internal/testutil`, fix `internal/git/adversarial_test.go` | none | IN_PROGRESS |
| M2 | Core Events & State Engine Optimization | `internal/events`, `internal/state`, full event reduction & typing | M1 | PLANNED |
| M3 | Database Store Modularization & Indexing | `internal/db`, `Store` interface, WAL mode, indexes, transactions | M1 | PLANNED |
| M4 | Git & Reconciler Optimization & Decoupling | `internal/git`, `internal/reconcile`, regex & status optimization | M1, M2 | PLANNED |
| M5 | Application Service Facade & CLI Testing | `internal/service`, refactor `cmd/`, add `history_test.go`, `resume_test.go` | M2, M3, M4 | PLANNED |
| M6 | Final E2E Verification & Adversarial Hardening | E2E suite validation, `go test -v ./...`, `go vet ./...`, Tier 5 stress | M5 | PLANNED |

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

### `internal/db` (Store Interface)
```go
package db

type Store interface {
    Init() error
    SaveCheckpoint(ctx context.Context, cp state.Checkpoint) error
    GetLatestCheckpoint(ctx context.Context, taskID string) (*state.Checkpoint, error)
    SaveEvent(ctx context.Context, e events.Event) error
    GetEvents(ctx context.Context, taskID string) ([]events.Event, error)
    WithTx(ctx context.Context, fn func(tx Store) error) error
    Close() error
}
```

### `internal/service` (Application Service)
```go
package service

type TaskService interface {
    CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
    GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
    GetHistory(ctx context.Context, taskID string) ([]events.Event, error)
    ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
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
│   ├── resume.go
│   └── resume_test.go
└── internal/
    ├── events/
    │   ├── models.go
    │   └── models_test.go
    ├── state/
    │   ├── models.go
    │   ├── engine.go
    │   └── engine_test.go
    ├── db/
    │   ├── db.go
    │   ├── store.go
    │   └── db_test.go
    ├── git/
    │   ├── models.go
    │   ├── runner.go
    │   ├── parser.go
    │   ├── parser_test.go
    │   ├── client.go
    │   ├── client_test.go
    │   ├── errors.go
    │   ├── adversarial_test.go
    │   └── lifecycle_adversarial_test.go
    ├── reconcile/
    │   ├── models.go
    │   ├── engine.go
    │   ├── engine_test.go
    │   └── reconcile_test.go
    ├── service/
    │   ├── service.go
    │   └── service_test.go
    └── testutil/
        ├── git.go
        ├── db.go
        └── fixtures.go
```

# Comprehensive Codebase Survey Report: Project Wake (Sentinel)

## 1. Observation

### 1.1 Project Metadata, Go Module & Dependencies
- **Workspace Location**: `C:/Users/USER/Desktop/Sentinel`
- **Go Module Name**: `github.com/wake/wake` (`go.mod:1`)
- **Go Target Version**: `go 1.27.0` (`go.mod:3`)
- **Direct Dependencies** (`go.mod:5-9`):
  - `github.com/google/uuid v1.6.0`
  - `github.com/spf13/cobra v1.10.2`
  - `modernc.org/sqlite v1.57.0` (pure Go SQLite driver, CGO-free)
- **Indirect Dependencies** (`go.mod:11-22`):
  - `github.com/dustin/go-humanize v1.0.1`
  - `github.com/inconshreveable/mousetrap v1.1.0`
  - `github.com/mattn/go-isatty v0.0.24`
  - `github.com/ncruces/go-strftime v1.0.0`
  - `github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec`
  - `github.com/spf13/pflag v1.0.10`
  - `golang.org/x/sys v0.47.0`
  - `modernc.org/libc v1.74.4`
  - `modernc.org/mathutil v1.7.1`
  - `modernc.org/memory v1.11.0`
- **Root Files**:
  - `main.go` (8 lines): calls `cmd.Execute()`
  - `PROJECT.md` (187 lines): architecture, milestones M1-M3, interface contracts
  - `Project Sentinel.md` (1261 lines): Product Requirements Document (PRD v0.3)
  - `ORIGINAL_REQUEST.md` (63 lines): prompt requirements and acceptance criteria
  - Precompiled binaries: `sentinel.exe`, `wake.exe`

### 1.2 Package Layout and File Catalog
```
C:/Users/USER/Desktop/Sentinel/
├── main.go
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go             # Root Cobra command "wake"
│   ├── checkpoint.go       # Checkpoint creation command & pipeline
│   ├── checkpoint_test.go  # Unit tests for checkpoint command
│   ├── status.go           # Status evaluation & terminal/JSON rendering
│   ├── status_test.go      # Unit tests for status command
│   ├── history.go          # Event history display command
│   └── resume.go           # Agent recovery packet generator command
└── internal/
    ├── events/
    │   └── models.go       # EventType constants, Event struct, NewEvent helper
    ├── state/
    │   ├── models.go       # State, Decision, Blocker, Checkpoint structs, ConfidenceLevel
    │   ├── engine.go       # Reduce function for event reduction
    │   └── engine_test.go  # Unit tests for state reduction
    ├── git/
    │   ├── models.go       # StatusCode, FileStatus, StatusResult, RepositoryState, FileChange
    │   ├── runner.go       # Runner interface, OSRunner, MockRunner
    │   ├── parser.go       # Porcelain v1 status parser, diff parsers
    │   ├── parser_test.go  # Unit tests for status and diff parsers
    │   ├── client.go       # Client interface, client struct implementation
    │   ├── client_test.go  # Mock and live integration tests for Client
    │   ├── errors.go       # GitError struct, domain error sentinels, classifyGitError
    │   ├── adversarial_test.go           # Adversarial test matrix for git client & parser
    │   └── lifecycle_adversarial_test.go # Live repository lifecycle integration tests
    ├── reconcile/
    │   ├── models.go       # ReconciliationStatus enum, ReconciliationResult struct
    │   ├── engine.go       # Engine interface, Reconcile pure function, ReconcileRepo helper
    │   ├── engine_test.go  # Unit tests for reconciliation logic & mock git client
    │   └── reconcile_test.go # Autonomous verification suite using temporary git repos
    └── db/
        ├── db.go           # InitDB, SQLite migrations, SaveCheckpoint, GetLatestCheckpoint, SaveEvent, GetEvents
        └── db_test.go      # Unit tests for SQLite database operations
```

### 1.3 Detailed Package Analysis: Interfaces, Structs, Models, and Functions

#### A. Package `internal/events` (`internal/events/models.go`)
- **Event Types** (`lines 11-29`):
  `TaskStarted`, `RequirementAdded`, `ConstraintAdded`, `UserApproval`, `UserRejection`, `DecisionMade`, `FileChanged`, `CommandExecuted`, `TestStarted`, `TestPassed`, `TestFailed`, `BlockerCreated`, `BlockerResolved`, `MilestoneCompleted`, `GitCommit`, `SessionInterrupted`, `SessionResumed`.
- **Structs**:
  ```go
  type Event struct {
      ID        uuid.UUID              `json:"id"`
      TaskID    uuid.UUID              `json:"task_id"`
      Type      EventType              `json:"type"`
      Timestamp time.Time              `json:"timestamp"`
      Payload   map[string]interface{} `json:"payload"`
  }
  ```
- **Functions**: `NewEvent(taskID uuid.UUID, eventType EventType, payload map[string]interface{}) Event`

#### B. Package `internal/state` (`internal/state/models.go`, `internal/state/engine.go`)
- **Confidence Level**: `ConfidenceHigh`, `ConfidenceLow`, `ConfidenceNone`
- **Evidence Status**: `Verified`, `UserConfirmed`, `AgentInferred`, `Unknown`
- **Structs**:
  ```go
  type State struct {
      TaskID       uuid.UUID
      Objective    string
      Constraints  []string
      Decisions    []Decision
      Completed    []string
      Current      string
      Remaining    []string
      Blocked      []Blocker
      DoNotRepeat  []string
      LastVerified string
      NextAction   string
      Confidence   ConfidenceLevel
  }

  type Decision struct {
      ID          string
      Description string
      Source      string
      Status      string // "ACTIVE", "REJECTED"
  }

  type Blocker struct {
      ID          string
      Description string
      Status      string // "ACTIVE", "RESOLVED"
  }

  type Checkpoint struct {
      ID            uuid.UUID
      TaskID        uuid.UUID
      Timestamp     string // RFC3339 string
      Repository    string
      Branch        string
      Commit        string
      StateVersion  int
      EventPosition int
      StateData     State
  }
  ```
- **State Reducer Function**: `Reduce(taskID string, history []events.Event) State` (`engine.go:8-76`)
  - Reduces: `TaskStarted`, `ConstraintAdded`, `DecisionMade`, `MilestoneCompleted`, `BlockerCreated`, `BlockerResolved`, `GitCommit`.
  - Leaves unhandled: `RequirementAdded`, `UserApproval`, `UserRejection`, `FileChanged`, `CommandExecuted`, `TestStarted`, `TestPassed`, `TestFailed`, `SessionInterrupted`, `SessionResumed`.

#### C. Package `internal/db` (`internal/db/db.go`)
- **Storage Location**: `.sentinel/state.db` relative to repository root (`db.go:21, 31`)
- **Schema Migrations**:
  - `events` table: `id TEXT PRIMARY KEY, task_id TEXT NOT NULL, type TEXT NOT NULL, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, payload TEXT NOT NULL`
  - `checkpoints` table: `id TEXT PRIMARY KEY, task_id TEXT NOT NULL, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP, commit_hash TEXT NOT NULL, state_version INTEGER NOT NULL, event_position INTEGER NOT NULL, state_data TEXT NOT NULL, repository TEXT DEFAULT '', branch TEXT DEFAULT ''`
- **Public Functions**:
  - `InitDB(projectRoot string) (*sql.DB, error)`
  - `SaveCheckpoint(ctx context.Context, db *sql.DB, cp state.Checkpoint) error`
  - `GetLatestCheckpoint(ctx context.Context, db *sql.DB, taskID string) (*state.Checkpoint, error)`
  - `SaveEvent(ctx context.Context, db *sql.DB, e events.Event) error`
  - `GetEvents(ctx context.Context, db *sql.DB, taskID string) ([]events.Event, error)`

#### D. Package `internal/git` (`internal/git/*.go`)
- **Interfaces**:
  ```go
  type Runner interface {
      Run(ctx context.Context, dir string, args ...string) (stdout []byte, stderr []byte, err error)
  }

  type Client interface {
      GetState(ctx context.Context, repoPath string) (*RepositoryState, error)
      GetCurrentCommit(ctx context.Context, repoPath string) (string, error)
      GetCurrentBranch(ctx context.Context, repoPath string) (string, error)
      GetStatus(ctx context.Context, repoPath string) (*StatusResult, error)
      GetDiff(ctx context.Context, repoPath string, staged bool) (string, error)
      GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error)
      GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error)
      IsClean(ctx context.Context, repoPath string) (bool, error)
      CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error)
      IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error)
      GetRepoRoot(ctx context.Context, dir string) (string, error)
  }
  ```
- **Structs**:
  - `OSRunner`, `MockRunner`, `FileStatus`, `StatusResult`, `RepositoryState`, `FileChange`, `GitError`
- **Sentinels**: `ErrGitNotFound`, `ErrNotGitRepo`, `ErrNoCommits`, `ErrInvalidCommit`, `ErrGitLockExists`, `ErrDubiousOwnership`, `ErrMergeConflict`.
- **Parsing Functions**: `ParsePorcelainStatus`, `ExtractModifiedFiles`, `ParseNameOnlyList`, `ParseDiffNameStatus`.

#### E. Package `internal/reconcile` (`internal/reconcile/*.go`)
- **Interfaces**:
  ```go
  type Engine interface {
      Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult
  }
  ```
- **Structs**:
  ```go
  type ReconciliationStatus string // "SAFE", "STALE", "CONFLICT"

  type ReconciliationResult struct {
      Status               ReconciliationStatus  `json:"status"`
      Reason               string                `json:"reason"`
      CheckpointCommit     string                `json:"checkpoint_commit"`
      CurrentCommit        string                `json:"current_commit"`
      BranchMatch          bool                  `json:"branch_match"`
      ChangedFiles         []string              `json:"changed_files"`
      TaskRelatedChanges   []string              `json:"task_related_changes"`
      UnrelatedChanges     []string              `json:"unrelated_changes"`
      ConstraintViolations []string              `json:"constraint_violations"`
      InvalidatedClaims    []string              `json:"invalidated_claims"`
      ConfidenceLevel      state.ConfidenceLevel `json:"confidence_level,omitempty"`
  }
  ```
- **Functions**:
  - `NewEngine() Engine`
  - `Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult`
  - `ReconcileRepo(ctx context.Context, cp state.Checkpoint, gitClient git.Client, repoPath string, taskFiles []string) (ReconciliationResult, error)`

#### F. Package `cmd` (`cmd/*.go`)
- **Commands**:
  - `rootCmd`: Root command (`cmd/root.go:10-16`)
  - `checkpointCmd`: Captures git state, reduces events, writes checkpoint (`cmd/checkpoint.go:23-136`)
  - `statusCmd`: Reconciles checkpoint vs live git state, renders text or JSON (`cmd/status.go:23-153`)
  - `historyCmd`: Prints chronological event log (`cmd/history.go:18-57`)
  - `resumeCmd`: Emits recovery packet for agent resumption (`cmd/resume.go:19-127`)

---

## 2. Logic Chain: Architectural Findings & Coupling Analysis

### Finding 1: Missing Application / Service Layer (CLI Over-Responsibility)
- **Observation**:
  - `cmd/checkpoint.go:32-136`, `cmd/status.go:32-153`, `cmd/history.go:21-56`, and `cmd/resume.go:28-127` each independently perform:
    1. Directory resolution (`os.Getwd()`)
    2. Git client creation (`git.NewClient(nil)`)
    3. Git repo root discovery (`gitClient.GetRepoRoot(ctx, targetDir)`)
    4. SQLite database opening & migration (`db.InitDB(repoRoot)`)
    5. Database connection lifecycle management (`defer database.Close()`)
    6. Event loading, reduction, checkpoint creation, and reconciliation logic.
- **Logic**:
  - When CLI command handlers contain orchestration, database lifecycle, business domain workflows, and error recovery policies, the CLI layer cannot be reused.
  - If a future requirement adds an MCP server (e.g. for Antigravity or Claude Desktop), a daemon, a REST API, or a Go library wrapper, every workflow would have to be duplicated or extracted then.
- **Impact**: High coupling between presentation (Cobra) and business logic.

### Finding 2: Lack of Repository / Store Abstraction in Database Layer
- **Observation**:
  - `internal/db/db.go` exports standalone functions accepting `*sql.DB`: `SaveCheckpoint(ctx, db, cp)`, `GetLatestCheckpoint(ctx, db, taskID)`, `SaveEvent(ctx, db, e)`, `GetEvents(ctx, db, taskID)`.
- **Logic**:
  - Because functions accept concrete `*sql.DB`, callers cannot mock or stub persistence without spinning up a real SQLite database.
  - Furthermore, multi-step operations (such as recording a `GitCommit` event and creating a `Checkpoint` in `cmd/checkpoint.go:101-122`) are executed in separate queries without transaction boundaries (`sql.Tx`).
- **Impact**: High coupling to concrete SQLite database driver; inability to perform unit tests on higher-level workflows without disk I/O; potential partial write inconsistency if a checkpoint write fails after event insert.

### Finding 3: Leaky Abstraction in Reconciler (`ReconcileRepo` File System I/O)
- **Observation**:
  - `internal/reconcile/engine.go:308-326` iterates through `Completed` and `DoNotRepeat` items and performs `os.Stat(filepath.Join(root, filepath.FromSlash(p)))`.
- **Logic**:
  - While `internal/reconcile/engine.go` provides a clean pure function `Reconcile(cp, repo, taskFiles)`, the extended helper `ReconcileRepo` mixes git client calls (`git.Client`) with direct OS filesystem calls (`os.Stat`).
  - When unit testing `ReconcileRepo` with a `mockGitClient` (`engine_test.go:504-548`), tests must still manipulate real disk files via `os.WriteFile` because `os.Stat` is hardcoded.
- **Impact**: Incomplete decoupling of the reconciliation engine from the host filesystem.

### Finding 4: Untyped Event Payloads (`map[string]interface{}`) and Incomplete Reducer
- **Observation**:
  - `internal/events/models.go:37` defines `Payload map[string]interface{}`.
  - `internal/state/engine.go:19-74` inspects only 7 of the 17 defined `EventType` constants (`TaskStarted`, `ConstraintAdded`, `DecisionMade`, `MilestoneCompleted`, `BlockerCreated`, `BlockerResolved`, `GitCommit`).
  - The other 10 event types (`RequirementAdded`, `UserApproval`, `UserRejection`, `FileChanged`, `CommandExecuted`, `TestStarted`, `TestPassed`, `TestFailed`, `SessionInterrupted`, `SessionResumed`) are silently ignored by `Reduce()`.
  - Type assertions in `engine.go` (e.g. `e.Payload["objective"].(string)`) silently fail if the caller passes non-string types or altered keys.
- **Logic**:
  - An event-sourced architecture requires strong typing or strict validation for event payloads to prevent state corruption across long agent lifecycles.
  - Key domain events like `UserApproval` and `UserRejection` directly impact decision validity and confidence level, but are unhandled in state reduction.
- **Impact**: Silent drops of state transitions, fragile schema evolution, and potential runtime type assertion failures.

### Finding 5: Unicode Sort Order Mismatch in Test Suite
- **Observation**:
  - In `internal/git/adversarial_test.go:202-203`, the test fixture specifies:
    ```go
    expectedModified := []string{
        "deeply/nested/path with spaces/another file.go",
        "new name with space.txt",
        "normal_deleted.txt",
        "old name with space.txt",
        "path with spaces/my file.txt",
        "unicode_日本語_test.txt",
        "unicode_üñîçødé_файл.md",
    }
    ```
  - However, in UTF-8 byte encoding:
    `ü` starts with byte `0xC3` (`\xC3\xBC`).
    `日` starts with byte `0xE6` (`\xE6\x97\xA5`).
  - Standard Go `sort.Strings` orders by UTF-8 byte values, so `0xC3` comes before `0xE6`. Thus `unicode_üñîçødé_файл.md` sorts BEFORE `unicode_日本語_test.txt`.
- **Logic**:
  - `ExtractModifiedFiles()` correctly calls `sort.Strings(result)`.
  - The test expectation has the two unicode strings inverted, causing `TestAdversarial_FilenamesWithSpacesAndUnicode` to fail.
- **Impact**: Fails automated test verification (`go test ./internal/git`) despite the production code behaving correctly according to Go string sorting specs.

### Finding 6: Timestamp Representation Inconsistency
- **Observation**:
  - `events.Event.Timestamp` is `time.Time` (`internal/events/models.go:36`).
  - `state.Checkpoint.Timestamp` is `string` (`internal/state/models.go:57`).
  - `db.SaveCheckpoint` formats as `time.Now().UTC().Format(time.RFC3339)` (`internal/db/db.go:94`).
  - SQLite tables declare `timestamp DATETIME DEFAULT CURRENT_TIMESTAMP` (`internal/db/db.go:52, 58`).
- **Logic**:
  - Storing RFC3339 strings vs Unix timestamps vs `time.Time` causes conversions back and forth and potential sub-second ordering differences across drivers.
- **Impact**: Minor technical debt and inconsistency across models.

### Finding 7: Metadata Directory and Brand Alignment
- **Observation**:
  - Module is `github.com/wake/wake`.
  - Database folder is `.sentinel/` (`internal/db/db.go:21`).
  - Output messages print `[WAKE]` (`cmd/checkpoint.go:124`) and `WAKE STATUS` (`cmd/status.go:65, 92`).
- **Logic**:
  - The folder `.sentinel` was inherited from the initial prototype.
  - Standardizing on `.wake` with backward compatibility for `.sentinel` ensures clean branding and directory isolation.

---

## 3. Concrete Recommendations for Modularization & Clean Interfaces

### Recommendation 1: Introduce an Application / Service Layer (`internal/service`)
Create an overarching service facade that encapsulates state management, git inspection, database persistence, and reconciliation:

```
[CLI / MCP / API Layer] (cmd/ or mcp/)
          │
          ▼
[Wake Application Service] (internal/service)
    - CheckpointService
    - StatusService
    - ResumeService
    - HistoryService
          │
    ┌─────┴──────────────────┬─────────────────┐
    ▼                        ▼                 ▼
[State & Event Engine]  [Git Wrapper]   [Persistence Store]
(internal/state, events)(internal/git)   (internal/db Store)
```

**Proposed Interface**:
```go
package service

type TaskService interface {
    CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
    GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
    GetHistory(ctx context.Context, taskID string) ([]events.Event, error)
    ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
}
```

### Recommendation 2: Introduce a Storage Interface (`internal/db` or `internal/store`)
Replace bare functions in `internal/db` with a structured `Store` interface:

```go
package db

type Store interface {
    SaveCheckpoint(ctx context.Context, cp state.Checkpoint) error
    GetLatestCheckpoint(ctx context.Context, taskID string) (*state.Checkpoint, error)
    SaveEvent(ctx context.Context, e events.Event) error
    GetEvents(ctx context.Context, taskID string) ([]events.Event, error)
    WithTx(ctx context.Context, fn func(tx Store) error) error
    Close() error
}

type SQLiteStore struct {
    db *sql.DB
}
```
This enables in-memory mock stores for unit testing without filesystem creation.

### Recommendation 3: Strongly-Typed Event Payloads and Payload Constructors
Define typed payloads in `internal/events`:

```go
package events

type TaskStartedPayload struct {
    Objective string `json:"objective"`
}

type ConstraintAddedPayload struct {
    Constraint string `json:"constraint"`
}

type DecisionMadePayload struct {
    ID          string `json:"id"`
    Description string `json:"description"`
    Source      string `json:"source"`
    Status      string `json:"status"` // ACTIVE, REJECTED
}

type MilestoneCompletedPayload struct {
    Milestone string `json:"milestone"`
    Artifacts []string `json:"artifacts,omitempty"`
}

type GitCommitPayload struct {
    Hash   string `json:"hash"`
    Branch string `json:"branch"`
    Clean  bool   `json:"clean"`
}
```

### Recommendation 4: Complete the Event Reducer
Expand `state.Reduce` to process all 17 event types:
- `UserApproval` / `UserRejection`: Updates decision status and adjusts `State.Confidence`.
- `RequirementAdded`: Adds to `State.Remaining` or `State.Objective`.
- `TestPassed` / `TestFailed`: Updates verification state and confidence.
- `SessionInterrupted` / `SessionResumed`: Adjusts task continuity counters and timestamp records.

### Recommendation 5: Fix Sort Order in Test Fixture (`internal/git/adversarial_test.go`)
Update lines 202-203 of `internal/git/adversarial_test.go`:
```go
// Before:
"unicode_日本語_test.txt",
"unicode_üñîçødé_файл.md",

// After:
"unicode_üñîçødé_файл.md",
"unicode_日本語_test.txt",
```

### Recommendation 6: Abstract Filesystem Checks in Reconciler
Allow passing a custom file stat interface to `ReconcileRepo`:
```go
type FileChecker interface {
    Exists(path string) bool
}
```
Or allow `git.Client` to provide a `FileExists(ctx, repoPath, relPath)` method.

---

## 4. Caveats
1. **Network & External Access**: All analysis was conducted completely locally within `C:/Users/USER/Desktop/Sentinel` without remote network dependency.
2. **Pure Read-Only Mode**: In accordance with the explorer role constraints, no source code or test files in `cmd/` or `internal/` were modified.
3. **Go Version**: `go.mod` specifies `go 1.27.0`. The local environment contains Go binary at `C:\Program Files\Go\bin\go.exe`.

---

## 5. Conclusion
Project Wake has a well-structured foundational architecture implementing event sourcing, SQLite snapshot persistence, Git repository inspection, and state reconciliation (SAFE/STALE/CONFLICT). 
The core engines (`internal/git`, `internal/reconcile`, `internal/state`, `internal/db`) are functional and supported by comprehensive tests.

To transition the codebase from MVP to a production-grade, highly maintainable platform:
1. Extract business workflows out of `cmd/` into an `internal/service` facade.
2. Define a clean `Store` interface for persistence with transaction support.
3. Replace loosely-typed `map[string]interface{}` event payloads with typed structs.
4. Expand `state.Reduce()` to handle all 17 event types.
5. Fix the single string sorting expectation in `internal/git/adversarial_test.go`.

---

## 6. Verification Method

### How to Independently Verify These Findings:
1. **Verify Module & File Layout**:
   - Inspect `go.mod`, `main.go`, `cmd/`, and `internal/` directories.
2. **Verify Test Failure in `internal/git/adversarial_test.go`**:
   - Run: `& 'C:\Program Files\Go\bin\go.exe' test -v ./internal/git -run TestAdversarial_FilenamesWithSpacesAndUnicode`
   - Observe failure at line 206 due to `expectedModified` UTF-8 byte sort order mismatch (`ü` vs `日`).
3. **Verify Passing Reconcile and State Suites**:
   - Run: `& 'C:\Program Files\Go\bin\go.exe' test -v ./internal/state ./internal/reconcile ./internal/db`
   - All tests in `state`, `reconcile` (including 10 temporary Git repository scenarios), and `db` pass cleanly.
4. **Verify Leaky Abstractions**:
   - Inspect `cmd/checkpoint.go:41-57` and `cmd/status.go:41-52` to confirm repeated `gitClient`, `GetRepoRoot`, `db.InitDB` setup logic.
   - Inspect `internal/db/db.go:81, 123, 193, 227` to confirm `*sql.DB` parameter passing without an interface.

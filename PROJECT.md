# Project: Sentinel MVP Phase 2 (Reconciliation)

## Architecture
Sentinel Phase 2 introduces Git repository reconciliation against saved task state checkpoints. The architecture bridges the SQLite checkpoint persistence layer (Phase 1) and the live Git filesystem state via a modular, testable Go architecture:

```
+-------------------------------------------------------------------------------+
|                               SENTINEL CLI                                    |
|              (cmd/checkpoint.go, cmd/status.go, cmd/root.go)                   |
+-------------------------------------------------------------------------------+
           |                                                 |
           v                                                 v
+------------------------+                        +-----------------------------+
|    STATE & DB LAYER    |                        |        GIT CLI LAYER        |
|  (internal/state, db)  |                        |       (internal/git)        |
| - Checkpoint model     |                        | - Runner (OSRunner / Mock)  |
| - StateData snapshot   |                        | - Git Client & Status Parser|
| - SQLite Checkpoints   |                        | - RepositoryState snapshot  |
+------------------------+                        +-----------------------------+
           \                                                 /
            \                                               /
             v                                             v
        +------------------------------------------------------+
        |                RECONCILIATION ENGINE                 |
        |                (internal/reconcile)                  |
        | - Reconcile(checkpoint, repoState, taskFiles)        |
        | - SAFE / STALE / CONFLICT Evaluation                 |
        | - Constraint & Decision Violation Detection          |
        | - Completed Milestone Invalidation Detection         |
        +------------------------------------------------------+
                                   |
                                   v
        +------------------------------------------------------+
        |                  VERIFICATION SUITE                  |
        |           (internal/reconcile/*_test.go)             |
        | - Isolated Temp Git Repos (t.TempDir)                |
        | - Automated SAFE, STALE, CONFLICT Scenarios          |
        +------------------------------------------------------+
```

## Feature Inventory
| # | Feature | Description | Milestone | Source |
|---|---------|-------------|-----------|--------|
| 1 | Git Command Runner | Execution interface (Runner, OSRunner, MockRunner) for git CLI invocation with context support | M1 | Survey R1 |
| 2 | Git Status & Diff Parser | Parse porcelain v1 output into structured Staged, Unstaged, Untracked, and Unmerged files | M1 | Survey R1 |
| 3 | Git Client Implementation | High-level Git client retrieving commit hash, branch, clean status, and repo state | M1 | Survey R1 |
| 4 | Git Error Classification | Structured errors (ErrGitNotFound, ErrNotGitRepo, ErrNoCommits, ErrMergeConflict, etc.) | M1 | Survey R1 |
| 5 | Reconciliation Status Types | Type definitions for StatusSafe, StatusStale, StatusConflict, and ReconciliationResult | M2 | Survey R2 |
| 6 | SAFE State Evaluation | Evaluate identical commit, matching branch, and zero uncommitted changes | M2 | Survey R2 |
| 7 | STALE State Evaluation | Evaluate forward commits and non-conflicting task modifications | M2 | Survey R2 |
| 8 | CONFLICT State Evaluation | Evaluate constraint violations, invalidated completed artifacts, and merge conflicts | M2 | Survey R2 |
| 9 | Temporary Git Repo Test Harness | Test helper initializing temporary git repos with t.TempDir() and configurable git state | M3 | Survey Suite |
| 10| SAFE Scenario Tests | Automated Go test verifying SAFE status when repo matches checkpoint commit cleanly | M3 | Survey Suite |
| 11| STALE Scenario Tests | Automated Go test verifying STALE status when non-conflicting changes or forward commits occur | M3 | Survey Suite |
| 12| CONFLICT Scenario Tests | Automated Go test verifying CONFLICT status when constraints or task files are modified | M3 | Survey Suite |
| 13| CLI Status Integration | Integrate Git State and Reconciliation into `sentinel status` and `sentinel checkpoint` | M3 | Survey Integration |

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| M1 | Git CLI Wrapper | `internal/git` package: Runner, OSRunner, StatusParser, Client, RepositoryState, unit tests | None | DONE |
| M2 | Reconciliation Engine | `internal/reconcile` package: Reconcile function, evaluation logic for SAFE/STALE/CONFLICT, constraint and invalidation checks, unit tests | M1 | DONE |
| M3 | Verification Suite & Integration | `internal/reconcile/*_test.go` integration tests with temp Git repos, `cmd/status.go` & `cmd/checkpoint.go` integration, end-to-end `go test ./...` verification | M1, M2 | DONE |

## Interface Contracts

### internal/git ↔ internal/reconcile
```go
package git

type StatusCode string

const (
    StatusUnmodified StatusCode = " "
    StatusModified   StatusCode = "M"
    StatusAdded      StatusCode = "A"
    StatusDeleted    StatusCode = "D"
    StatusRenamed    StatusCode = "R"
    StatusUntracked  StatusCode = "?"
    StatusUnmerged   StatusCode = "U"
)

type FileStatus struct {
    Path           string     `json:"path"`
    OrigPath       string     `json:"orig_path,omitempty"`
    StagingStatus  StatusCode `json:"staging_status"`
    WorkTreeStatus StatusCode `json:"worktree_status"`
}

type RepositoryState struct {
    RootPath          string       `json:"root_path"`
    Branch            string       `json:"branch"`
    CommitHash        string       `json:"commit_hash"`
    IsDetached        bool         `json:"is_detached"`
    HasCommits        bool         `json:"has_commits"`
    IsClean           bool         `json:"is_clean"`
    HasMergeConflicts bool         `json:"has_merge_conflicts"`
    StagedFiles       []FileStatus `json:"staged_files"`
    UnstagedFiles     []FileStatus `json:"unstaged_files"`
    UntrackedFiles    []string     `json:"untracked_files"`
    UnmergedFiles     []string     `json:"unmerged_files"`
    ModifiedFiles     []string     `json:"modified_files"`
}

type Client interface {
    GetState(ctx context.Context, repoPath string) (*RepositoryState, error)
    GetCurrentCommit(ctx context.Context, repoPath string) (string, error)
    GetCurrentBranch(ctx context.Context, repoPath string) (string, error)
    IsClean(ctx context.Context, repoPath string) (bool, error)
    CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error)
    IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error)
    GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error)
}
```

### internal/state ↔ internal/reconcile
```go
package reconcile

import (
    "github.com/sentinel/sentinel/internal/git"
    "github.com/sentinel/sentinel/internal/state"
)

type ReconciliationStatus string

const (
    StatusSafe     ReconciliationStatus = "SAFE"
    StatusStale    ReconciliationStatus = "STALE"
    StatusConflict ReconciliationStatus = "CONFLICT"
)

type ReconciliationResult struct {
    Status               ReconciliationStatus `json:"status"`
    Reason               string               `json:"reason"`
    CheckpointCommit     string               `json:"checkpoint_commit"`
    CurrentCommit        string               `json:"current_commit"`
    BranchMatch          bool                 `json:"branch_match"`
    ChangedFiles         []string             `json:"changed_files"`
    TaskRelatedChanges   []string             `json:"task_related_changes"`
    UnrelatedChanges     []string             `json:"unrelated_changes"`
    ConstraintViolations []string             `json:"constraint_violations"`
    InvalidatedClaims    []string             `json:"invalidated_claims"`
}

// Engine evaluates a Checkpoint against live Git repository state.
type Engine interface {
    Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult
}

func Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult
```

## Code Layout
```
C:/Users/USER/Desktop/Sentinel/
|-- cmd/
|   |-- checkpoint.go          # Checkpoint command with Git state & reconciliation
|   |-- root.go                # Root CLI command
|   `-- status.go              # Status command displaying SAFE/STALE/CONFLICT
|-- internal/
|   |-- db/
|   |   `-- db.go              # SQLite database & migrations
|   |-- events/
|   |   `-- models.go          # Event definitions
|   |-- git/
|   |   |-- client.go          # Git Client implementation & constructor
|   |   |-- errors.go          # Git error types
|   |   |-- models.go          # RepositoryState, FileStatus, StatusCode
|   |   |-- parser.go          # Porcelain status parser
|   |   |-- runner.go          # Runner and OSRunner execution interfaces
|   |   |-- parser_test.go     # Unit tests for porcelain parser
|   |   `-- client_test.go     # Tests for git client
|   |-- reconcile/
|   |   |-- engine.go          # Reconcile function and status evaluation
|   |   |-- models.go          # ReconciliationStatus and ReconciliationResult
|   |   |-- engine_test.go     # Unit tests for reconciliation logic
|   |   `-- reconcile_test.go  # Autonomous verification suite with temp git repos
|   `-- state/
|       |-- engine.go          # Event reduction engine
|       |-- engine_test.go     # Reducer unit tests
|       `-- models.go          # Checkpoint, State, Decision, Blocker
|-- go.mod
|-- go.sum
`-- main.go
```

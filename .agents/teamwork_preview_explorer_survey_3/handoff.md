# Handoff Report: Requirement R2 (Reconciliation Engine) and Verification Suite Analysis

**Agent:** teamwork_preview_explorer_survey_3  
**Working Directory:** `C:/Users/USER/Desktop/Sentinel/.agents/teamwork_preview_explorer_survey_3`  
**Target:** Phase 2 Reconciliation Architecture and Verification Test Suite  
**Date:** 2026-08-28T16:56:00Z  

---

## 1. Observation

### 1.1 Original Request Specifications
From `C:/Users/USER/Desktop/Sentinel/.agents/ORIGINAL_REQUEST.md`:
- Lines 18-23:
  > `### R1. Git CLI Wrapper`  
  > `Build a utility layer that shells out to the local git binary to retrieve current repository information. It must be able to retrieve the current commit hash, list modified files, and list uncommitted changes.`  
  > `### R2. Reconciliation Engine`  
  > `Implement a comparison function that takes a saved Checkpoint object (from the SQLite database layer) and the current Git repository state. It must evaluate the differences and return a status of SAFE, STALE, or CONFLICT.`
- Lines 26-30:
  > `### Verification Suite`  
  > `- [ ] A Go test suite exists that uses a temporary Git repository to simulate SAFE, STALE, and CONFLICT scenarios.`  
  > `- [ ] The test suite runs automatically via go test and passes without human intervention.`  
  > `- [ ] The reconciliation engine correctly returns SAFE when the simulated repository exactly matches the checkpoint commit with no uncommitted changes.`  
  > `- [ ] The reconciliation engine correctly returns CONFLICT or STALE when simulated task-related files have been manually modified since the checkpoint.`

### 1.2 Product Requirements Document Specifications
From `C:/Users/USER/Desktop/Sentinel/Project Sentinel.md`:
- Section 9 (lines 346-420):
  - **SAFE**: Checkpoint remains consistent with current repository. Resuming is safe without invalidation.
  - **STALE**: Repository has changed since checkpoint, but changes can potentially be incorporated. Task state must be updated before resuming.
  - **CONFLICT**: Checkpoint assumptions materially contradict current repository state (e.g. protected constraint file modified). Requires developer attention or explicit reconciliation. Sentinel principle: *"Sentinel must prefer uncertainty over silently resuming from incorrect state."*
- Section 14 (lines 558-615):
  - Repository reconciliation compares Git state (checkpoint commit vs current commit, branch, uncommitted files, modified files), File categories (expected files, changed task files, deleted files, unexpected modifications), and Task state (completed steps, blockers, remaining work, decisions, constraints).
- Section 15-17 (lines 617-712):
  - Do-Not-Repeat and Completed claims become invalidated if verified files are altered or deleted.

### 1.3 Existing Codebase Structures
- `internal/state/models.go` (lines 24-64):
  - `Checkpoint` holds: `ID uuid.UUID`, `TaskID uuid.UUID`, `Timestamp string`, `Repository string`, `Branch string`, `Commit string`, `StateVersion int`, `EventPosition int`, `StateData State`.
  - `State` holds: `TaskID uuid.UUID`, `Objective string`, `Constraints []string`, `Decisions []Decision`, `Completed []string`, `Current string`, `Remaining []string`, `Blocked []Blocker`, `DoNotRepeat []string`, `LastVerified string`, `NextAction string`, `Confidence ConfidenceLevel`.
- `internal/db/db.go` (lines 37-45):
  - Checkpoints table stores: `id TEXT PRIMARY KEY`, `task_id TEXT NOT NULL`, `timestamp DATETIME`, `commit_hash TEXT NOT NULL`, `state_version INTEGER NOT NULL`, `event_position INTEGER NOT NULL`, `state_data TEXT NOT NULL`.
- `internal/state/engine.go` (lines 8-76):
  - State reduction engine supports `TaskStarted`, `ConstraintAdded`, `DecisionMade`, `MilestoneCompleted`, `BlockerCreated`, `BlockerResolved`, and `GitCommit` events.

---

## 2. Logic Chain

### 2.1 State Qualification Business Logic & Exact Conditions

```
+-----------------------------------------------------------------------------------+
|                           INCOMING RECONCILIATION INPUTS                          |
|  - Checkpoint (Commit, Branch, Constraints, Decisions, Completed, DoNotRepeat)    |
|  - Live Git State (CurrentCommit, CurrentBranch, ModifiedFiles, UntrackedFiles)   |
+-----------------------------------------------------------------------------------+
                                          |
                                          v
                +---------------------------------------------------+
                | 1. CONFLICT EVALUATION                            |
                |    - Any modified file violates a Constraint?     |
                |    - Any modified file violates an Active Decision?|
                |    - Any Completed / DoNotRepeat artifact deleted?|
                |    - Git branch diverged / history rewritten?     |
                |    - Working directory has merge conflict markers?|
                +---------------------------------------------------+
                                   /             \
                             [YES]                [NO]
                              /                     \
                             v                       v
                    +----------------+     +-----------------------------------+
                    | STATE: CONFLICT|     | 2. SAFE EVALUATION                |
                    +----------------+     |    - CurrentCommit == Cp.Commit?  |
                                           |    - CurrentBranch == Cp.Branch?  |
                                           |    - Working tree is 100% clean?  |
                                           +-----------------------------------+
                                                          /             \
                                                    [YES]                [NO]
                                                     /                     \
                                                    v                       v
                                           +-------------+         +--------------+
                                           | STATE: SAFE |         | STATE: STALE |
                                           +-------------+         +--------------+
```

#### Condition 1: SAFE
A state qualifies as **SAFE** if and only if ALL of the following criteria are met:
1. **Commit Equality:** `LiveGitState.CurrentCommit == Checkpoint.Commit` (and `Checkpoint.Commit` is not empty).
2. **Branch Equality:** `LiveGitState.CurrentBranch == Checkpoint.Branch` (or matching reference).
3. **Clean Working Tree:** Zero unstaged modifications, zero staged uncommitted changes, and zero deleted tracked files (`len(LiveGitState.ModifiedFiles) == 0`).
4. **Zero Constraint Violations:** No modified or deleted files conflict with any item in `Checkpoint.StateData.Constraints` or `Checkpoint.StateData.Decisions`.

#### Condition 2: STALE
A state qualifies as **STALE** if it is not a CONFLICT, but the repository has evolved since the checkpoint:
1. **Forward Commits:** `LiveGitState.CurrentCommit != Checkpoint.Commit`, where `CurrentCommit` is a forward descendant of `Checkpoint.Commit` without modifying constraint-protected files.
2. **Non-Conflicting Working Tree Modifications:** Uncommitted file modifications exist (`len(LiveGitState.ModifiedFiles) > 0`), but they belong to active task files or unrelated files and do NOT violate any `Constraints` or `Decisions`.
3. **Additive Untracked Changes:** New untracked files or documentation edits that do not contradict previous claims.
4. **Action:** The state is not broken, but task state confidence is downgraded (e.g. `ConfidenceLow` / `SAFE WITH UPDATES`), requiring state recalculation before resumption.

#### Condition 3: CONFLICT
A state qualifies as **CONFLICT** if ANY of the following criteria are met:
1. **Constraint / Decision Violation:** Any file changed (committed or uncommitted) matches a path or module protected by `Checkpoint.StateData.Constraints` (e.g., constraint "Do not touch auth" combined with edits to `auth/*`) or active `Decisions`.
2. **Completed Work Invalidation:** Files representing deliverables in `Checkpoint.StateData.Completed` or `Checkpoint.StateData.DoNotRepeat` have been deleted, reverted, or destructively modified.
3. **Branch Discontinuity / Divergence:** Current Git branch does not match `Checkpoint.Branch`, or Git history was rebased/force-pushed such that `Checkpoint.Commit` is no longer an ancestor of `HEAD`.
4. **Merge Conflict State:** Repository contains unmerged paths or conflict markers (`<<<<<<<`, `=======`, `>>>>>>>`).
5. **Sentinel Bias:** When in doubt between STALE and CONFLICT, Sentinel must fail safe to CONFLICT to prevent silent corruption.

---

### 2.2 Comparison Function Design and Interfaces

#### Proposed Package Structure:
- `internal/git/` — Git CLI execution layer (R1)
- `internal/reconcile/` — Comparison and state classification engine (R2)

#### Data Structures:
```go
package git

type FileStatus struct {
    Path     string // Relative slash-path
    Staging  byte   // 'M', 'A', 'D', 'R', '?'
    Worktree byte   // 'M', 'D', '?'
}

type RepoState struct {
    CurrentCommit string
    CurrentBranch string
    IsClean       bool
    ModifiedFiles []string // Staged, unstaged, or deleted files
    UntrackedFiles []string
}
```

```go
package reconcile

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
```

#### Comparison Function Signature:
```go
package reconcile

import (
    "github.com/sentinel/sentinel/internal/git"
    "github.com/sentinel/sentinel/internal/state"
)

// Reconcile evaluates a saved Checkpoint against live Git repository state.
func Reconcile(cp state.Checkpoint, repo git.RepoState, taskFiles []string) ReconciliationResult
```

#### Comparison Logic Flow:
1. **Extract Constraint Paths:** Parse path rules or keywords from `cp.StateData.Constraints` and `cp.StateData.Decisions`.
2. **Scan Changed Files:** Aggregate all files in `repo.ModifiedFiles` and any commit diffs between `cp.Commit` and `repo.CurrentCommit`.
3. **Evaluate Invalidation & Constraints:**
   - If any changed file matches constraint paths -> Flag as `ConstraintViolation`, return `StatusConflict`.
   - If any completed milestone file is missing or corrupted -> Flag as `InvalidatedClaims`, return `StatusConflict`.
4. **Evaluate Clean / Match:**
   - If `repo.CurrentCommit == cp.Commit` && `repo.IsClean` && (`repo.CurrentBranch == cp.Branch` || cp.Branch == "") -> return `StatusSafe`.
5. **Default to STALE:**
   - If there are changes, but no constraints/milestones are violated -> return `StatusStale`.

---

### 2.3 Verification Test Strategy

To satisfy the Acceptance Criteria without human intervention:

#### 1. Test Harness Design
- Use Go's built-in `t.TempDir()` to guarantee isolated temporary directories that are cleanly purged after each test execution.
- Create a test helper `initTestGitRepo(t *testing.T) string` that:
  1. Creates directory inside `t.TempDir()`.
  2. Runs `git init`.
  3. Configures local identity: `git config user.name "Sentinel Test"` and `git config user.email "test@sentinel.local"`.
  4. Configures `git config commit.gpgsign false`.
  5. Creates initial files, runs `git add .`, and `git commit -m "initial commit"`.
  6. Returns the path and HEAD commit hash.

#### 2. Automated Test Scenarios

##### Test 1: `TestReconciliation_SAFE`
- **Setup:**
  1. Call `initTestGitRepo(t)` -> commit `C1`.
  2. Build `state.Checkpoint` with `Commit: C1`, `Branch: "main"`, clean `StateData`.
- **Execution:**
  1. Retrieve live `git.RepoState` from temporary repo.
  2. Run `reconcile.Reconcile(checkpoint, repoState, nil)`.
- **Assertion:**
  - `result.Status == reconcile.StatusSafe`
  - `len(result.ChangedFiles) == 0`
  - `len(result.ConstraintViolations) == 0`

##### Test 2: `TestReconciliation_STALE_UncommittedEdits`
- **Setup:**
  1. Call `initTestGitRepo(t)` -> commit `C1`.
  2. Checkpoint created at `C1` with task objective "Billing Service".
  3. Modify `billing/invoice.go` in working directory without committing.
- **Execution:**
  1. Retrieve live `git.RepoState`.
  2. Run `reconcile.Reconcile(checkpoint, repoState, []string{"billing/invoice.go"})`.
- **Assertion:**
  - `result.Status == reconcile.StatusStale`
  - `result.ChangedFiles` contains `"billing/invoice.go"`
  - `len(result.ConstraintViolations) == 0`

##### Test 3: `TestReconciliation_STALE_ForwardCommits`
- **Setup:**
  1. Call `initTestGitRepo(t)` -> commit `C1`.
  2. Checkpoint created at `C1`.
  3. Create new file `docs/readme.md`, `git add .`, `git commit -m "update docs"` -> commit `C2`.
- **Execution:**
  1. Retrieve live `git.RepoState`.
  2. Run `reconcile.Reconcile(checkpoint, repoState, nil)`.
- **Assertion:**
  - `result.Status == reconcile.StatusStale`
  - `result.CurrentCommit != result.CheckpointCommit`
  - `result.Status != reconcile.StatusConflict`

##### Test 4: `TestReconciliation_CONFLICT_ConstraintViolation`
- **Setup:**
  1. Call `initTestGitRepo(t)` -> commit `C1` (contains `auth/session.go` and `billing/pay.go`).
  2. Checkpoint created at `C1` with constraint `"Do not touch auth"` (or constraint path `"auth/"`).
  3. Edit `auth/session.go`.
- **Execution:**
  1. Retrieve live `git.RepoState`.
  2. Run `reconcile.Reconcile(checkpoint, repoState, []string{"billing/pay.go"})`.
- **Assertion:**
  - `result.Status == reconcile.StatusConflict`
  - `len(result.ConstraintViolations) > 0`
  - `result.Reason` specifies constraint violation on `auth/session.go`.

##### Test 5: `TestReconciliation_CONFLICT_DeletedMilestoneArtifact`
- **Setup:**
  1. Call `initTestGitRepo(t)` -> commit `C1` (contains `schema/migration.sql`).
  2. Checkpoint created at `C1` with `Completed: ["schema/migration.sql"]` or `DoNotRepeat: ["schema/migration.sql"]`.
  3. Delete `schema/migration.sql` from the filesystem.
- **Execution:**
  1. Retrieve live `git.RepoState`.
  2. Run `reconcile.Reconcile(checkpoint, repoState, nil)`.
- **Assertion:**
  - `result.Status == reconcile.StatusConflict`
  - `len(result.InvalidatedClaims) > 0`

---

## 3. Caveats

1. **Path Normalization:** File paths on Windows use backslashes (`\`), whereas Git outputs forward slashes (`/`). All path comparisons across Git output and Checkpoint state MUST be normalized using `filepath.ToSlash` and lowercase comparison where file systems are case-insensitive.
2. **Git Binary Requirement:** Tests rely on the presence of the `git` binary in the host `PATH`. The test helper should check `exec.LookPath("git")` and skip with `t.Skip("git not found")` if missing.
3. **Empty Checkpoint Commit Edge Case:** If a checkpoint is recorded before any initial commit exists in a brand new repository, `Checkpoint.Commit` may be empty. The comparison engine must treat an uninitialized repository with dirty files as `STALE` or `CONFLICT`, never `SAFE`.

---

## 4. Conclusion

1. The boundary conditions for `SAFE`, `STALE`, and `CONFLICT` are well-defined and deterministic based on the combination of Git commit hashes, working directory cleanliness, constraint pattern matching, and completed milestone file integrity.
2. The comparison engine function `Reconcile(cp state.Checkpoint, repo git.RepoState, taskFiles []string) ReconciliationResult` seamlessly connects Phase 1 SQLite Checkpoints with Phase 2 Git CLI observations.
3. The proposed Go verification test suite leverages `t.TempDir()` and automated Git commands to guarantee 100% autonomous execution across Windows, Linux, and macOS without requiring mock libraries or human interaction.

---

## 5. Verification Method

To independently verify the implementation once coded:

1. **Test Execution Command:**
   ```bash
   go test -v ./internal/reconcile/... ./internal/git/...
   ```
2. **Inspect Test Coverage:**
   ```bash
   go test -coverprofile=coverage.out ./...
   go tool cover -func=coverage.out
   ```
3. **Invalidation Check:**
   - Any test failure where a modified constraint file returns `SAFE` or `STALE` instead of `CONFLICT` indicates an invalidation of Acceptance Criteria.
   - Any test failure where an unmodified repo matching the checkpoint commit returns non-`SAFE` indicates an invalidation of Acceptance Criteria.

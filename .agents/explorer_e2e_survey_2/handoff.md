# E2E Testing Survey and Gap Analysis Report

## Executive Summary
This report provides a comprehensive investigation of the testing utilities, test suites, simulation mechanisms, and coverage gaps across the Wake codebase (`C:/Users/USER/Desktop/Sentinel`). It defines an actionable blueprint for the End-to-End (E2E) Testing Track across Tiers 1 through 4, resolves test harness duplication, and specifies a unified `internal/testutil` package.

---

## 1. Observation

### 1.1 Existing Test Suite Inventory and Execution Baseline
The repository currently contains 10 test files across 4 packages (`cmd`, `internal/db`, `internal/git`, `internal/reconcile`, `internal/state`). No test files exist in `internal/events`, and no shared test utilities exist in `internal/testutil`.

```
====================================================================================================
Package                 Test File                                Lines   Tests/Subtests   Status
====================================================================================================
cmd                     cmd/checkpoint_test.go                   134     3                PASS
cmd                     cmd/status_test.go                       80      2                PASS
cmd                     cmd/history_test.go                      0       0 (Missing)      0% Coverage
cmd                     cmd/resume_test.go                       0       0 (Missing)      0% Coverage
internal/events         (No test files)                          0       0 (Missing)      0% Coverage
internal/db             internal/db/db_test.go                   195     4                PASS
internal/state          internal/state/engine_test.go            98      3                PASS
internal/git            internal/git/parser_test.go              194     5                PASS
internal/git            internal/git/client_test.go              401     7                PASS
internal/git            internal/git/adversarial_test.go         562     9 (24 subtests)  FAIL (1 test)
internal/git            internal/git/lifecycle_adversarial_test  315     5 subtests       PASS
internal/reconcile      internal/reconcile/engine_test.go        710     21               PASS
internal/reconcile      internal/reconcile/reconcile_test.go     558     10               PASS
internal/testutil       (Directory does not exist)               0       0                N/A
====================================================================================================
```

#### Test Execution Baseline:
Running `& "C:\Program Files\Go\bin\go.exe" test ./...` produces:
```
?       github.com/wake/wake                    [no test files]
ok      github.com/wake/wake/cmd                4.089s
ok      github.com/wake/wake/internal/db        2.007s
?       github.com/wake/wake/internal/events    [no test files]
--- FAIL: TestAdversarial_FilenamesWithSpacesAndUnicode (0.00s)
    adversarial_test.go:206: ExtractModifiedFiles mismatch:
        expected: [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_日本語_test.txt unicode_üñîçødé_файл.md]
        got:      [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_üñîçødé_файл.md unicode_日本語_test.txt]
FAIL
FAIL    github.com/wake/wake/internal/git       4.782s
ok      github.com/wake/wake/internal/reconcile 6.364s
ok      github.com/wake/wake/internal/state     1.024s
FAIL
```

#### Static Analysis Baseline:
Running `& "C:\Program Files\Go\bin\go.exe" vet ./...` produces:
```
Exit code: 0
Stdout: (empty)
Stderr: (empty)
```

---

### 1.2 Inventory of Existing Test Helpers and Duplication

Existing test files construct ad-hoc simulation mechanisms independently:

#### 1. Git Repository Simulators & Temp Dir Setup:
- **`cmd/checkpoint_test.go:14-60`**:
  - `findGit()`: Searches `PATH` and Windows candidate paths (`C:\Program Files\Git\cmd\git.exe`, etc.).
  - `setupTempGitRepo(t)`: Runs `git init`, `config user.name`, `config user.email`, `config commit.gpgsign false`, creates `README.md`, and commits.
- **`cmd/status_test.go:13-41`**:
  - `setupTempGitRepoForStatus(t)`: Duplicates `setupTempGitRepo` logic but creates `main.go`.
- **`internal/git/client_test.go:222-246`**:
  - `TestIntegration_RealGitRepositoryLifecycle`: Inlines `exec.LookPath("git")`, `t.TempDir()`, `runGit` closure, `git init`, and `config`.
- **`internal/git/lifecycle_adversarial_test.go:14-51`**:
  - `runGitCommand(t, ctx, dir, args...)`, `runGitCommandAllowError(ctx, dir, args...)`, and test repo setup in `TestIntegration_AdversarialLifecycle`.
- **`internal/reconcile/reconcile_test.go:17-167`**:
  - `gitTestRepo` struct (`lines 17-23`), `locateGitBinary()` (`lines 26-41`), `initGitTestRepo(t)` (`lines 44-64`), `runGit` (`lines 67-76`), `runGitAllowError` (`lines 79-85`), `writeFile` (`lines 88-98`), `deleteFile` (`lines 101-107`), `commitAll` (`lines 110-115`), `currentCommit` (`lines 118-121`), `currentBranch` (`lines 124-128`), `createBranch` (`lines 131-134`), `checkoutBranch` (`lines 137-140`), `makeTestCheckpoint` (`lines 143-167`).

#### 2. Mock Runners & Mock Git Clients:
- **`internal/git/runner.go:97-145` (`MockRunner`)**: In-memory `Runner` implementation storing string-keyed canned responses. Used in `internal/git/client_test.go` and `internal/git/adversarial_test.go`.
- **`internal/reconcile/engine_test.go:656-709` (`mockGitClient`)**: Custom struct implementing `git.Client` with preconfigured `state`, `commitExists`, `isAncestor`, and `changedFiles`.

#### 3. SQLite Database Test Setup:
- **`internal/db/db_test.go:16-21, 35-40, 140-145`**:
  - Calls `tmpDir := t.TempDir()` followed by `db, err := InitDB(tmpDir)`.
  - Hardcoded to write `.sentinel/state.db` on disk; no in-memory `:memory:` fixture option exists.

#### 4. Synthetic State Checkpoint Fixtures:
- **`internal/reconcile/engine_test.go:16-37` (`newTestCheckpoint`)**:
  - Constructs `state.Checkpoint` instances with pre-set UUIDs, RFC3339 timestamps, branch, commit, and state data.

---

## 2. Logic Chain: Root Causes and Architectural Survey

### 2.1 Harness Fragmentation and Lack of `internal/testutil`
- **Observation**: 5 different test files across `cmd`, `internal/git`, and `internal/reconcile` implement their own Git binary finder and temporary repo initializer.
- **Logic**:
  - Maintaining 5 separate Git setup implementations introduces drift (e.g. some configure `core.quotepath=false`, some do not; some use `main`, some fallback to default initial branch).
  - Test maintenance cost increases quadratically as new E2E tests are added.
- **Deduction**: A unified `internal/testutil` package with `GitRepo`, `DBFixture`, and `StateFixtures` is required before implementing Tiers 1-4.

### 2.2 UTF-8 Sort Order Mismatch in `adversarial_test.go:206`
- **Observation**:
  - Production code `internal/git/parser.go:161` executes standard Go `sort.Strings(result)`.
  - Go sorts strings by UTF-8 raw byte values (`\xC3` for `ü` vs `\xE6` for `日`).
  - Test fixture in `internal/git/adversarial_test.go:202-203` expected `unicode_日本語_test.txt` before `unicode_üñîçødé_файл.md`.
- **Logic**:
  - Byte `0xC3` is strictly less than byte `0xE6`. Therefore, `unicode_ü...` sorts before `unicode_日...`.
  - The production code behaves correctly; the test assertion contains inverted elements.
- **Deduction**: Reordering lines 202-203 in `internal/git/adversarial_test.go` will restore 100% pass rate in `internal/git`.

### 2.3 Comprehensive Test Gap Analysis Across Tiers 1-4

```
+---------------------------------------------------------------------------------------------------+
| TIER 4: Real-World Workloads & Multi-Step Agent Recovery Scenarios                               |
| - Multi-Step AI Agent Lifecycle: Objective -> Constraints -> File Edits -> Blocker -> Resume     |
| - Agent Crash & Recovery: Mid-operation crash with dirty working tree, resume instruction check  |
| - Collaborative Divergence: Concurrent branch switch, merge conflict injection & resolution       |
| - State Rollback / Forking: Reverting git commit to earlier checkpoint, verifying state drift     |
| - Concurrency Torture: 50 concurrent workers writing events, creating checkpoints, reading status|
+---------------------------------------------------------------------------------------------------+
                                                  |
+---------------------------------------------------------------------------------------------------+
| TIER 3: Cross-Feature Pairwise & Multi-Subsystem Combinations                                     |
| - Checkpoint + Working Tree Edits + Status + Resume Pipeline                                      |
| - Checkpoint + Constraint Modification + Status (CONFLICT) + Resume (Warning Banner)               |
| - Checkpoint + Milestone Deletion + Status (InvalidatedClaims) + Resume                           |
| - Checkpoint + Forward Commits + Status (STALE) + Resume (Diff Review Instruction)                |
| - Checkpoint + Branch Switching + Status (Branch Mismatch) + Resume                               |
| - Multi-Task Isolation: Concurrent task streams in same repo without state cross-contamination   |
+---------------------------------------------------------------------------------------------------+
                                                  |
+---------------------------------------------------------------------------------------------------+
| TIER 2: Boundary, Corner Cases & System Resilience                                                |
| - Empty Git Repositories: 0-commit repo checkpointing, status, resume, and untracked handling     |
| - Detached HEAD States: Checkpoints and reconciliation on detached HEAD vs named branches         |
| - Missing / Diverged Commits: Checkpoint commit missing from git object db or diverged history    |
| - Database Faults: Corrupted DB bytes, locked SQLite DB, invalid JSON payload strings in rows     |
| - Pathological Paths: Deeply nested paths (200+ chars), unicode (CJK, Cyrillic, Latin), spaces   |
| - Giant Payloads & Malformed Timestamps: 10MB event payloads, 10k events, zero/future timestamps  |
+---------------------------------------------------------------------------------------------------+
                                                  |
+---------------------------------------------------------------------------------------------------+
| TIER 1: Feature Coverage (Happy Path Verification across all Components)                          |
| - CLI & Service Commands: checkpoint, status, history (0% tested), resume (0% tested)            |
| - Events System: All 17 EventTypes serialization, deserialization, and validation (0% tested)    |
| - State Reducer: Complete reduction matrix across all 17 EventTypes and dynamic confidence        |
| - Database Store: Full CRUD operations, event chronological queries, checkpoint versioning        |
| - Git Client: Repository state inspection, branch detection, diff extraction, porcelain parsing   |
| - Reconciliation Engine: Pure in-memory SAFE, STALE, and CONFLICT evaluation                      |
+---------------------------------------------------------------------------------------------------+
```

---

## 3. Detailed Test Gaps & Specification Matrix (Tiers 1-4)

### 3.1 Tier 1: Feature Coverage (Happy Path)

| # | Feature Target | Current Coverage | Missing Happy Path Scenarios | Target File |
|---|---|---|---|---|
| 1.1 | `wake checkpoint` CLI | Partial (`cmd/checkpoint_test.go`) | - Checkpoint creation with auto-generated TaskID<br>- Checkpoint creation with explicit `--objective`<br>- Version incrementing across 3 sequential checkpoints<br>- Checkpoint creation with explicit `--dir` pointing to relative/absolute repo paths | `cmd/checkpoint_test.go` |
| 1.2 | `wake status` CLI | Partial (`cmd/status_test.go`) | - Status report on clean matching repo (SAFE)<br>- Status report on modified task files (STALE)<br>- Status report on constraint violations (CONFLICT)<br>- Structured JSON rendering (`--json`) matching schema | `cmd/status_test.go` |
| 1.3 | `wake history` CLI | **0.0%** (Missing) | - History display for active task with 5+ chronological events<br>- History filtering with `--task-id`<br>- Formatted output matching timestamp `[15:04:05]` and type<br>- Summary total events count rendering<br>- Graceful error handling when no active checkpoint exists | `cmd/history_test.go` |
| 1.4 | `wake resume` CLI | **0.0%** (Missing) | - Recovery packet rendering for SAFE state ("Safe to resume from Next Action")<br>- Recovery packet rendering for STALE state (includes changed files list & context reload instructions)<br>- Recovery packet rendering for CONFLICT state (includes constraint violation warnings)<br>- Active blocker banner display vs omission of resolved blockers<br>- Objective, completed milestones, constraints, and last verified commit rendering | `cmd/resume_test.go` |
| 1.5 | `internal/events` | **0.0%** (Missing) | - `NewEvent` constructor validation for all 17 `EventType` constants<br>- JSON serialization and deserialization roundtrip for all 17 event payload structures<br>- Verification of UTC timestamp assignment and non-nil UUID generation | `internal/events/models_test.go` |
| 1.6 | `internal/state` Reducer | Partial (`engine_test.go`: 4/17 events) | - Full reduction matrix covering all 17 event types:<br>  * `RequirementAdded`<br>  * `UserApproval` & `UserRejection`<br>  * `FileChanged` & `CommandExecuted`<br>  * `TestStarted`, `TestPassed`, `TestFailed`<br>  * `SessionInterrupted` & `SessionResumed`<br>- Dynamic confidence computation (`ConfidenceHigh`, `ConfidenceLow`, `ConfidenceNone`)<br>- State fields assignment (`Current`, `Remaining`, `DoNotRepeat`, `NextAction`, `TaskID`) | `internal/state/engine_test.go` |
| 1.7 | `internal/db` Store | High (`db_test.go`) | - Checkpoint version ordering query verification<br>- Events chronological retrieval by `task_id`<br>- Global latest checkpoint retrieval when taskID is empty | `internal/db/db_test.go` |
| 1.8 | `internal/reconcile` Engine | High (`engine_test.go`, `reconcile_test.go`) | - Reconcile pure function with matching commit and clean tree -> SAFE<br>- Reconcile pure function with forward commit -> STALE<br>- Reconcile pure function with modified constraint -> CONFLICT | `internal/reconcile/engine_test.go` |

---

### 3.2 Tier 2: Boundary & Corner Cases

| # | Subsystem | Category | Detailed Scenario & Test Objective | Expected Behavior |
|---|---|---|---|---|
| 2.1 | Git & CLI | Empty Repository | Execute `checkpoint`, `status`, `history`, and `resume` in a fresh repository with 0 commits (`git init`). | - `checkpoint`: succeeds with empty commit hash or records initial commit<br>- `status`: gracefully reports empty repo status without crashing<br>- `git.GetState`: returns `HasCommits=false`, `CommitHash=""`, `IsClean=true` |
| 2.2 | Git & Reconcile | Untracked in Empty Repo | Fresh 0-commit repo with untracked files and staged files before initial commit. | `git.GetState` reports `HasCommits=false`, `IsClean=false`, staged/untracked files correctly populated. |
| 2.3 | Git & Reconcile | Detached HEAD State | Reconcile repository when checked out to a raw commit hash (`git checkout <hash>`). Checkpoint recorded on named branch vs detached HEAD. | If commit matches, `BranchMatch=true` (HEAD compatibility), status is SAFE. If commit differs, STALE or CONFLICT. |
| 2.4 | Reconcile | Missing Commit Object | Checkpoint references a commit hash that does not exist in local Git object database (e.g. garbage collected or external). | `ReconcileRepo` returns `StatusConflict`, `ConfidenceNone`, and reason "Checkpoint commit ... does not exist in repository". |
| 2.5 | Reconcile | Diverged Git History | Checkpoint was created on commit A; current HEAD is on commit B which diverged from base (A is not an ancestor of B). | `ReconcileRepo` returns `StatusConflict`, `ConfidenceNone`, and reason "Checkpoint commit ... has diverged". |
| 2.6 | DB & Persistence | Corrupted SQLite DB | SQLite database file has invalid header or random bytes injected. | `db.InitDB` or queries return descriptive error without application panic. |
| 2.7 | DB & Store | Malformed JSON in DB Rows | Database rows in `checkpoints` or `events` contain truncated/invalid JSON strings in `state_data` or `payload`. | `GetLatestCheckpoint` and `GetEvents` return deserialization error without crashing. |
| 2.8 | DB & Store | Malformed / Extreme Task IDs | Task IDs formatted as invalid UUIDs (`"invalid-uuid"`), SQL injection attempts (`"' OR 1=1 --"`), or 10,000-character strings. | Validation rejects invalid UUIDs in CLI; parameterized queries in DB prevent SQL injection. |
| 2.9 | Git & Reconcile | Pathological File Paths | Files with spaces, unicode characters (CJK `日本語`, Cyrillic `файл`, Latin accents `üñîçødé`), quotes, brackets, and deep nesting (>200 chars). | Status and diff parsers normalize paths correctly, extract modified files cleanly, and evaluate constraints without dropping characters. |
| 2.10 | Git & Reconcile | Windows Path Separators | Checkpoints and constraints written with Windows backslashes (`auth\session.go`, `.\internal\git\*`) evaluated against forward-slash Git output. | `normalizePath` standardizes all paths to forward slashes; constraints match correctly. |
| 2.11 | Events & State | Giant Event Payloads | Events with 10MB payload maps (e.g. massive compiler logs or diffs). | Event serialization and `state.Reduce` process large payloads without memory leaks or stack overflow. |
| 2.12 | Events & State | Out-of-Order / Invalid Timestamps | Event streams with zero time (`0001-01-01`), future timestamps, or non-chronological event inputs. | DB ordering by `timestamp ASC, rowid ASC` preserves deterministic sequence. |

---

### 3.3 Tier 3: Cross-Feature Pairwise Combinations

| # | Combination Pair / Pipeline | Workflow Steps | Verification Assertions |
|---|---|---|---|
| 3.1 | Checkpoint + Git Edits + Status | 1. Initialize repo and create base commit.<br>2. Run `wake checkpoint --task-id <id>`.<br>3. Edit a non-constrained task file in worktree.<br>4. Run `wake status --task-id <id>`. | - `status` outputs `[STALE]`.<br>- `Confidence` is `Low`.<br>- `TaskRelatedChanges` contains the edited file.<br>- `ConstraintViolations` is 0. |
| 3.2 | Checkpoint + Constraint Violation + Status | 1. Run `wake checkpoint` with constraint `"protected/*"`.<br>2. Modify `protected/config.go` in worktree.<br>3. Run `wake status`. | - `status` outputs `[CONFLICT]`.<br>- `Confidence` is `None`.<br>- `ConstraintViolations` contains `Constraint 'protected/*' violated by modified file 'protected/config.go'`. |
| 3.3 | Checkpoint + Milestone Deletion + Status | 1. Run `wake checkpoint` with completed milestone `"docs/api.md"`.<br>2. Physically delete `docs/api.md` from disk.<br>3. Run `wake status`. | - `status` outputs `[CONFLICT]`.<br>- `InvalidatedClaims` reports `Claimed file 'docs/api.md' does not exist on disk`. |
| 3.4 | Checkpoint + Git Modifications + Resume | 1. Create checkpoint with objective and next action.<br>2. Modify working tree file `handler.go`.<br>3. Run `wake resume`. | - `resume` output contains goal, completed items, next action.<br>- Delta section lists `handler.go` under changed files.<br>- Output includes `RECOVERY INSTRUCTION: Read the changed files above before continuing`. |
| 3.5 | Events Stream + History + Resume | 1. Append sequential events: `TaskStarted`, `ConstraintAdded`, `BlockerCreated (B1)`, `BlockerResolved (B1)`, `MilestoneCompleted`.<br>2. Run `wake checkpoint`.<br>3. Run `wake history`.<br>4. Run `wake resume`. | - `history` renders all 5 events with timestamps.<br>- `resume` lists completed milestone, no active blockers (B1 is resolved), and correct constraints. |
| 3.6 | Checkpoint + Git Commit Advancement + Status | 1. Create checkpoint at Commit 1.<br>2. Stage and commit changes to create Commit 2.<br>3. Run `wake status`. | - `status` outputs `[STALE]`.<br>- Checkpoint commit is Commit 1; current commit is Commit 2.<br>- Reason indicates commit difference without constraint violations. |
| 3.7 | Checkpoint + Branch Switch + Status | 1. Create checkpoint on branch `feat/auth`.<br>2. Switch branch to `feat/billing` (pointing to same commit).<br>3. Run `wake status`. | - `status` outputs `[STALE]`.<br>- `BranchMatch` is `false`.<br>- Reason indicates branch mismatch (`feat/billing` vs `feat/auth`). |
| 3.8 | Multi-Task Concurrency & Isolation | 1. In same repository, run `wake checkpoint --task-id <UUID_A>` (Objective A).<br>2. Run `wake checkpoint --task-id <UUID_B>` (Objective B).<br>3. Query status and history for A and B. | - Status for A returns Objective A; Status for B returns Objective B.<br>- Event histories for A and B remain strictly partitioned without crosstalk. |

---

### 3.4 Tier 4: Real-World Workloads & Agent Recovery Scenarios

| # | Workload / Recovery Scenario | Simulation Method | Success Criteria & Assertions |
|---|---|---|---|
| 4.1 | **Complete Multi-Step AI Agent Session Lifecycle** | Simulate a 10-step AI developer session:<br>1. `TaskStarted`: "Refactor Database Layer"<br>2. `RequirementAdded`: "Must support SQLite WAL mode"<br>3. `ConstraintAdded`: "Do not alter auth/session.go"<br>4. `DecisionMade`: "DEC-1: Use modernc.org/sqlite"<br>5. `FileChanged`: "internal/db/store.go"<br>6. `GitCommit`: commit 1<br>7. `Checkpoint`: StateVersion 1<br>8. `BlockerCreated`: "B-1: Windows file lock contention"<br>9. `BlockerResolved`: "B-1: Added busy_timeout pragma"<br>10. `MilestoneCompleted`: "Database refactored" -> Checkpoint StateVersion 2 | - State reduction deterministically produces StateVersion 2.<br>- `wake history` lists all 10 events chronologically.<br>- `wake resume` generates a recovery packet with high confidence, completed milestone, 0 active blockers, active decision DEC-1, and next action. |
| 4.2 | **Agent Crash & Dirty Worktree Recovery** | 1. Agent creates Checkpoint 1 at Commit 1.<br>2. Agent begins modifying 3 files in working tree (`a.go`, `b.go`, `c.go`).<br>3. Agent process crashes / context window truncates.<br>4. Replacement agent process wakes up and executes `wake resume`. | - `wake resume` reads SQLite checkpoint from disk.<br>- Reconciler inspects live repo and identifies uncommitted edits in `a.go`, `b.go`, `c.go`.<br>- Emits STALE recovery packet with recovery instructions directing the new agent to review the 3 uncommitted files before continuing. |
| 4.3 | **Collaborative Divergence & Merge Conflict Injection** | 1. Agent A creates Checkpoint on branch `main` at Commit 1.<br>2. Branch `feature` commits conflicting changes to `service/auth.go`.<br>3. External process runs `git merge feature` into `main`, triggering unresolved merge conflicts (`UU` state).<br>4. Agent runs `wake status` and `wake resume`. | - `wake status` detects `HasMergeConflicts=true` and unmerged file `service/auth.go`.<br>- Returns `StatusConflict` with `ConfidenceNone` and reason "Working tree has unresolved merge conflicts".<br>- `wake resume` halts autonomous progression and prompts for manual conflict resolution. |
| 4.4 | **Checkpoint Rollback & Git Reset Recovery** | 1. Agent creates Checkpoint 1 at Commit 1.<br>2. Agent progresses and creates Checkpoint 2 at Commit 2.<br>3. Agent determines approach was flawed and executes `git reset --hard Commit 1`.<br>4. Agent checks `wake status`. | - Reconciler detects that Checkpoint 2 commit (Commit 2) does not match current commit (Commit 1) and is not an ancestor.<br>- Reports CONFLICT / STALE drift, allowing the agent to branch or re-checkpoint from Commit 1. |
| 4.5 | **High-Concurrency DB Access & Lock Torture** | Launch 50 concurrent goroutines continuously calling `SaveEvent`, `SaveCheckpoint`, `GetLatestCheckpoint`, `GetEvents`, and `ReconcileRepo` simultaneously against the same SQLite database. | - With SQLite WAL mode and busy timeout configured, 100% of operations succeed.<br>- Zero `database is locked` panics or database corruption.<br>- All event streams and checkpoints maintain transactional integrity. |

---

## 4. Unified Test Fixture Architecture (`internal/testutil`)

To eliminate code duplication across the entire test suite, `internal/testutil` must provide three core modules:

```
internal/testutil/
├── git.go        # Unified GitTestRepo with fluent API for repository simulation
├── db.go         # In-memory and temp SQLite database fixtures with WAL support
└── fixtures.go   # Pre-built Checkpoint, Event, and State test generators
```

### 4.1 Specification for `internal/testutil/git.go`
```go
package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// GitRepo encapsulates an isolated Git repository in a temporary directory.
type GitRepo struct {
	T       *testing.T
	Dir     string
	GitPath string
}

// NewGitRepo creates and initializes a fresh Git repository for testing.
func NewGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	gitPath := FindGitBinary()

	repo := &GitRepo{
		T:       t,
		Dir:     dir,
		GitPath: gitPath,
	}

	if _, err := repo.RunAllowError("init", "-b", "main"); err != nil {
		repo.Run("init")
	}
	repo.Run("config", "user.name", "Wake Test Agent")
	repo.Run("config", "user.email", "agent@wake.dev")
	repo.Run("config", "commit.gpgsign", "false")
	repo.Run("config", "core.quotepath", "false")

	return repo
}

// FindGitBinary discovers git executable across PATH and standard Windows locations.
func FindGitBinary() string {
	if p, err := exec.LookPath("git"); err == nil && p != "" {
		return p
	}
	for _, c := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "git"
}

// Run executes a git command and returns trimmed stdout, failing test on error.
func (r *GitRepo) Run(args ...string) string {
	r.T.Helper()
	cmd := exec.Command(r.GitPath, args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %v failed in %s: %v\nOutput: %s", args, r.Dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// RunAllowError executes a git command allowing non-zero exit codes.
func (r *GitRepo) RunAllowError(args ...string) (string, error) {
	r.T.Helper()
	cmd := exec.Command(r.GitPath, args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// WriteFile creates or overwrites a file relative to repository root.
func (r *GitRepo) WriteFile(relPath, content string) {
	r.T.Helper()
	fullPath := filepath.Join(r.Dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		r.T.Fatalf("MkdirAll failed for %s: %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		r.T.Fatalf("WriteFile failed for %s: %v", fullPath, err)
	}
}

// DeleteFile removes a file relative to repository root.
func (r *GitRepo) DeleteFile(relPath string) {
	r.T.Helper()
	fullPath := filepath.Join(r.Dir, filepath.FromSlash(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		r.T.Fatalf("DeleteFile failed for %s: %v", fullPath, err)
	}
}

// Commit stages all changes and creates a commit, returning the new commit hash.
func (r *GitRepo) Commit(message string) string {
	r.T.Helper()
	r.Run("add", "-A")
	r.Run("commit", "-m", message)
	return r.CurrentCommit()
}

// CurrentCommit returns HEAD commit hash.
func (r *GitRepo) CurrentCommit() string {
	r.T.Helper()
	return r.Run("rev-parse", "HEAD")
}

// CurrentBranch returns the active branch name.
func (r *GitRepo) CurrentBranch() string {
	r.T.Helper()
	return r.Run("rev-parse", "--abbrev-ref", "HEAD")
}

// CreateBranch creates and checks out a new branch.
func (r *GitRepo) CreateBranch(name string) {
	r.T.Helper()
	r.Run("checkout", "-b", name)
}

// Checkout switches to an existing branch or commit.
func (r *GitRepo) Checkout(nameOrCommit string) {
	r.T.Helper()
	r.Run("checkout", nameOrCommit)
}
```

### 4.2 Specification for `internal/testutil/db.go`
```go
package testutil

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/wake/wake/internal/db"
	_ "modernc.org/sqlite"
)

// NewTestDB creates a real SQLite database in t.TempDir() with migrations applied.
func NewTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(tmpDir)
	if err != nil {
		t.Fatalf("InitDB failed in testutil: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	return database, tmpDir
}
```

### 4.3 Specification for `internal/testutil/fixtures.go`
```go
package testutil

import (
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/state"
)

// MakeTestCheckpoint constructs a valid state.Checkpoint for test scenarios.
func MakeTestCheckpoint(taskID uuid.UUID, commit, branch, repoPath string) state.Checkpoint {
	if taskID == uuid.Nil {
		taskID = uuid.New()
	}
	return state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    repoPath,
		Branch:        branch,
		Commit:        commit,
		StateVersion:  1,
		EventPosition: 1,
		StateData: state.State{
			TaskID:       taskID,
			Objective:    "E2E Verification Task",
			Constraints:  make([]string, 0),
			Decisions:    make([]state.Decision, 0),
			Completed:    make([]string, 0),
			Remaining:    make([]string, 0),
			Blocked:      make([]state.Blocker, 0),
			DoNotRepeat:  make([]string, 0),
			LastVerified: commit,
			Confidence:   state.ConfidenceHigh,
		},
	}
}

// MakeEventStream generates a standard chronological sequence of test events.
func MakeEventStream(taskID uuid.UUID) []events.Event {
	if taskID == uuid.Nil {
		taskID = uuid.New()
	}
	now := time.Now().UTC()
	return []events.Event{
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.TaskStarted,
			Timestamp: now.Add(-5 * time.Minute),
			Payload:   map[string]interface{}{"objective": "Build E2E Harness"},
		},
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.ConstraintAdded,
			Timestamp: now.Add(-4 * time.Minute),
			Payload:   map[string]interface{}{"constraint": "auth/*"},
		},
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.DecisionMade,
			Timestamp: now.Add(-3 * time.Minute),
			Payload: map[string]interface{}{
				"id":          "DEC-01",
				"description": "Use Go 1.27 standard test runner",
				"status":      "ACTIVE",
			},
		},
		{
			ID:        uuid.New(),
			TaskID:    taskID,
			Type:      events.MilestoneCompleted,
			Timestamp: now.Add(-2 * time.Minute),
			Payload:   map[string]interface{}{"milestone": "Harness designed"},
		},
	}
}
```

---

## 5. Caveats
1. **Read-Only Scope**: In strict adherence to Explorer constraints, no source code, tests, or build files were modified in `cmd/` or `internal/`.
2. **Platform & Tooling**: Local test runs utilize the Go binary at `C:\Program Files\Go\bin\go.exe`. Git operations rely on Git 2.x on Windows.
3. **CGO / Race Detector**: Pure Go `modernc.org/sqlite` does not require CGO. However, `go test -race` requires a C toolchain (GCC/Clang) on Windows. Concurrency tests are therefore validated using Goroutine waitgroups and high-load stress testing.

---

## 6. Conclusion
The Wake codebase has a strong foundation for state reconciliation and Git status inspection, but currently suffers from:
1. Fragmented, duplicated test utilities across 4 packages.
2. An assertion sort order bug in `internal/git/adversarial_test.go:206`.
3. Total absence of test coverage for CLI commands `history` and `resume`, and the `internal/events` package.
4. An incomplete event reduction test matrix in `internal/state/engine_test.go`.

Implementing the unified `internal/testutil` harness and executing the comprehensive test suite across Tiers 1 through 4 will guarantee 100% automated test verification, zero `go vet` warnings, and rock-solid reliability for AI agent workflows.

---

## 7. Verification Method

### 7.1 Verify Test Bug in `internal/git`
Execute:
```powershell
& "C:\Program Files\Go\bin\go.exe" test -v ./internal/git -run TestAdversarial_FilenamesWithSpacesAndUnicode
```
*Expected Result*: Fails at line 206 due to `expectedModified` UTF-8 sort order mismatch.

### 7.2 Verify Existing Passing Test Suites
Execute:
```powershell
& "C:\Program Files\Go\bin\go.exe" test -v ./cmd ./internal/db ./internal/state ./internal/reconcile
```
*Expected Result*: All 30+ tests pass cleanly.

### 7.3 Verify Static Analysis
Execute:
```powershell
& "C:\Program Files\Go\bin\go.exe" vet ./...
```
*Expected Result*: Clean exit with 0 warnings.

### 7.4 Invalidation Conditions
- If `git status --porcelain=v1` output formatting changes, parser tests will fail.
- If SQLite schema alters table column definitions without database migration, `TestDB_InitAndMigrations` will fail.

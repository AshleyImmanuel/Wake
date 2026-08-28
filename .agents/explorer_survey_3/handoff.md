# Wake Testing & Verification Survey Report

## Executive Summary
This survey provides a comprehensive investigation of the testing architecture, test suites, simulation mechanisms, coverage gaps, static analysis readiness, and testability across the Wake codebase at `C:/Users/USER/Desktop/Sentinel`. 

Baseline test execution results:
- Statement coverage across packages:
  - `github.com/wake/wake/internal/reconcile`: **95.9%** (PASS)
  - `github.com/wake/wake/internal/git`: **87.7%** (FAIL — 1 test assertion bug)
  - `github.com/wake/wake/internal/db`: **84.3%** (PASS)
  - `github.com/wake/wake/internal/state`: **84.0%** (PASS)
  - `github.com/wake/wake/cmd`: **55.1%** (PASS — `history.go` and `resume.go` have 0% test coverage)
  - `github.com/wake/wake/internal/events`: **0.0%** (No test files)
- Static analysis with `go vet ./...`: **0 warnings, clean pass**.
- Single test failure in `internal/git/adversarial_test.go:206` due to UTF-8 byte sort order expectation mismatch in test fixture.

---

## 1. Observation

### 1.1 Complete Inventory of Existing Test Files and Test Functions
The codebase contains 10 test files across 4 packages (excluding `internal/events` which has 0 test files):

| Package | Test File | Line Count | Test Functions / Subtests | Pass / Fail |
|---|---|---|---|---|
| `cmd` | `cmd/checkpoint_test.go` | 134 | `TestCheckpoint_RunCheckpoint`<br>`TestCheckpoint_InvalidTargetDir`<br>`TestCheckpoint_InvalidTaskID` | PASS |
| `cmd` | `cmd/status_test.go` | 80 | `TestStatus_NoCheckpoint`<br>`TestStatus_WithCheckpoint` | PASS |
| `cmd` | `cmd/history_test.go` | 0 | *None (File does not exist)* | N/A (0% coverage) |
| `cmd` | `cmd/resume_test.go` | 0 | *None (File does not exist)* | N/A (0% coverage) |
| `internal/events` | *None* | 0 | *None (Package has no `*_test.go` files)* | N/A (0% coverage) |
| `internal/db` | `internal/db/db_test.go` | 195 | `TestDB_InitAndMigrations`<br>`TestDB_SaveAndGetLatestCheckpoint`<br>`TestDB_EventsPersistence`<br>`TestDB_NilDBErrors` | PASS |
| `internal/state` | `internal/state/engine_test.go` | 98 | `TestReduce_TaskStarted`<br>`TestReduce_BlockerLifecycle`<br>`TestReduce_MilestoneAndDecision` | PASS |
| `internal/git` | `internal/git/parser_test.go` | 194 | `TestParsePorcelainStatus_Clean`<br>`TestParsePorcelainStatus_Mixed`<br>`TestParsePorcelainStatus_UnmergedVariations`<br>`TestParseNameOnlyList`<br>`TestParseDiffNameStatus` | PASS |
| `internal/git` | `internal/git/client_test.go` | 401 | `TestClient_GetState_Normal`<br>`TestClient_GetState_EmptyRepo`<br>`TestClient_GetState_DetachedHead`<br>`TestClient_NotGitRepo`<br>`TestClient_DiffOperations`<br>`TestClient_CommitExistsAndAncestry`<br>`TestIntegration_RealGitRepositoryLifecycle` | PASS |
| `internal/git` | `internal/git/adversarial_test.go` | 562 | `TestAdversarial_EmptyRepoStates` (2 subtests)<br>`TestAdversarial_DetachedHeadMatrix` (2 subtests)<br>`TestAdversarial_FilenamesWithSpacesAndUnicode`<br>`TestAdversarial_FullConflictMatrix`<br>`TestAdversarial_DualStagedUnstagedCombinations`<br>`TestAdversarial_CommitValidationAndAncestry`<br>`TestAdversarial_ErrorClassificationEdgeCases` (8 subtests)<br>`TestAdversarial_ConcurrentClientUsage`<br>`TestAdversarial_ParserEdgeCases` (4 subtests) | **FAIL** (1 test failure) |
| `internal/git` | `internal/git/lifecycle_adversarial_test.go` | 315 | `TestIntegration_AdversarialLifecycle` (5 subtests: Empty repo, Spaces/Unicode/Renames, Unstaged deletions, Merge conflicts, Ancestry) | PASS |
| `internal/reconcile` | `internal/reconcile/engine_test.go` | 710 | `TestReconcile_SAFE_MatchingCommitAndClean`<br>`TestReconcile_STALE_ForwardCommits`<br>`TestReconcile_STALE_NonConflictingModifications`<br>`TestReconcile_STALE_UntrackedFiles`<br>`TestReconcile_CONFLICT_ConstraintViolation`<br>`TestReconcile_CONFLICT_DecisionViolation`<br>`TestReconcile_CONFLICT_CompletedOrDoNotRepeatModified`<br>`TestReconcile_CONFLICT_CompletedDeleted`<br>`TestReconcile_CONFLICT_MergeConflicts`<br>`TestReconcile_EdgeCases_EmptyRepo`<br>`TestReconcile_EdgeCases_EmptyCheckpointCommit`<br>`TestReconcile_EdgeCases_BranchMismatch`<br>`TestReconcile_EdgeCases_BranchHeadCompatibility`<br>`TestReconcile_EdgeCases_PathPrefixAndNormalization`<br>`TestReconcile_StagedRenamedFiles`<br>`TestNewEngine`<br>`TestReconcileRepo_MissingClaimedFile`<br>`TestReconcileRepo_DivergedCommit`<br>`TestReconcileRepo_CommitDoesNotExist`<br>`TestReconcileRepo_CommittedChangedFiles`<br>`TestReconcileRepo_ClientError` | PASS |
| `internal/reconcile` | `internal/reconcile/reconcile_test.go` | 558 | `TestReconciliationSuite_SAFE`<br>`TestReconciliationSuite_STALE_ForwardCommits`<br>`TestReconciliationSuite_STALE_TaskFilesModified`<br>`TestReconciliationSuite_CONFLICT_ConstraintViolation`<br>`TestReconciliationSuite_CONFLICT_DecisionViolation`<br>`TestReconciliationSuite_CONFLICT_DeletedMilestoneArtifact`<br>`TestReconciliationSuite_CONFLICT_MergeConflicts`<br>`TestReconciliationSuite_BranchMismatch`<br>`TestReconciliationSuite_DivergedHistory`<br>`TestReconciliationSuite_UntrackedFiles` | PASS |

---

### 1.2 Verbatim Test Failure and Static Analysis Output

#### Command: `go test -v ./internal/git`
```
=== RUN   TestAdversarial_FilenamesWithSpacesAndUnicode
    adversarial_test.go:206: ExtractModifiedFiles mismatch:
        expected: [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_日本語_test.txt unicode_üñîçødé_файл.md]
        got:      [deeply/nested/path with spaces/another file.go new name with space.txt normal_deleted.txt old name with space.txt path with spaces/my file.txt unicode_üñîçødé_файл.md unicode_日本語_test.txt]
--- FAIL: TestAdversarial_FilenamesWithSpacesAndUnicode (0.00s)
FAIL
coverage: 87.7% of statements
FAIL	github.com/wake/wake/internal/git	4.540s
```

#### Command: `go vet ./...`
```
Exit code: 0
Stdout: (empty)
Stderr: (empty)
```

---

### 1.3 How Tests Currently Simulate Subsystems

#### A. Git Simulation Mechanisms
1. **MockRunner (`internal/git/runner.go:97-145`)**:
   - Uses an in-memory struct holding `Responses map[string]MockResponse` guarded by `sync.Mutex`.
   - String key matching: `key := strings.Join(args, " ")`.
   - Used in `internal/git/client_test.go` and `internal/git/adversarial_test.go`.
   - **Limitation**: Fragile argument matching. If arguments have quotes or multi-word parameters (e.g. `-m "Initial commit"`), `strings.Join` flattens them into space-separated words which fail exact map lookups unless registered with identical space slicing.
2. **Real Temporary Git Repositories (`t.TempDir()`)**:
   - Initialized via `exec.Command(gitBin, "init", ...)` in `t.TempDir()`.
   - Used in:
     - `cmd/checkpoint_test.go:31-60` (`setupTempGitRepo`)
     - `cmd/status_test.go:13-41` (`setupTempGitRepoForStatus`)
     - `internal/git/client_test.go:222-400` (`TestIntegration_RealGitRepositoryLifecycle`)
     - `internal/git/lifecycle_adversarial_test.go:14-314` (`TestIntegration_AdversarialLifecycle`)
     - `internal/reconcile/reconcile_test.go:44-64` (`initGitTestRepo`)
   - **Duplication**: The logic to discover the Git binary on Windows and configure `user.name`, `user.email`, and `commit.gpgsign=false` is copy-pasted across 4 different test files under slightly different function names (`findGit`, `locateGitBinary`, `setupTempGitRepo`, `setupTempGitRepoForStatus`, `initGitTestRepo`).
3. **Mock Git Client (`internal/reconcile/engine_test.go:656-709`)**:
   - Implements `git.Client` with canned struct fields (`state`, `commitExists`, `isAncestor`, `changedFiles`, `err`).
   - Enables fast unit testing of `reconcile.ReconcileRepo` without spawning subprocesses.

#### B. Database Simulation Mechanisms
1. **Real SQLite Databases in `t.TempDir()`**:
   - All database tests (`internal/db/db_test.go`, `cmd/checkpoint_test.go`, `cmd/status_test.go`) invoke `db.InitDB(tmpDir)`, which creates `.sentinel/state.db` on disk and executes migration DDL.
   - Cleanup is handled by `t.TempDir()` and `defer db.Close()`.
2. **Missing In-Memory SQLite (`:memory:`) Support**:
   - `db.InitDB` hardcodes the path `filepath.Join(projectRoot, ".sentinel", "state.db")`.
   - There is no option to pass an in-memory DSN (e.g. `file::memory:?cache=shared`) or a pre-existing `*sql.DB` connection.
   - Consequently, every database test performs disk I/O, directory creation, and physical file locking.

#### C. State Transition Simulation Mechanisms
1. **Direct Event Slices**:
   - `internal/state/engine_test.go` constructs raw `[]events.Event` slices with manual payload maps and passes them to `state.Reduce(taskID, history)`.
   - Only 3 test cases exist, verifying `TaskStarted`, `BlockerCreated/Resolved`, and `MilestoneCompleted/DecisionMade`.
2. **State Checkpoint Synthetic Fixtures**:
   - `internal/reconcile/engine_test.go:16-37` provides `newTestCheckpoint(commit, branch)` to build synthetic `state.Checkpoint` structs.
   - Evaluated against synthetic `git.RepositoryState` structs with varied `StagedFiles`, `UnstagedFiles`, `ModifiedFiles`, `Constraints`, and `Decisions`.

---

## 2. Logic Chain: Analysis & Coverage Gap Mapping

```
Existing Test Suites
        │
        ├── 1. Test Bug: adversarial_test.go:206 (UTF-8 byte sort order mismatch)
        │
        ├── 2. Major Coverage Blindspots:
        │       ├── cmd/ (55.1%): history.go (0%) & resume.go (0%) completely untested
        │       ├── internal/events/ (0.0%): No tests for models or NewEvent
        │       ├── internal/state/ (84.0%): 10 of 17 event types untested & unhandled
        │       ├── internal/db/ (84.3%): No concurrency, locking, or transaction failure tests
        │       └── internal/git/ & internal/reconcile/: Missing edge cases (globs, case-sensitivity)
        │
        ├── 3. Simulation Infrastructure Deficiencies:
        │       ├── Duplicated Git binary finders & temp repo harnesses across 4 files
        │       ├── Hardcoded disk SQLite path (.sentinel/state.db) preventing in-memory testing
        │       └── Fragile argument string matching in MockRunner
        │
        └── 4. Static Analysis Readiness:
                ├── go vet passes cleanly (0 errors)
                ├── Race detector unavailable without CGO on Windows
                └── Unchecked errors in test helpers and CLI routines
```

### 2.1 Package-by-Package Coverage Gap Analysis

#### A. Package `cmd` (Statement Coverage: 55.1%)
- **Zero Coverage Commands**:
  - `cmd/history.go`: Completely untested. No tests verify event listing, formatted output, or error when no checkpoint exists.
  - `cmd/resume.go`: Completely untested. No tests verify recovery packet generation, goal rendering, completed milestone listings, blocker status alerts, or delta instructions.
- **Missing Edge & Error Cases**:
  - `cmd/checkpoint.go`:
    - Checkpoint creation in an empty repo (0 commits).
    - Failure when SQLite DB creation fails or is locked.
    - Checkpoint creation with custom `--dir` path pointing to subdirectory or relative path.
    - Checkpoint state version rollover and event position tracking across multi-turn sessions.
  - `cmd/status.go`:
    - JSON formatting validation (`--json` flag) for SAFE, STALE, and CONFLICT outputs.
    - Behavior when repo root is not found.
    - Behavior when git status fails mid-execution.
  - `cmd/root.go`:
    - Subcommand execution and help flag handling.

#### B. Package `internal/events` (Statement Coverage: 0.0%)
- **Gaps**:
  - No `*_test.go` exists.
  - `NewEvent` constructor has no unit tests.
  - JSON marshaling and unmarshaling round-trip tests for `Event` with diverse `Payload` structures are completely absent.
  - No validation tests for unknown or empty `EventType`.

#### C. Package `internal/state` (Statement Coverage: 84.0%)
- **Gaps**:
  - `state.Reduce` only handles 7 of 17 event types. The remaining 10 event types are completely untested:
    - `RequirementAdded`, `UserApproval`, `UserRejection`, `FileChanged`, `CommandExecuted`, `TestStarted`, `TestPassed`, `TestFailed`, `SessionInterrupted`, `SessionResumed`.
  - Unpopulated state fields: `Current`, `Remaining`, `DoNotRepeat`, and `NextAction` are never asserted because `Reduce` never assigns them.
  - Error and Boundary Gaps:
    - Empty event history (`[]events.Event{}`).
    - History with non-UUID task ID.
    - History containing events from multiple distinct task IDs.
    - Events with missing or malformed payload keys (e.g. `payload["objective"] = 123` instead of string).
    - Resolving a blocker ID that does not exist in `currentState.Blocked`.
    - Multiple `TaskStarted` events in a single stream (objective override behavior).

#### D. Package `internal/db` (Statement Coverage: 84.3%)
- **Gaps**:
  - **Concurrency & Locking**: Zero tests execute concurrent readers/writers against `*sql.DB`. `modernc.org/sqlite` under default journal mode without WAL will error on concurrent writes.
  - **Transaction Rollback**: No tests verify rollback behavior if a multi-operation batch fails.
  - **Migration Idempotency**: No test runs `migrate(db)` twice on an existing database to verify ALTER TABLE safety.
  - **Corrupted Data**: No test verifies behavior when `state_data` or `payload` contains invalid JSON in SQLite rows.
  - **Context Cancellation**: No test verifies that operations cleanly abort when `ctx.Done()` is closed.
  - **Scale / Stress**: No test tests retrieving thousands of events or hundreds of checkpoints.

#### E. Package `internal/git` (Statement Coverage: 87.7%)
- **Test Assertion Bug (`internal/git/adversarial_test.go:206`)**:
  - `ExtractModifiedFiles` sorts paths using Go's standard `sort.Strings(result)`.
  - In UTF-8:
    - `unicode_üñîçødé_файл.md` begins with `ü` (`0xC3 0xBC`).
    - `unicode_日本語_test.txt` begins with `日` (`0xE6 0x97 0xA5`).
    - `0xC3 < 0xE6`, so `unicode_ü...` is sorted before `unicode_日...`.
  - The test fixture erroneously hardcoded `unicode_日本語_test.txt` first, causing `TestAdversarial_FilenamesWithSpacesAndUnicode` to fail.
- **Gaps**:
  - Context timeout/cancellation during long-running Git commands (e.g. diff on huge repos).
  - Git operations in subdirectories vs repository root.
  - Git repositories with symlinks, submodules, or git worktrees (`.git` is a file, not a directory).
  - Git index lock collision (`.git/index.lock` exists).

#### F. Package `internal/reconcile` (Statement Coverage: 95.9%)
- **Gaps**:
  - **Windows Path Case Sensitivity**: Git paths are case-sensitive on Linux but case-insensitive on Windows NTFS. No tests verify reconciliation when file casing differs (e.g. `Readme.md` vs `README.md`).
  - **Stop-Word False Negatives**: In `engine.go:417-430`, stop-words include `"file"`, `"package"`, `"touch"`, `"code"`. If a real project file is named `internal/file/file.go` or `package.go`, constraint matching will strip the token and fail to detect a violation!
  - **Glob Pattern Boundaries**: Missing tests for recursive double-star globs (`**/*.ts`), character ranges (`[0-9]`), and negated patterns.
  - **Claimed Missing Files in Nested Subdirectories**: `ReconcileRepo` disk check when claimed path contains relative traversal segments (`../`).

---

### 2.2 Static Analysis & Code Hygiene Assessment

1. **`go vet` Results**:
   - Ran `go vet ./...` across the entire workspace. Returned exit code 0 with zero warnings. The AST and standard Go calling conventions are structurally sound.
2. **Race Detector Support**:
   - `go test -race` requires CGO and a C compiler (GCC/Clang). On Windows with pure Go `modernc.org/sqlite`, CGO is disabled by default. CI pipelines must ensure thread-safety through design reviews and mock concurrency tests (`TestAdversarial_ConcurrentClientUsage`).
3. **Error Handling & Ignored Return Values**:
   - In `cmd/checkpoint.go:106`: `_ = db.SaveEvent(ctx, database, commitEv)` ignores database error.
   - In `internal/reconcile/reconcile_test.go:446`: `_, _ = repo.runGitAllowError("merge", "feature-branch")` ignores potential unexpected failures.
   - In `internal/db/db.go:75-76`: `_, _ = db.Exec(...)` ignores ALTER TABLE error (which is intentional for schema migration backward compatibility, but should log or check sqlite error codes).

---

## 3. Caveats
1. **Read-Only Investigation**: No production code or test files were modified during this survey.
2. **Platform Specifics**: Test timings and Git binary resolution were verified on Windows 10/11 x64 with Go 1.27.0 and Git 2.x.
3. **Package Scope**: This survey covers all packages currently in the repository (`cmd`, `internal/events`, `internal/state`, `internal/git`, `internal/reconcile`, `internal/db`).

---

## 4. Conclusion
The Wake test suite demonstrates impressive depth in reconciliation evaluation logic (95.9% coverage) and Git porcelain status parsing, with strong integration tests that simulate Git repository states using temporary directories.

However, several critical testing deficiencies must be resolved to achieve enterprise-grade verification:
1. **Fix the failing UTF-8 test fixture** in `internal/git/adversarial_test.go:206`.
2. **Establish 100% test coverage for CLI commands** by adding `cmd/history_test.go` and `cmd/resume_test.go`.
3. **Create `internal/events/models_test.go`** for event creation and serialization.
4. **Expand `internal/state/engine_test.go`** to cover all 17 event types and boundary conditions.
5. **Consolidate duplicated Git test harnesses** into a unified test fixture package (`testutil` or `internal/testutil`).
6. **Implement an in-memory SQLite test strategy** allowing instant, diskless database testing.

---

## 5. Concrete Testing Architecture Recommendations

### 5.1 Four-Tier Testing Hierarchy

```
+-----------------------------------------------------------------------------------+
| TIER 4: End-to-End & Adversarial Scenarios (< 10s)                               |
| - Full Agent Session Interruption & Resume Cycles                                 |
| - Diverged Commit Trees, Real Merge Conflicts, Deleted Milestone Recovery         |
| - High-Concurrency DB Access & Lock Contention Tests                              |
+-----------------------------------------------------------------------------------+
                                          |
+-----------------------------------------------------------------------------------+
| TIER 3: Subsystem Integration Tests (< 2s)                                        |
| - Real Temp Git Repositories (Isolated t.TempDir())                               |
| - Real SQLite Database Migrations & Multi-Checkpoint Lifecycle                    |
| - Full CLI Command Execution with In-Memory Stdio Buffers                         |
+-----------------------------------------------------------------------------------+
                                          |
+-----------------------------------------------------------------------------------+
| TIER 2: Component Tests with In-Memory Mocks (< 200ms)                            |
| - Database Store with in-memory SQLite (:memory:?cache=shared)                    |
| - Git Client with Programmable MockRunner                                         |
| - Application Service Facade (internal/service) Unit Verification                |
+-----------------------------------------------------------------------------------+
                                          |
+-----------------------------------------------------------------------------------+
| TIER 1: Pure In-Memory Unit Tests (< 20ms)                                        |
| - Event Reducer (state.Reduce) Matrix across all 17 Event Types                   |
| - Porcelain Status & Diff Parsers with Pathological Inputs                        |
| - In-Memory Reconciliation Logic (reconcile.Reconcile)                             |
| - Event Serializers & Payload Type Validators                                     |
+-----------------------------------------------------------------------------------+
```

---

### 5.2 Unified Test Fixture Architecture (`internal/testutil`)

To eliminate code duplication across `cmd`, `internal/git`, and `internal/reconcile`, create a shared testing support package `internal/testutil`:

#### Proposed Structure:
```
internal/testutil/
├── git.go        # Unified GitTestRepo with fluent API
├── db.go         # In-memory and temp SQLite DB fixtures
└── fixtures.go   # Pre-built Checkpoint and Event generators
```

#### Code Specification for `internal/testutil/git.go`:
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

type GitRepo struct {
	T       *testing.T
	Dir     string
	GitPath string
}

func NewGitRepo(t *testing.T) *GitRepo {
	t.Helper()
	dir := t.TempDir()
	gitPath := FindGitBinary()

	repo := &GitRepo{T: t, Dir: dir, GitPath: gitPath}
	repo.Run("init", "-b", "main")
	repo.Run("config", "user.name", "Wake Tester")
	repo.Run("config", "user.email", "tester@wake.dev")
	repo.Run("config", "commit.gpgsign", "false")
	repo.Run("config", "core.quotepath", "false")
	return repo
}

func FindGitBinary() string {
	if p, err := exec.LookPath("git"); err == nil && p != "" {
		return p
	}
	candidates := []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "git"
}

func (r *GitRepo) Run(args ...string) string {
	r.T.Helper()
	cmd := exec.Command(r.GitPath, args...)
	cmd.Dir = r.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %v failed: %v\nOutput: %s", args, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func (r *GitRepo) WriteFile(relPath, content string) {
	r.T.Helper()
	fp := filepath.Join(r.Dir, filepath.FromSlash(relPath))
	_ = os.MkdirAll(filepath.Dir(fp), 0755)
	if err := os.WriteFile(fp, []byte(content), 0644); err != nil {
		r.T.Fatalf("WriteFile failed: %v", err)
	}
}

func (r *GitRepo) Commit(msg string) string {
	r.T.Helper()
	r.Run("add", "-A")
	r.Run("commit", "-m", msg)
	return r.Run("rev-parse", "HEAD")
}
```

---

### 5.3 Concrete Test Scenarios to Implement

#### Tier 1: State Reduction & Event Engine Scenarios
1. **Event Reduction Full Matrix**: Test all 17 event types individually and in interleaved chains.
2. **Confidence Adjustment**: Verify confidence drops to `ConfidenceLow` on `TestFailed` or `UserRejection`, and restores to `ConfidenceHigh` on `TestPassed` or `UserApproval`.
3. **Payload Type Safety**: Verify graceful handling of missing fields or wrong types without panic.
4. **Blocker Resolution Indexing**: Verify resolution of specific blockers in a list of 50+ blockers.

#### Tier 2: Database Store & Concurrency Scenarios
1. **In-Memory Store Tests**: Run full CRUD suite against `:memory:` database in < 5ms.
2. **Concurrent Readers and Writers**: Launch 20 parallel goroutines writing events and checkpoints to verify WAL mode and no `database is locked` errors.
3. **Atomic Multi-Entity Transactions**: Verify that if `SaveCheckpoint` fails, preceding `SaveEvent` within the same transaction is cleanly rolled back.

#### Tier 3: CLI Subcommand Scenarios
1. **`wake history` Test Suite (`cmd/history_test.go`)**:
   - `TestHistory_NoCheckpointFound`: Verifies error message when no checkpoints exist.
   - `TestHistory_WithEvents`: Verifies correct chronological output of 5+ events with timestamps.
   - `TestHistory_CustomTaskID`: Verifies filtering by specific task UUID.
2. **`wake resume` Test Suite (`cmd/resume_test.go`)**:
   - `TestResume_SafeState`: Verifies compact recovery packet with "Safe to resume" guidance.
   - `TestResume_StaleState`: Verifies recovery packet with changed files list and context reload instructions.
   - `TestResume_ConflictState`: Verifies conflict warnings, violated decisions, and manual review alert.
   - `TestResume_ActiveBlockers`: Verifies active blockers are highlighted while resolved ones are omitted.

#### Tier 4: End-to-End & Adversarial Scenarios
1. **Agent Crash & Recovery Lifecycle**:
   - Initialize task -> Create 3 checkpoints -> Simulate agent crash -> Execute `wake resume` -> Verify reconstructed state exactly matches checkpoint 3.
2. **Pathological File Paths & Unicode**:
   - Deeply nested directories (200+ characters), spaces in folder names, special UTF-8 characters (CJK, accented Cyrillic/Latin, emoji filenames).
3. **Diverged History & Detached HEAD Recovery**:
   - Create checkpoint on branch `feat/a` -> Switch to detached HEAD at prior commit -> Verify `wake status` detects divergence with `CONFLICT` status.

---

## 6. Verification Method

### How to Independently Verify This Report:
1. **Verify Test Failure in `internal/git`**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v ./internal/git -run TestAdversarial_FilenamesWithSpacesAndUnicode
   ```
   *Expected result*: Fails at `adversarial_test.go:206` due to UTF-8 sort order mismatch.

2. **Verify Static Analysis (`go vet`)**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" vet ./...
   ```
   *Expected result*: Exits cleanly with code 0.

3. **Verify Coverage Percentages**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -cover ./cmd ./internal/db ./internal/state ./internal/reconcile ./internal/git
   ```

4. **Verify Missing Test Files**:
   - Check `cmd/history_test.go` -> File does not exist.
   - Check `cmd/resume_test.go` -> File does not exist.
   - Check `internal/events/*_test.go` -> No test files exist.

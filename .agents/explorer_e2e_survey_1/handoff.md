# E2E Testing Track Survey Report: Wake Architecture & Verification Blueprint

## 1. Observation

### 1.1 Codebase & Build Environment
- **Go Toolchain**: Installed at `C:\Program Files\Go\bin\go.exe` (Go version 1.27.0).
- **Module Definition** (`go.mod`):
  - Module path: `github.com/wake/wake`
  - Core dependencies: `github.com/google/uuid v1.6.0`, `github.com/spf13/cobra v1.10.2`, `modernc.org/sqlite v1.57.0` (pure Go SQLite driver, CGO-free).
- **Entry Points & Orphaned Artifacts**:
  - `main.go:1-8`: Standard entry point invoking `cmd.Execute()`.
  - `simulate_conflict.go:1-32`: Orphaned file in the root workspace declaring `package main` with an unused import `time` and a second `main()` function. Running `go build .` or `go vet ./...` in the root workspace fails with `main redeclared in this block`. Running `go build -o wake.exe main.go` or `go vet ./cmd/... ./internal/...` compiles and vets with 0 warnings.
- **Pre-existing Tests & Known Issues**:
  - `internal/git/adversarial_test.go:206`: Test fails due to inverted UTF-8 string sort order in expected test slice (`unicode_日本語_test.txt` vs `unicode_üñîçødé_файл.md`). Go's byte sort places `\xc3` before `\xe6`.
  - All other tests in `cmd/`, `internal/db/`, `internal/state/`, and `internal/reconcile/` pass 100%.
  - `cmd/history_test.go` and `cmd/resume_test.go` do not yet exist in `cmd/`.

### 1.2 CLI Command Surface
All commands are registered under `rootCmd` (`wake`) in package `cmd`:

| Command | Arguments | Flags | Description & Output Formats | Exit Code |
|---|---|---|---|---|
| `wake` | None | None | Help / description banner. | 0 on success / 1 on error |
| `wake checkpoint` | None | `--task-id <uuid>`<br>`--objective <string>`<br>`--dir <path>` | Creates state checkpoint and commits git state to SQLite.<br>Output:<br>`[WAKE] Checkpoint created successfully.`<br>`Task ID: <uuid>`<br>`Commit: <hash>`<br>`Branch: <branch>`<br>`State Version: <int>`<br>`Working Tree: Clean \| <N> modified file(s)` | 0 on success<br>1 on invalid UUID, non-git dir, DB failure |
| `wake status` | None | `--task-id <uuid>`<br>`--dir <path>`<br>`--json` | Evaluates checkpoint against live git repository.<br>Text Output: Header banner, Task ID, Objective, Status (`[SAFE]`, `[STALE]`, `[CONFLICT]`), Confidence (`High`, `Low`, `None`), Repository State (Commits, Branch Match), Evaluation Summary (Counts, Violations, Claims), Guidance.<br>JSON Output: `{status, reason, checkpoint_commit, current_commit, branch_match, changed_files, task_related_changes, unrelated_changes, constraint_violations, invalidated_claims, confidence_level}`.<br>Special Case (No Checkpoint): Prints `WAKE STATUS: NO CHECKPOINT FOUND` (text) or `{"status": "UNKNOWN", "message": "..."}` (JSON). | 0 on success<br>1 on non-git dir, DB read error |
| `wake history` | None | `--task-id <uuid>`<br>`--dir <path>` | Queries chronological event stream for task.<br>Output:<br>`Event History for Task: <uuid>`<br>`--------------------------------------------------`<br>`[15:04:05] <EventType>`<br>`Total Events: <int>` | 0 on success<br>1 if no active task found or DB error |
| `wake resume` | None | `--task-id <uuid>`<br>`--dir <path>` | Generates AI recovery packet.<br>Output:<br>`RESUMING TASK: <uuid>`<br>`GOAL`, `COMPLETED`, `CURRENT`, `BLOCKERS`, `CONSTRAINTS`, `DO NOT REPEAT`, `LAST VERIFIED`, `NEXT ACTION`, `STATE CONFIDENCE`, `--- CURRENT REPOSITORY DELTA ---`, `RECOVERY INSTRUCTION`. | 0 on success<br>1 if checkpoint not found or reconcile fails |

### 1.3 Database Architecture & Storage Mechanism
- **Location**: `<repoRoot>/.sentinel/state.db` (initialized automatically on first run).
- **Directory Protection**: `InitDB` creates `<repoRoot>/.sentinel/.gitignore` containing `*\n` to prevent SQLite database files from ever leaking into user Git commits.
- **Driver**: Pure-Go SQLite (`modernc.org/sqlite`).
- **Schema**:
  1. `events`:
     - `id` (TEXT PRIMARY KEY): Event UUID.
     - `task_id` (TEXT NOT NULL): Associated Task UUID.
     - `type` (TEXT NOT NULL): EventType string (`TASK_STARTED`, `REQUIREMENT_ADDED`, etc.).
     - `timestamp` (DATETIME DEFAULT CURRENT_TIMESTAMP): RFC3339 timestamp.
     - `payload` (TEXT NOT NULL): JSON serialized `map[string]interface{}`.
  2. `checkpoints`:
     - `id` (TEXT PRIMARY KEY): Checkpoint UUID.
     - `task_id` (TEXT NOT NULL): Associated Task UUID.
     - `timestamp` (DATETIME DEFAULT CURRENT_TIMESTAMP): RFC3339 timestamp.
     - `commit_hash` (TEXT NOT NULL): Git HEAD commit hash.
     - `state_version` (INTEGER NOT NULL): Incremental version number (1, 2, ...).
     - `event_position` (INTEGER NOT NULL): Total event count at snapshot.
     - `state_data` (TEXT NOT NULL): JSON serialized `state.State` struct.
     - `repository` (TEXT DEFAULT ''): Absolute repository path.
     - `branch` (TEXT DEFAULT ''): Branch name.

### 1.4 Git Operations & Reconciliation Engine
- **Git Execution**: `internal/git/runner.go` executes `git` CLI subprocesses with context cancellation, timeout support, and error classification into domain sentinels (`ErrNotGitRepo`, `ErrNoCommits`, `ErrInvalidCommit`, `ErrGitLockExists`, `ErrDubiousOwnership`, `ErrMergeConflict`).
- **Reconciliation Status Matrix** (`internal/reconcile/engine.go`):
  1. `CONFLICT` (Confidence: `None`):
     - Merge conflicts present in working tree (`HasMergeConflicts == true` or `len(UnmergedFiles) > 0`).
     - Constraint violation: Any modified, staged, unstaged, or untracked file matches any constraint in `State.Constraints`.
     - Decision violation: Any modified file matches an active decision in `State.Decisions`.
     - Claim invalidation: Any file listed in `State.Completed` or `State.DoNotRepeat` is modified, deleted in git status, or physically missing from disk (`os.Stat` fails).
     - History divergence: Checkpoint commit is not an ancestor of current repository HEAD (`git merge-base --is-ancestor` fails).
     - Non-existent commit: Checkpoint commit hash does not exist in local object database (`git cat-file -e` fails).
     - Repository has 0 commits while checkpoint references a commit.
  2. `SAFE` (Confidence: `High`):
     - Working tree is completely clean (0 staged, 0 unstaged, 0 untracked, 0 unmerged files).
     - Current commit matches checkpoint commit (`repo.CommitHash == cp.Commit`).
     - Branch matches (`repo.Branch == cp.Branch` or either is `HEAD`).
     - 0 constraint violations, 0 claim invalidations, all claimed files physically exist on disk.
  3. `STALE` (Confidence: `Low`):
     - Forward commits on linear branch without constraint violations (current commit is descendant of checkpoint commit).
     - Uncommitted modified/untracked files present that do NOT violate constraints, decisions, or completed milestones.
     - Branch mismatch where commit history has not diverged.

---

## 2. Logic Chain

1. **Premise**: E2E testing must validate the system as an opaque box from the perspective of an external AI agent or developer invoking the CLI.
2. **Observation from CLI Execution**: All CLI commands rely strictly on standard CLI flags (`--task-id`, `--dir`, `--objective`, `--json`), read repository state by shelling out to `git`, and persist/read state in `<repoRoot>/.sentinel/state.db`.
3. **Inference for Test Fixtures**:
   - Every E2E test case can create a clean, fully isolated temporary directory via `t.TempDir()`.
   - The test fixture initializes a real Git repository (`git init`, `git config user.name`, `git config user.email`, `git config commit.gpgsign false`).
   - The test fixture executes the compiled `wake` binary (or calls the CLI command runner) pointing `--dir <tempDir>`.
4. **Observation on Compilation**:
   - `simulate_conflict.go` in the workspace root conflicts with `main.go` when building package `.`.
   - Building `go build -o <tempDir>/wake.exe main.go` succeeds cleanly.
5. **Inference for Test Harness**:
   - The E2E test harness should compile the `wake` binary once at `TestMain` or suite initialization into a temporary test bin folder, then invoke `exec.Command(binaryPath, args...)`.
   - For fast in-process tests, the harness can also invoke `cmd.Execute()` with redirected buffers.
6. **Inference for Assertion Strategy**:
   - **CLI Exit Code**: Check for expected exit code 0 or 1.
   - **CLI Stdout / Stderr**:
     - For `--json` status calls: Unmarshal JSON into `reconcile.ReconciliationResult` and assert exact status (`SAFE`, `STALE`, `CONFLICT`), confidence level, changed files, and violation messages.
     - For text status/resume/checkpoint/history calls: Validate human-readable guidance, task ID, state version, and section headers.
   - **State Persistence (Grey-box verification)**: Open `.sentinel/state.db` using standard SQLite queries to confirm rows in `events` and `checkpoints` tables match expected payloads and versions.

---

## 3. Caveats

- **External Git Dependency**: E2E tests executing real git commands depend on `git` being available on PATH or in standard Windows installation directories (`C:\Program Files\Git\cmd\git.exe`). The test harness must include a helper to locate `git` or skip if unavailable.
- **File System Timestamps**: Git status and SQLite timestamps operate with second/millisecond precision. Rapid consecutive test operations should ensure filesystem flushes occur before running git status or checkpointing.
- **Root Orphan File**: `simulate_conflict.go` in the root workspace must be handled by building `main.go` explicitly rather than `./...` until Track 2 removes/relocates it.

---

## 4. Conclusion & Recommendations for E2E Testing Architecture

### 4.1 Test Directory Structure & Layout
The E2E test suite should be placed in a dedicated test suite directory following Go conventions:

```
C:/Users/USER/Desktop/Sentinel/
├── e2e/
│   ├── e2e_test.go               # TestMain, compilation fixture, global helpers
│   ├── harness_test.go           # Temporary Git repo & CLI runner utilities
│   ├── tier1_features_test.go    # Tier 1: 5+ tests per feature across 12 features (60+ tests)
│   ├── tier2_boundaries_test.go  # Tier 2: 5+ tests per feature corner cases (60+ tests)
│   ├── tier3_cross_feature_test.go # Tier 3: Pairwise lifecycle transitions
│   └── tier4_scenarios_test.go   # Tier 4: Real-world AI agent workflows & disaster recovery
```

### 4.2 Comprehensive 4-Tier Test Matrix Design

#### Tier 1: Feature Coverage (>= 5 tests per feature)
1. **Feature 1 (Test Harness & Fixtures)**:
   - F1.1: Initialize isolated git repository with clean commit.
   - F1.2: Initialize git repo with custom default branch names (`main`, `master`, `trunk`, `release`).
   - F1.3: Create and verify SQLite database `.sentinel/state.db` with `.gitignore`.
   - F1.4: Verify temporary repository cleanup after test completion.
   - F1.5: Verify multi-threaded test isolation across separate temp directories.
2. **Feature 2 (UTF-8 & Unicode Paths)**:
   - F2.1: Checkpoint repository containing UTF-8 filenames with Japanese characters (`日本語.txt`).
   - F2.2: Checkpoint repository with European accented characters (`üñîçødé.go`).
   - F2.3: Checkpoint repository with spaces and special characters in paths (`path with spaces/file #1.go`).
   - F2.4: Status check with Unicode modified files preserving correct lexicographical byte sort.
   - F2.5: Resume command correctly displaying Unicode changed paths in delta section.
3. **Feature 3 (Event Payloads & Typing)**:
   - F3.1: Record `TaskStarted` event with objective payload and verify persistence.
   - F3.2: Record `ConstraintAdded` event and verify payload in database.
   - F3.3: Record `DecisionMade` event with active status and source.
   - F3.4: Record `MilestoneCompleted` event with completed artifact path.
   - F3.5: Record `BlockerCreated` and `BlockerResolved` lifecycle events.
4. **Feature 4 (17-Event State Reduction)**:
   - F4.1: Reduce full sequence from `TaskStarted` to `MilestoneCompleted`.
   - F4.2: Reduce `BlockerCreated` followed by `BlockerResolved` verifying resolution state.
   - F4.3: Reduce multiple constraints and verify deduplication.
   - F4.4: Reduce decisions with conflicting active vs rejected statuses.
   - F4.5: Reduce `GitCommit` events and verify `LastVerified` hash updates.
5. **Feature 5 (Incremental Event Folding & Versioning)**:
   - F5.1: Create checkpoint 1 -> verify StateVersion = 1.
   - F5.2: Add events and create checkpoint 2 -> verify StateVersion = 2, EventPosition = 2.
   - F5.3: Multiple consecutive checkpoints with interleaved commits.
   - F5.4: Checkpoint without new events maintaining state consistency.
   - F5.5: Checkpoint with updated explicit objective via `--objective` flag.
6. **Feature 6 (Database Store & Schemas)**:
   - F6.1: Verify DDL execution and table creation (`checkpoints`, `events`).
   - F6.2: Verify `.gitignore` creation inside `.sentinel`.
   - F6.3: Query latest checkpoint by specific Task UUID.
   - F6.4: Query latest checkpoint globally when Task UUID is empty.
   - F6.5: Verify query returns `sql.ErrNoRows` / appropriate error when table is empty.
7. **Feature 7 (Atomic Transactions & DB Integrity)**:
   - F7.1: Verify atomic checkpoint creation with recorded commit event.
   - F7.2: Verify rollback behavior on simulated insertion error.
   - F7.3: Verify concurrent read access during checkpoint write.
   - F7.4: Verify recovery when database is closed and reopened across CLI invocations.
   - F7.5: Verify database integrity check (`PRAGMA integrity_check`).
8. **Feature 8 (Git Invocations & Status Extraction)**:
   - F8.1: Extract clean repository state with matching commit hash.
   - F8.2: Extract unstaged modifications across multiple directories.
   - F8.3: Extract staged files (added, modified, renamed).
   - F8.4: Extract untracked files with nested subdirectories.
   - F8.5: Extract detached HEAD state and branch name fallback.
9. **Feature 9 (Reconciler Delimiter & Matching Optimization)**:
   - F9.1: Match exact file constraint (`auth/jwt.go`).
   - F9.2: Match directory prefix constraint (`internal/db/*` and `auth/`).
   - F9.3: Match glob constraint (`*.sql` and `config/*.json`).
   - F9.4: Match natural language constraint with stop words (`Do not touch auth files`).
   - F9.5: Match active decision constraint (`Protect billing module`).
10. **Feature 10 (Filesystem Physical Check)**:
    - F10.1: Claimed `Completed` file exists on disk -> returns SAFE.
    - F10.2: Claimed `Completed` file deleted from disk -> returns CONFLICT with invalidated claim.
    - F10.3: Claimed `DoNotRepeat` file deleted from disk -> returns CONFLICT.
    - F10.4: Claimed file recreated after deletion -> returns SAFE.
    - F10.5: Multiple claimed files with mixed presence on disk.
11. **Feature 11 (Application Service Orchestration)**:
    - F11.1: Service facade checkpoint creation workflow.
    - F11.2: Service facade status inquiry workflow.
    - F11.3: Service facade history retrieval workflow.
    - F11.4: Service facade resume packet generation workflow.
    - F11.5: Service facade error handling on missing repo or database.
12. **Feature 12 (CLI Decoupling & Expanded Command Coverage)**:
    - F12.1: `wake checkpoint` CLI execution with standard flags.
    - F12.2: `wake status` text output formatting and section structure.
    - F12.3: `wake status --json` valid JSON schema and deserialization.
    - F12.4: `wake history` CLI execution and event list formatting.
    - F12.5: `wake resume` CLI execution and recovery packet section formatting.

#### Tier 2: Boundary & Corner Cases (>= 5 tests per feature)
- Deeply nested directory trees (20+ path segments).
- Massive commit messages and long constraint descriptions (10KB+ strings).
- Repositories with 0 commits (freshly initialized git repository before initial commit).
- Checkpoints referencing non-existent commit hashes (simulated rebase / commit amend).
- Untracked files inside ignored directories (`.gitignore` edge cases).
- Git index lock file present (`.git/index.lock`).
- Symlinks pointing to non-existent targets (where OS supported).
- Concurrent CLI processes accessing same `.sentinel/state.db`.
- Database opened in read-only filesystem or restricted permissions.
- CLI invocations with invalid UUID formats, whitespace strings, empty flags.

#### Tier 3: Cross-Feature Pairwise Matrix
- **CP + ST + BR**: Checkpoint on `main` -> switch to `feature` branch -> `status` returns STALE (branch mismatch) -> switch back to `main` -> `status` returns SAFE.
- **CP + MOD + ST**: Checkpoint -> edit non-constraint file -> `status` returns STALE -> edit constraint file -> `status` returns CONFLICT.
- **CP + MERGE + ST**: Checkpoint -> trigger git merge conflict (`UU` state) -> `status` returns CONFLICT (unresolved merge conflicts).
- **CP + REBASE + ST**: Checkpoint on branch A -> rebase/diverge branch history -> `status` returns CONFLICT (diverged history).
- **CP + DEL + RESUME**: Checkpoint with completed milestone -> delete milestone file -> `resume` reports CONFLICT with recovery instruction.
- **CP + HIST + RESUME**: Create multi-event lifecycle across multiple checkpoints -> `history` lists full chronological audit log -> `resume` accurately displays current goal and blockers.

#### Tier 4: Real-World AI Agent Workloads & Recovery Scenarios
1. **Scenario 1: Full Multi-Turn AI Coding Agent Lifecycle**:
   - Step 1: Agent starts task: `wake checkpoint --task-id <id> --objective "Build User Auth"`.
   - Step 2: Agent writes code, runs tests, creates checkpoint 2: `wake checkpoint --task-id <id>`.
   - Step 3: Session interrupts (simulated context crash).
   - Step 4: New agent session boots up, runs `wake resume --task-id <id>`, parses Goal, Completed items, Next Action, Delta.
   - Step 5: Agent continues work, completes task, creates final checkpoint.
2. **Scenario 2: Developer Intervention & Conflict Interruption**:
   - Step 1: Agent creates checkpoint with constraint `Do not touch config/prod.json`.
   - Step 2: Human developer edits `config/prod.json` while agent is idle.
   - Step 3: Agent runs `wake status --json`, detects `CONFLICT` with `ConstraintViolation`.
   - Step 4: Agent halts and prompts developer for review rather than overwriting human edits.
3. **Scenario 3: Diverged Upstream Branch Recovery**:
   - Step 1: Agent checkpoints on topic branch.
   - Step 2: Upstream force push / rebase alters commit history.
   - Step 3: Agent runs `wake status`, detects ancestry break, flags CONFLICT with clear reason.
4. **Scenario 4: Multi-Task Switching in Single Repository**:
   - Step 1: Task A created with UUID 1.
   - Step 2: Task B created with UUID 2.
   - Step 3: Agent switches between Task A and Task B; verify `wake history --task-id <id>` and `wake resume --task-id <id>` maintain strict task isolation.
5. **Scenario 5: Disaster Recovery from Event Stream**:
   - Step 1: Working tree files accidentally corrupted or wiped.
   - Step 2: Agent reconstructs goal, constraints, and decisions from `.sentinel/state.db` via `wake history` and `wake resume`.

---

## 5. Verification Method

To independently verify the observations and findings documented in this report:

1. **Verify Go Compiler Location**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" version
   ```
2. **Verify Building the CLI Binary**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" build -o wake_test.exe main.go
   .\wake_test.exe --help
   Remove-Item wake_test.exe
   ```
3. **Verify Package Unit & Integration Tests**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v github.com/wake/wake/cmd
   & "C:\Program Files\Go\bin\go.exe" test -v github.com/wake/wake/internal/db
   & "C:\Program Files\Go\bin\go.exe" test -v github.com/wake/wake/internal/state
   & "C:\Program Files\Go\bin\go.exe" test -v github.com/wake/wake/internal/reconcile
   ```
4. **Verify Known Test Gap in Git Package**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" test -v github.com/wake/wake/internal/git -run TestAdversarial_FilenamesWithSpacesAndUnicode
   ```
5. **Verify Package Vet**:
   ```powershell
   & "C:\Program Files\Go\bin\go.exe" vet ./cmd/... ./internal/...
   ```

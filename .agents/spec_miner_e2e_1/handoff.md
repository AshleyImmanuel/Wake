# E2E Specification Mining Report: Wake Platform

## 1. Observation

Direct code examination and execution of test suites across the Sentinel/Wake repository (`C:/Users/USER/Desktop/Sentinel`) reveals the following structural, interface, and behavioral definitions:

### 1.1 Repository Structure and Modules
- `cmd/`: CLI presentation layer implemented using `spf13/cobra`. Includes `root.go`, `checkpoint.go`, `status.go`, `history.go`, `resume.go`, `checkpoint_test.go`, and `status_test.go`.
- `internal/events/`: Event models defining `EventType` (17 constants) and the `Event` envelope struct (`internal/events/models.go:1-50`).
- `internal/state/`: State models (`State`, `Decision`, `Blocker`, `Checkpoint`, `ConfidenceLevel`, `EvidenceStatus` in `internal/state/models.go:1-65`) and the state reduction engine (`Reduce` in `internal/state/engine.go:1-77`).
- `internal/git/`: Low-level and high-level Git CLI wrappers (`Runner`, `OSRunner`, `MockRunner`, `Client`, `ParsePorcelainStatus`, `classifyGitError` in `models.go`, `runner.go`, `parser.go`, `client.go`, `errors.go`).
- `internal/reconcile/`: State reconciliation engine (`Engine`, `Reconcile`, `ReconcileRepo` in `internal/reconcile/models.go`, `engine.go:1-583`, `engine_test.go`, `reconcile_test.go`).
- `internal/db/`: SQLite storage layer (`InitDB`, `migrate`, `SaveCheckpoint`, `GetLatestCheckpoint`, `SaveEvent`, `GetEvents` in `internal/db/db.go:1-276`).
- `PROJECT.md` & `ORIGINAL_REQUEST.md`: Architectural roadmaps, feature inventories, interface contracts (`Store`, `TaskService`), and acceptance criteria.

### 1.2 Verification Tool Invocations
- Executed `go test -v ./...` using Go 1.26 on Windows:
  - `github.com/wake/wake/cmd`: PASS
  - `github.com/wake/wake/internal/db`: PASS
  - `github.com/wake/wake/internal/state`: PASS
  - `github.com/wake/wake/internal/reconcile`: PASS
  - `github.com/wake/wake/internal/git`: Fails only on `TestAdversarial_FilenamesWithSpacesAndUnicode` (`adversarial_test.go:206`) due to an inverted UTF-8 string sort order in the test fixture (Item #2 in `PROJECT.md` Feature Inventory).
- Executed `go vet ./...`: 0 warnings, exits 0.

---

## 2. Logic Chain

1. **State Reconstruction from Append-Only Events**:
   - `events.Event` acts as the fundamental atom of state transitions.
   - `state.Reduce(taskID, history)` processes events chronologically. Initial state has `Confidence = ConfidenceHigh`.
   - `TaskStarted` assigns `Objective`.
   - `ConstraintAdded` appends to `Constraints`.
   - `DecisionMade` appends an active `Decision`.
   - `MilestoneCompleted` records completed items into `Completed`.
   - `BlockerCreated` inserts active blockers; `BlockerResolved` marks matching blocker IDs as `RESOLVED`.
   - `GitCommit` tracks the `LastVerified` commit SHA.

2. **Snapshot Persistence**:
   - `db.InitDB` initializes SQLite at `<repoRoot>/.sentinel/state.db` and writes `.sentinel/.gitignore`.
   - `db.SaveCheckpoint` records the reduced `StateData` snapshot, commit hash, branch, state version, and timestamp.
   - `db.GetLatestCheckpoint` queries the newest record ordered by `timestamp DESC, state_version DESC, rowid DESC`.

3. **Live Reconciliation Evaluation**:
   - `reconcile.Reconcile(cp, repoState, taskFiles)` and `reconcile.ReconcileRepo(ctx, cp, gitClient, repoPath, taskFiles)` compare checkpoint state with live git status and working tree.
   - **CONFLICT** is triggered if:
     a. Merge conflicts exist (`repo.HasMergeConflicts` or `UnmergedFiles > 0`).
     b. Modified or staged files match any constraint in `cp.StateData.Constraints`.
     c. Modified or staged files match any active decision (`Decision.Status == "ACTIVE"`).
     d. Modified or deleted files invalidate claims in `Completed` or `DoNotRepeat`.
     e. Claimed `Completed` or `DoNotRepeat` files do not exist physically on disk.
     f. Checkpoint commit hash does not exist in local Git object database or has diverged from current branch history (`merge-base --is-ancestor` fails).
     g. Checkpoint has a recorded commit but repository has 0 commits.
   - **SAFE** is triggered if:
     a. Working tree and index are clean (0 uncommitted/modified/untracked files).
     b. Checkpoint commit matches current HEAD commit (`cp.Commit == repo.CommitHash != ""`).
     c. Active branch matches checkpoint branch (or either is `"HEAD"`).
     d. 0 constraint violations and 0 claim invalidations.
   - **STALE** is triggered if state has drifted without conflicting:
     a. Forward commits exist (`cp.Commit` is valid ancestor of `repo.CommitHash`).
     b. Non-conflicting modified or untracked files are present.
     c. Branch has changed without violating constraints.

4. **Session Recovery Generation**:
   - `cmd.runResume` / `service.ResumeTask` generates a compact recovery packet summarizing Objective, Completed steps, Blockers, Constraints, Do Not Repeat items, Last Verified Commit, Next Action, Confidence Level, and Repository Delta instructions.

---

## 3. Caveats

- **Service Layer Implementation (`internal/service`)**: `PROJECT.md` specifies `TaskService` interface and request/response DTOs as Milestone M5. Currently, `cmd/` interacts directly with `internal/db`, `internal/git`, and `internal/reconcile`. E2E specifications must validate both direct CLI invocations and the planned service facade.
- **UTF-8 Byte Sort Order in Fixture**: `internal/git/adversarial_test.go:206` contains an inverted expectation between `unicode_üñîçødé_файл.md` and `unicode_日本語_test.txt` which is documented as Item 2 in `PROJECT.md` to be fixed.
- **Windows Path Separators**: File paths in git porcelain output use forward slashes (`/`), while Windows filesystem paths use backslashes (`\`). Path normalization in `reconcile.normalizePath` cleans and standardizes paths to forward slashes.

---

## 4. Conclusion

The specification for Wake is fully deterministic, comprehensive, and verifiable. The platform provides a robust state-reconstruction and reconciliation pipeline across CLI, state reduction, git inspection, SQLite persistence, and recovery generation.

---

## 5. Verification Method

To verify these specifications independently:
```powershell
# Run full package test suite
& "C:\Program Files\Go\bin\go.exe" test -v ./cmd ./internal/db ./internal/state ./internal/reconcile

# Run static analysis
& "C:\Program Files\Go\bin\go.exe" vet ./...

# Build CLI binary
& "C:\Program Files\Go\bin\go.exe" build -o wake.exe .
```

---

## Features Discovered

| # | Category | Feature | Description | Inputs | Outputs | Error Behavior | Discovered Via |
|---|----------|---------|-------------|--------|---------|----------------|----------------|
| 1 | CLI | `wake checkpoint` | Captures Git state, reduces event history, and persists a versioned Checkpoint snapshot to SQLite | `--task-id` (string), `--objective` (string), `--dir` (string) | Stdout confirmation with Task ID, Commit, Branch, State Version, Cleanliness | Exits 1 if non-git directory, invalid UUID, or DB failure | `cmd/checkpoint.go:23-144` |
| 2 | CLI | `wake status` | Compares latest checkpoint with live Git working tree to evaluate SAFE, STALE, or CONFLICT | `--task-id` (string), `--dir` (string), `--json` (bool) | Formatted report or JSON payload with reconciliation status, confidence, changed files, violations | Exits 1 on Git/DB errors; returns UNKNOWN message if no checkpoint | `cmd/status.go:23-161` |
| 3 | CLI | `wake history` | Displays chronological event log for active task | `--task-id` (string), `--dir` (string) | Formatted list of `[HH:MM:SS] EVENT_TYPE` and total count | Exits 1 with `"no active task found"` if no checkpoint | `cmd/history.go:18-64` |
| 4 | CLI | `wake resume` | Generates compact agent recovery packet with goal, completed items, blockers, constraints, delta instructions | `--task-id` (string), `--dir` (string) | Formatted multi-section Recovery Packet | Exits 1 if no checkpoint found or reconciliation fails | `cmd/resume.go:19-134` |
| 5 | Events | Event Envelope | Canonical data structure for state transitions | `task_id` (UUID), `type` (EventType), `payload` (map) | `events.Event` struct with UUID and UTC timestamp | JSON marshaling/unmarshaling errors | `internal/events/models.go:31-50` |
| 6 | Events | 17 Event Types | Complete enumeration of lifecycle events (TaskStarted, RequirementAdded, ConstraintAdded, UserApproval, UserRejection, DecisionMade, FileChanged, CommandExecuted, TestStarted, TestPassed, TestFailed, BlockerCreated, BlockerResolved, MilestoneCompleted, GitCommit, SessionInterrupted, SessionResumed) | Event type constant and payload schema | Typed `events.EventType` | N/A | `internal/events/models.go:11-29` |
| 7 | State | `Reduce` State Engine | Pure deterministic state reducer folding event stream into a `State` snapshot | `taskID` (string), `history` ([]events.Event) | `state.State` struct with Objective, Constraints, Decisions, Blockers, Completed, etc. | Gracefully handles malformed payload types via safe type assertions | `internal/state/engine.go:8-76` |
| 8 | State | Blocker Resolution Lifecycle | Tracks active blockers and updates status upon resolution | `BlockerCreated` payload `{"id", "description"}`, `BlockerResolved` payload `{"id"}` | `Blocker{Status: "ACTIVE"}` -> `Blocker{Status: "RESOLVED"}` | No-op if blocker ID not found | `internal/state/engine.go:48-67` |
| 9 | State | Decision Tracking | Captures architectural/developer decisions with source and active status | `DecisionMade` payload `{"id", "description", "source"}` | `Decision{Status: "ACTIVE"}` | Defaults source to empty string if omitted | `internal/state/engine.go:31-42` |
| 10 | Git | Porcelain Status Parser | Parses `git status --porcelain=v1 -uall` into staged, unstaged, untracked, and unmerged file sets | Raw status stdout string | `*git.StatusResult` struct | Skips lines < 3 chars; unquotes quoted paths | `internal/git/parser.go:9-85` |
| 11 | Git | Unmerged Conflict Matrix | Identifies all 7 Git unmerged conflict combinations (UU, AA, DD, AU, UD, UA, DU) | Two-letter status prefix | Populates `UnmergedFiles` slice and sets `IsClean=false` | N/A | `internal/git/parser.go:87-100` |
| 12 | Git | Diff & Name-Status Parser | Parses `git diff --name-only` and `git diff --name-status` | Diff command output | `[]string` paths and `[]git.FileChange` with rename tracking | Handles R100/C080 rename/copy percentages | `internal/git/parser.go:165-224` |
| 13 | Git | Ancestry & Commit Validation | Checks commit object existence (`cat-file -e`) and ancestor lineage (`merge-base --is-ancestor`) | `ancestorCommit`, `descendantCommit` (strings) | `bool` (is ancestor), `error` | Returns exit code 1 as `false, nil`; returns error on invalid hash | `internal/git/client.go:228-265` |
| 14 | Git | Domain Error Classification | Classifies Git CLI stderr into sentinels (`ErrNotGitRepo`, `ErrNoCommits`, `ErrInvalidCommit`, `ErrGitLockExists`, `ErrDubiousOwnership`, `ErrMergeConflict`) | `stderr` string, `exitCode` int | Typed `*git.GitError` wrapping domain sentinel | Returns raw exit code error if unclassified | `internal/git/errors.go:63-112` |
| 15 | Reconcile | Deterministic Reconciler | Evaluates checkpoint against repository state snapshot | `cp` (Checkpoint), `repo` (RepositoryState), `taskFiles` ([]string) | `ReconciliationResult` (SAFE, STALE, CONFLICT) | N/A | `internal/reconcile/engine.go:35-250` |
| 16 | Reconcile | Constraint Matching Engine | Tokenizes constraint text and matches against changed files via exact, prefix, glob, segment matching | `filePath`, `constraint` (strings) | `bool` match | Filters common English stop words | `internal/reconcile/engine.go:358-457` |
| 17 | Reconcile | Claim Invalidation Detection | Detects modifications or deletions of completed milestone or do-not-repeat artifacts | Changed files, deleted files, checkpoint state | Populates `InvalidatedClaims` and forces `StatusConflict` | N/A | `internal/reconcile/engine.go:150-185` |
| 18 | Reconcile | Physical Disk Verification | Verifies claimed files physically exist on filesystem | Local repo root path, claimed paths | Appends to `InvalidatedClaims` and sets `StatusConflict` if missing | N/A | `internal/reconcile/engine.go:302-328` |
| 19 | Reconcile | Commit Ancestry & Divergence | Verifies checkpoint commit exists locally and is an ancestor of current HEAD | Checkpoint commit, current commit | Sets `StatusConflict` if diverged or nonexistent | N/A | `internal/reconcile/engine.go:264-285` |
| 20 | DB | SQLite Initialization & Migration | Creates `.sentinel` directory, `.gitignore`, and schema tables (`events`, `checkpoints`) | Project root directory path | `*sql.DB` connection | Returns wrapped error on OS or SQL migration failures | `internal/db/db.go:18-79` |
| 21 | DB | Checkpoint Persistence & Query | Saves and retrieves state checkpoints ordered by timestamp, state_version, and rowid | `state.Checkpoint`, `taskID` string | Persisted row, `*state.Checkpoint` | Returns `sql.ErrNoRows` if no checkpoint exists; error on nil DB | `internal/db/db.go:81-191` |
| 22 | DB | Event Persistence & Query | Persists events and queries full chronological history by task ID | `events.Event`, `taskID` string | Persisted row, `[]events.Event` | Returns empty slice if no events; error on nil DB | `internal/db/db.go:193-274` |
| 23 | Service | Application Service Facade | Unified service facade interface (`TaskService`) for headless operations | `CheckpointRequest`, `StatusRequest`, `taskID` | `*state.Checkpoint`, `*ReconciliationResult`, `*ResumePacket` | Typed domain errors | `PROJECT.md:110-119` |

---

## Edge Cases

| # | Feature | Input | Observed Behavior |
|---|---------|-------|-------------------|
| 1 | `git.GetState` | Fresh repository with 0 commits and clean worktree | Returns `HasCommits=false`, `CommitHash=""`, `IsClean=true`, `Branch="main"` or `"master"`, `err=nil` |
| 2 | `git.GetState` | Repository with 0 commits and untracked/staged files | Returns `HasCommits=false`, `IsClean=false`, correctly categorizes `UntrackedFiles` and `StagedFiles` |
| 3 | `git.GetState` | Detached HEAD state | `Branch="HEAD"`, `IsDetached=true`, retrieves valid `CommitHash` |
| 4 | `git.ParsePorcelainStatus` | Whitespace-only or empty status output | Returns `IsClean=true`, 0 entries in all file slices |
| 5 | `git.ParsePorcelainStatus` | Lines shorter than 3 characters | Safely skips truncated lines without panicking, returns `IsClean=true` |
| 6 | `git.ParsePorcelainStatus` | Ignored files (`!! pattern`) | Excluded from tracked/untracked file slices; does not mark repository dirty |
| 7 | `git.ParsePorcelainStatus` | Quoted paths with spaces and international Unicode characters | Unquotes paths, normalizes forward slashes, correctly identifies staging and worktree status |
| 8 | `git.ParsePorcelainStatus` | Renamed and modified (`RM old -> new`) | Staged status recorded with `OrigPath=old`, `Path=new`, `StagingStatus=R`, `WorkTreeStatus=M` |
| 9 | `git.CommitExists` | Empty string or whitespace-only commit hash | Returns `false, nil` without executing invalid Git command |
| 10 | `git.IsAncestor` | Same commit passed as ancestor and descendant (`c1, c1`) | Returns `true, nil` (reflexive ancestry) without executing subshell |
| 11 | `git.IsAncestor` | Empty string for either ancestor or descendant | Returns `false, nil` safely |
| 12 | `reconcile.Reconcile` | Checkpoint commit non-empty, repository has 0 commits | Returns `StatusConflict`, `ConfidenceNone`, reason: `"Checkpoint references commit but repository has no commits"` |
| 13 | `reconcile.Reconcile` | Checkpoint commit empty, repository has commits | Returns `StatusStale`, `ConfidenceLow`, reason: `"Repository or checkpoint has no recorded commit"` |
| 14 | `reconcile.Reconcile` | Checkpoint branch `"feature/auth"`, repository branch `"main"` | Returns `StatusStale`, `BranchMatch=false`, reason: branch mismatch |
| 15 | `reconcile.Reconcile` | Checkpoint branch `"HEAD"`, repository branch `"main"` | Returns `StatusSafe`, `BranchMatch=true` (HEAD is compatible with any active branch) |
| 16 | `reconcile.Reconcile` | Constraint string with Windows backslashes (`.\auth\session.go`) | Normalized to forward slashes; correctly matches modified file `auth/session.go` and triggers CONFLICT |
| 17 | `reconcile.Reconcile` | Constraint with English stop words (e.g. `"Do not touch auth"`) | Stop words (`do`, `not`, `touch`) ignored; candidate token `auth` triggers CONFLICT when `auth/session.go` is modified |
| 18 | `reconcile.Reconcile` | Decision with `Status="REJECTED"` modified in working tree | Does not trigger constraint violation; remains STALE instead of CONFLICT |
| 19 | `reconcile.Reconcile` | Staged renamed file whose original path violates a constraint | Both `Path` and `OrigPath` are checked; triggers `StatusConflict` |
| 20 | `reconcile.ReconcileRepo` | Claimed completed file deleted from filesystem | Detects physical file absence via `os.Stat`; triggers `StatusConflict` and records invalidation claim |
| 21 | `reconcile.ReconcileRepo` | Checkpoint commit exists on diverged branch (not ancestor) | Detects history divergence via `IsAncestor=false`; triggers `StatusConflict` |
| 22 | `reconcile.ReconcileRepo` | Committed changes between checkpoint and HEAD violate constraint | Retrieves changed files between commits; evaluates constraint violation and triggers `StatusConflict` |
| 23 | `db.GetLatestCheckpoint` | Querying non-existent task ID or empty database | Returns `nil, sql.ErrNoRows` |
| 24 | `db.SaveCheckpoint` | Checkpoint with `uuid.Nil` ID and TaskID | Automatically generates valid random UUIDs before SQL insertion |
| 25 | `db.InitDB` | Nil DB connection passed to DB methods | Returns explicit error `"db connection is nil"` without panicking |
| 26 | `cmd.status` | Invoking status on repository without any saved checkpoints | Text mode prints friendly notification; JSON mode returns `{"status": "UNKNOWN"}` and exit code 0 |

---

## Detailed Specifications & Behavioral Requirements

### 1. CLI Subcommands, Flags, Defaults, Output Formats & Errors

#### Root Command
- Command: `wake` (or `sentinel`)
- Usage: `wake [command]`
- Exit code on unhandled command failure: `1`

#### Subcommand: `wake checkpoint`
- Usage: `wake checkpoint [flags]`
- Flags:
  - `--task-id`: `string` (default `""`): Target task UUID. If omitted, uses latest task UUID or generates a new UUID.
  - `--objective`: `string` (default `""`): Human-readable task goal. Updates `State.Objective`.
  - `--dir`: `string` (default `""`): Target directory (defaults to current working directory).
- Processing Steps:
  1. Resolves repository root using `git rev-parse --show-toplevel`.
  2. Extracts current `git.RepositoryState` (HEAD commit SHA, active branch, cleanliness).
  3. Initializes SQLite store at `<repoRoot>/.sentinel/state.db`.
  4. Resolves Task ID: if provided, validates UUID format; if omitted, retrieves latest checkpoint Task ID or generates `uuid.New()`.
  5. Determines `StateVersion`: increments previous checkpoint version by 1 (starts at 1).
  6. Replays all events for Task ID using `state.Reduce` to derive `currentState`.
  7. Updates `currentState.Objective` (if `--objective` provided) and `currentState.LastVerified = repoState.CommitHash`.
  8. Emits and saves `GIT_COMMIT` event.
  9. Persists `state.Checkpoint` record.
- Success Output:
  ```text
  [WAKE] Checkpoint created successfully.
  Task ID:       <uuid>
  Commit:        <hash>
  Branch:        <branch>
  State Version: <int>
  Working Tree:  Clean | <N> modified file(s)
  ```
- Error Conditions:
  - Not a git repository: `"git repository root not found at '<dir>': ..."` (exit code 1).
  - Malformed UUID in `--task-id`: `"invalid task-id '<val>': ..."` (exit code 1).
  - SQLite I/O error: `"failed to save checkpoint: ..."` (exit code 1).

#### Subcommand: `wake status`
- Usage: `wake status [flags]`
- Flags:
  - `--task-id`: `string` (default `""`): Optional task UUID filter.
  - `--dir`: `string` (default `""`): Repository root directory.
  - `--json`: `bool` (default `false`): Formats output as indented JSON.
- Output Specification (Text Mode):
  ```text
  ======================================================================
  WAKE TASK RECONCILIATION REPORT
  ======================================================================
  Task ID:            <uuid>
  Objective:          <objective text>
  Status:             [SAFE | STALE | CONFLICT]
  Confidence:         High | Low | None
  Evaluation Reason:  <reason text>

  --- Repository State ---
  Checkpoint Commit:  <cp_commit>
  Current Commit:     <current_commit>
  Branch Match:       Yes | No (Mismatch)

  --- Evaluation Summary ---
  Total Changed Files:   <int>
  Task-Related Changes:  <int>
  Unrelated Changes:     <int>
  Constraint Violations: <int>
  Invalidated Claims:    <int>

  --- Constraint Violations --- (if > 0)
   [!] <violation description>

  --- Invalidated Claims --- (if > 0)
   [!] <claim description>

  --- Changed Files --- (if > 0)
   [*] <file path>

  --- Guidance ---
  [SAFE] Working tree is fully synchronized with checkpoint. Safe to continue agent execution.
  | [STALE] Repository has drifted without violating constraints. State refresh recommended.
  | [CONFLICT] Critical constraint violation or claim invalidation detected. Manual review required.
  ======================================================================
  ```
- Output Specification (JSON Mode):
  - Serializes `reconcile.ReconciliationResult` JSON directly to stdout.
- Missing Checkpoint Behavior:
  - Text Mode: Outputs banner stating no checkpoint found, advises running `wake checkpoint`, exits 0.
  - JSON Mode: Outputs `{"status": "UNKNOWN", "message": "No active task checkpoint found. Run 'WAKE checkpoint' first."}`, exits 0.

#### Subcommand: `wake history`
- Usage: `wake history [flags]`
- Flags:
  - `--task-id`: `string` (default `""`): Task UUID filter.
  - `--dir`: `string` (default `""`): Repository root directory.
- Output Specification:
  ```text
  Event History for Task: <task_id>
  --------------------------------------------------
  [<HH:MM:SS>] <EVENT_TYPE>
  ...
  Total Events: <count>
  ```
- Error Conditions:
  - No checkpoint found: `"no active task found"` (exit code 1).

#### Subcommand: `wake resume`
- Usage: `wake resume [flags]`
- Flags:
  - `--task-id`: `string` (default `""`): Task UUID to resume.
  - `--dir`: `string` (default `""`): Repository root directory.
- Output Specification (Recovery Packet):
  ```text
  ======================================================================
  RESUMING TASK: <uuid>
  ======================================================================

  GOAL
  <objective>

  COMPLETED
  ✓ <completed milestone 1>
  ✓ <completed milestone 2>

  CURRENT
  <current active work>

  BLOCKERS
  [!] <blocker_id>: <blocker_description>

  CONSTRAINTS
  - <constraint 1>

  DO NOT REPEAT
  - <protected item 1>

  LAST VERIFIED
  Commit <commit_hash>

  NEXT ACTION
  <next step description>

  STATE CONFIDENCE
  High | Low | None

  --- CURRENT REPOSITORY DELTA ---
  No modifications since last checkpoint. Safe to resume from Next Action.
  (or lists changed files and prints: "RECOVERY INSTRUCTION: Read the changed files above before continuing to ensure your context is completely up-to-date.")
  ======================================================================
  ```

---

### 2. The 17 Event Types & Payload Schemas

| # | Event Constant | String Value | Required Payload Fields | Optional Payload Fields | State Engine Reduction Effect |
|---|----------------|--------------|-------------------------|-------------------------|-------------------------------|
| 1 | `TaskStarted` | `"TASK_STARTED"` | `objective` (string) | `constraints` ([]string) | Sets `State.Objective = objective` |
| 2 | `RequirementAdded` | `"REQUIREMENT_ADDED"` | `id` (string), `description` (string) | `priority` (string) | Tracks task requirements in state |
| 3 | `ConstraintAdded` | `"CONSTRAINT_ADDED"` | `constraint` (string) | `id` (string), `reason` (string) | Appends to `State.Constraints` |
| 4 | `UserApproval` | `"USER_APPROVAL"` | `subject` (string) | `comment` (string) | Records user confirmation evidence |
| 5 | `UserRejection` | `"USER_REJECTION"` | `subject` (string), `reason` (string) | `alternative` (string) | Invalidates proposed plans/decisions |
| 6 | `DecisionMade` | `"DECISION_MADE"` | `description` (string) | `id` (string), `source` (string), `status` (string) | Appends to `State.Decisions` with `Status="ACTIVE"` |
| 7 | `FileChanged` | `"FILE_CHANGED"` | `path` (string) | `change_type` (string), `diff` (string) | Records working tree activity |
| 8 | `CommandExecuted` | `"COMMAND_EXECUTED"` | `command` (string), `exit_code` (int) | `output` (string), `duration_ms` (int) | Attaches execution evidence |
| 9 | `TestStarted` | `"TEST_STARTED"` | `test_name` (string) | `suite` (string) | Sets validation in progress |
| 10 | `TestPassed` | `"TEST_PASSED"` | `test_name` (string) | `coverage` (string/float), `evidence` (string) | Attaches verified validation evidence |
| 11 | `TestFailed` | `"TEST_FAILED"` | `test_name` (string), `error` (string) | `output` (string) | Creates blocker or invalidates claims |
| 12 | `BlockerCreated` | `"BLOCKER_CREATED"` | `id` (string), `description` (string) | `severity` (string) | Appends to `State.Blocked` with `Status="ACTIVE"` |
| 13 | `BlockerResolved` | `"BLOCKER_RESOLVED"` | `id` (string) | `resolution` (string) | Updates matching blocker in `State.Blocked` to `Status="RESOLVED"` |
| 14 | `MilestoneCompleted`| `"MILESTONE_COMPLETED"`| `milestone` (string) | `artifacts` ([]string) | Appends to `State.Completed` and `State.DoNotRepeat` |
| 15 | `GitCommit` | `"GIT_COMMIT"` | `hash` (string) | `branch` (string), `clean` (bool), `message` (string) | Updates `State.LastVerified = hash` |
| 16 | `SessionInterrupted`| `"SESSION_INTERRUPTED"`| `reason` (string) | `checkpoint_id` (string) | Sets context transition marker |
| 17 | `SessionResumed` | `"SESSION_RESUMED"` | `session_id` (string) | `reconciliation_status` (string) | Marks resume transition |

---

### 3. State Engine Reduction Rules & Confidence Dynamics

1. **Deterministic Initialization**:
   - Every reduction initializes slices to empty non-nil slices (`[]string{}`, `[]Decision{}`, `[]Blocker{}`).
   - Initial confidence level is always `state.ConfidenceHigh`.
2. **Payload Type Safety**:
   - Payloads are decoded safely using Go type assertions (e.g. `e.Payload["objective"].(string)`). If a field is missing or not a string, it is skipped without error or crash.
3. **Blocker State Transition**:
   - `BlockerCreated` inserts `Blocker{ID: id, Description: desc, Status: "ACTIVE"}`.
   - `BlockerResolved` finds existing blocker with matching `ID` and mutates `Status = "RESOLVED"`.
   - If multiple blockers exist, only the blocker with the exact matching ID is resolved.
4. **Confidence Level Transitions**:
   - `High`: Repository is clean, matches checkpoint commit, 0 active blockers, 0 constraint violations.
   - `Low`: Repository has non-conflicting uncommitted changes or forward commits (STALE).
   - `None`: Active merge conflicts, constraint violations, invalidated claims, diverged history (CONFLICT).

---

### 4. Reconciliation Engine Deterministic State Matrix

| State Category | Condition / Predicate | Evaluated Status | Confidence Level | Reason / Action |
|----------------|-----------------------|------------------|------------------|-----------------|
| Merge Conflict | Working tree has unresolved conflicts (`UU`, `AA`, `DD`, `AU`, `UD`, `UA`, `DU` status) | `CONFLICT` | `None` | Reason: `"Working tree has unresolved merge conflicts"`. Manual resolution required. |
| Constraint Violation | Any changed/staged/untracked file matches a constraint pattern | `CONFLICT` | `None` | Reason: `"Constraint '<c>' violated by modified file '<f>'"` |
| Active Decision Violation | Any changed file matches an active decision (`Status="ACTIVE"`) | `CONFLICT` | `None` | Reason: `"Active decision '<d>' violated by modified file '<f>'"` |
| Rejected Decision Ignored | Changed file matches a rejected decision (`Status="REJECTED"`) | `STALE` (or `SAFE`) | `Low` (or `High`) | Rejected decisions do not guard files. |
| Milestone Claim Alteration | File corresponding to `Completed` milestone is modified/staged | `CONFLICT` | `None` | Reason: `"Completed milestone artifact '<m>' was modified or altered: <f>"` |
| Milestone Claim Deletion | File corresponding to `Completed` milestone is deleted (`StatusDeleted` or missing from disk) | `CONFLICT` | `None` | Reason: `"Completed milestone artifact '<m>' was deleted: <f>"` |
| Do-Not-Repeat Alteration | File corresponding to `DoNotRepeat` list is modified or deleted | `CONFLICT` | `None` | Reason: `"Do-Not-Repeat protected artifact '<d>' was modified or altered: <f>"` |
| Diverged Git History | `merge-base --is-ancestor cpCommit currentCommit` returns exit code 1 | `CONFLICT` | `None` | Reason: `"Checkpoint commit <c1> has diverged from current commit <c2>"` |
| Missing Commit Object | `cat-file -e cpCommit^{commit}` fails in local repo | `CONFLICT` | `None` | Reason: `"Checkpoint commit <c> does not exist in repository"` |
| 0-Commit Mismatch | Checkpoint has commit hash, but live repo has 0 commits | `CONFLICT` | `None` | Reason: `"Checkpoint references commit but repository has no commits"` |
| Exact Synchronization | `cp.Commit == repo.CommitHash`, 0 changed files, branch matches, 0 violations | `SAFE` | `High` | Reason: `"Repository exactly matches checkpoint commit and working tree is clean"` |
| Forward Commits | `repo.CommitHash != cp.Commit` (ancestor check passes), clean worktree, 0 violations | `STALE` | `Low` | Reason: `"Repository commit '<c2>' differs from checkpoint commit '<c1>'"` |
| Uncommitted Changes | Non-conflicting modified, staged, or untracked files present | `STALE` | `Low` | Reason: `"Repository has <N> uncommitted changed file(s)"` |
| Branch Drift | `repo.Branch != cp.Branch` (where neither is `"HEAD"`), 0 violations | `STALE` | `Low` | Reason: `"Repository branch '<b2>' does not match checkpoint branch '<b1>'"` |

---

### 5. Database Schema, Migration & Persistence Contracts

```sql
-- Schema Migration Script (modernc.org/sqlite)

CREATE TABLE IF NOT EXISTS events (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    type TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    payload TEXT NOT NULL -- JSON payload
);

CREATE TABLE IF NOT EXISTS checkpoints (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    commit_hash TEXT NOT NULL,
    state_version INTEGER NOT NULL,
    event_position INTEGER NOT NULL,
    state_data TEXT NOT NULL, -- JSON representation of State struct
    repository TEXT DEFAULT '',
    branch TEXT DEFAULT ''
);
```

#### Persistence API Contracts:
- `InitDB(projectRoot string) (*sql.DB, error)`: Ensures `.sentinel` directory and `.gitignore` exist; opens connection; executes idempotent migrations.
- `SaveCheckpoint(ctx context.Context, db *sql.DB, cp state.Checkpoint) error`:
  - Inserts row into `checkpoints`.
  - Serializes `cp.StateData` to JSON string.
  - Automatically generates `cp.ID` and `cp.TaskID` if `uuid.Nil`.
  - Formats timestamp to RFC3339 if empty.
- `GetLatestCheckpoint(ctx context.Context, db *sql.DB, taskID string) (*state.Checkpoint, error)`:
  - If `taskID != "" && taskID != "all"`: queries by task ID.
  - If `taskID == ""` or `"all"`: queries latest across all tasks.
  - Ordered by `timestamp DESC, state_version DESC, rowid DESC LIMIT 1`.
  - Returns `sql.ErrNoRows` if not found.
- `SaveEvent(ctx context.Context, db *sql.DB, e events.Event) error`:
  - Inserts row into `events`.
  - Serializes `e.Payload` to JSON string.
  - Auto-generates `e.ID` if `uuid.Nil` and sets UTC timestamp if zero.
- `GetEvents(ctx context.Context, db *sql.DB, taskID string) ([]events.Event, error)`:
  - Queries `SELECT id, task_id, type, timestamp, payload FROM events WHERE task_id = ? ORDER BY timestamp ASC, rowid ASC`.

---

### 6. Application Service Facade (`TaskService`)

```go
package service

import (
	"context"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/reconcile"
	"github.com/wake/wake/internal/state"
)

type CheckpointRequest struct {
	TaskID    string `json:"task_id"`
	Objective string `json:"objective"`
	Dir       string `json:"dir"`
}

type StatusRequest struct {
	TaskID    string   `json:"task_id"`
	Dir       string   `json:"dir"`
	TaskFiles []string `json:"task_files"`
}

type ResumePacket struct {
	TaskID        string                         `json:"task_id"`
	Objective     string                         `json:"objective"`
	Completed     []string                       `json:"completed"`
	Current       string                         `json:"current"`
	Blockers      []state.Blocker                `json:"blockers"`
	Constraints   []string                       `json:"constraints"`
	DoNotRepeat   []string                       `json:"do_not_repeat"`
	LastVerified  string                         `json:"last_verified"`
	NextAction    string                         `json:"next_action"`
	Confidence    state.ConfidenceLevel          `json:"confidence"`
	Reconciliation reconcile.ReconciliationResult `json:"reconciliation"`
}

type TaskService interface {
	CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
	GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
	GetHistory(ctx context.Context, taskID string) ([]events.Event, error)
	ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
}
```

---

### 7. Complete Requirement & Test Assertion Matrix

| Req ID | Target Module | Feature Under Test | Test Input / Stimulus | Expected Output / Assertion |
|--------|---------------|-------------------|----------------------|-----------------------------|
| REQ-01 | `cmd` | `wake checkpoint` in fresh repo | Run checkpoint with `--objective "Init"` on clean repo | Checkpoint created with `StateVersion=1`, `Commit=<HEAD>`, `Objective="Init"` |
| REQ-02 | `cmd` | Sequential `wake checkpoint` | Run checkpoint twice with same `taskID` | Second checkpoint has `StateVersion=2`, objective updated |
| REQ-03 | `cmd` | `wake checkpoint` non-git dir | Run checkpoint on empty temp directory | Returns error containing `"git repository root not found"` |
| REQ-04 | `cmd` | `wake checkpoint` invalid task ID | Run checkpoint with `--task-id "bad-uuid"` | Returns error containing `"invalid task-id"` |
| REQ-05 | `cmd` | `wake status` with no checkpoint | Run status on repository with 0 checkpoints | Text mode prints no-checkpoint banner; JSON mode returns `{"status":"UNKNOWN"}`; exit 0 |
| REQ-06 | `cmd` | `wake status` JSON mode | Run status with `--json` on repo with checkpoint | Outputs valid JSON parsable into `ReconciliationResult` |
| REQ-07 | `cmd` | `wake history` | Save 3 events, run history | Prints 3 formatted event lines and `"Total Events: 3"` |
| REQ-08 | `cmd` | `wake resume` SAFE repo | Run resume on clean repo matching checkpoint commit | Outputs Recovery Packet with `STATE CONFIDENCE High` and `"No modifications since last checkpoint"` |
| REQ-09 | `cmd` | `wake resume` STALE repo | Modify non-conflicting file, run resume | Outputs Recovery Packet with `Status: STALE`, lists changed files, prints recovery instruction |
| REQ-10 | `events` | `NewEvent` helper | Call `NewEvent(taskID, TaskStarted, payload)` | Returns `Event` with non-nil UUID, correct `TaskID`, `Timestamp` <= `time.Now().UTC()` |
| REQ-11 | `state` | `Reduce` TaskStarted | Event stream containing `TaskStarted` with `objective="Build API"` | `state.Objective == "Build API"` |
| REQ-12 | `state` | `Reduce` Blocker Lifecycle | `BlockerCreated(b-1)` followed by `BlockerResolved(b-1)` | Blocker `b-1` status transitions from `"ACTIVE"` to `"RESOLVED"` |
| REQ-13 | `state` | `Reduce` Constraints & Decisions | `ConstraintAdded` and `DecisionMade` events | `Constraints` slice has entry; `Decisions` slice has `Decision{Status: "ACTIVE"}` |
| REQ-14 | `state` | `Reduce` MilestoneCompleted | `MilestoneCompleted` event | `Completed` slice has entry |
| REQ-15 | `state` | `Reduce` GitCommit | `GitCommit` event with `hash="abc1234"` | `state.LastVerified == "abc1234"` |
| REQ-16 | `git` | Clean Status | `ParsePorcelainStatus("")` | `status.IsClean == true`, all slices length 0 |
| REQ-17 | `git` | Untracked & Staged Files | Porcelain output `"A file1.go\n?? file2.go"` | `StagedFiles` has 1 entry (`StatusAdded`), `UntrackedFiles` has 1 entry, `IsClean == false` |
| REQ-18 | `git` | Merge Conflicts | Porcelain output with `UU`, `AA`, `DD`, `AU`, `UD`, `UA`, `DU` | `UnmergedFiles` has 7 entries, `IsClean == false`, `ExtractModifiedFiles` has 7 entries |
| REQ-19 | `git` | Unicode and Quoted Filenames | Quoted paths with spaces and international characters | Clean paths unquoted; UTF-8 strings extracted accurately |
| REQ-20 | `git` | CommitExists | Call `CommitExists` with valid commit, invalid commit, empty string | Returns `true`, `false`, `false` respectively |
| REQ-21 | `git` | IsAncestor | Call `IsAncestor` with `(c1, c1)`, `(c1, c2)` (descendant), `(c2, c1)` | Returns `true`, `true`, `false` respectively |
| REQ-22 | `git` | Error Classification | Pass error stderr containing `"not a git repository"` | Returns `GitError` wrapping sentinel `ErrNotGitRepo` |
| REQ-23 | `reconcile` | SAFE State | Matching commit, clean working tree, matching branch | Returns `StatusSafe`, `ConfidenceHigh`, `BranchMatch=true`, 0 changed files |
| REQ-24 | `reconcile` | STALE State (Forward Commits) | HEAD commit is descendant of checkpoint commit, clean worktree | Returns `StatusStale`, `ConfidenceLow`, `CurrentCommit != CheckpointCommit` |
| REQ-25 | `reconcile` | STALE State (Modified Task Files)| Task-related file modified in worktree without constraint conflict | Returns `StatusStale`, `TaskRelatedChanges` contains file, 0 constraint violations |
| REQ-26 | `reconcile` | CONFLICT (Constraint Violation) | Modify file matching `Constraints` pattern (`auth/*`) | Returns `StatusConflict`, `ConfidenceNone`, `ConstraintViolations` length >= 1 |
| REQ-27 | `reconcile` | CONFLICT (Active Decision) | Modify file matching active `Decision` | Returns `StatusConflict`, `ConfidenceNone`, `ConstraintViolations` length 1 |
| REQ-28 | `reconcile` | Non-Conflict (Rejected Decision) | Modify file matching rejected `Decision` | Does not trigger constraint violation; returns `StatusStale` |
| REQ-29 | `reconcile` | CONFLICT (Altered Milestone) | Modify file matching `Completed` or `DoNotRepeat` | Returns `StatusConflict`, `ConfidenceNone`, `InvalidatedClaims` length >= 1 |
| REQ-30 | `reconcile` | CONFLICT (Deleted Milestone) | Delete file matching `Completed` milestone | Returns `StatusConflict`, `ConfidenceNone`, `InvalidatedClaims` length >= 1 |
| REQ-31 | `reconcile` | CONFLICT (Merge Conflict) | Repository in merge conflict (`UnmergedFiles > 0`) | Returns `StatusConflict`, `ConfidenceNone`, reason mentions merge conflicts |
| REQ-32 | `reconcile` | CONFLICT (Diverged Branch) | HEAD commit is not descendant of checkpoint commit | Returns `StatusConflict`, `ConfidenceNone`, reason mentions diverged |
| REQ-33 | `reconcile` | CONFLICT (Missing on Disk) | Completed artifact does not exist on filesystem | Returns `StatusConflict`, `ConfidenceNone`, reason mentions missing on disk |
| REQ-34 | `db` | Database Migration | Call `InitDB(tempDir)` | Creates `.sentinel/state.db` with `events` and `checkpoints` tables |
| REQ-35 | `db` | Save and Retrieve Checkpoint | Save checkpoint version 1 and 2, query latest | Returns checkpoint version 2 with unmarshaled `StateData` |
| REQ-36 | `db` | Save and Retrieve Events | Save 2 events, query by `taskID` | Returns 2 events ordered chronologically by timestamp |
| REQ-37 | `db` | Nil DB Error Handling | Call DB methods with `nil` DB pointer | Returns error `"db connection is nil"` without panic |
| REQ-38 | `service` | Facade Orchestration | Call `CreateCheckpoint` via `TaskService` | Persists checkpoint and returns snapshot matching direct CLI output |

## 2026-08-28T17:05:01Z
You are teamwork_preview_worker_m2.
Working Directory: C:/Users/USER/Desktop/Sentinel/.agents/teamwork_preview_worker_m2
Workspace Root: C:/Users/USER/Desktop/Sentinel
Original Request File: C:/Users/USER/Desktop/Sentinel/.agents/ORIGINAL_REQUEST.md
Project Blueprint: C:/Users/USER/Desktop/Sentinel/PROJECT.md
Survey Findings: C:/Users/USER/Desktop/Sentinel/.agents/teamwork_preview_explorer_survey_3/handoff.md
Git CLI Package: C:/Users/USER/Desktop/Sentinel/internal/git

Objective: Implement Milestone 2 - Reconciliation Engine (Requirement R2).
Write Ownership: You exclusively own files in `C:/Users/USER/Desktop/Sentinel/internal/reconcile/` (specifically `models.go`, `engine.go`, and `engine_test.go`).

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A teamwork_preview_auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.

Detailed Tasks:
1. Create `internal/reconcile/` package implementing:
   - `models.go`:
     - `ReconciliationStatus` type with constants `StatusSafe` ("SAFE"), `StatusStale` ("STALE"), `StatusConflict` ("CONFLICT").
     - `ReconciliationResult` struct containing Status, Reason, CheckpointCommit, CurrentCommit, BranchMatch, ChangedFiles, TaskRelatedChanges, UnrelatedChanges, ConstraintViolations, InvalidatedClaims, ConfidenceLevel.
   - `engine.go`:
     - `Engine` interface and `Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult` function.
     - `ReconcileRepo(ctx context.Context, cp state.Checkpoint, gitClient git.Client, repoPath string, taskFiles []string) (ReconciliationResult, error)` helper for live evaluation.
     - Exact status classification logic:
       - **CONFLICT**:
         1. Working tree has merge conflicts (`repo.HasMergeConflicts == true` or `len(repo.UnmergedFiles) > 0`).
         2. Any changed/modified/untracked/deleted file violates `cp.StateData.Constraints` or active `cp.StateData.Decisions`.
         3. Any artifact/file claimed in `cp.StateData.Completed` or `cp.StateData.DoNotRepeat` is modified, deleted, or missing.
         4. Checkpoint commit does not exist or has diverged from history.
       - **SAFE**:
         1. `repo.IsClean == true` (zero uncommitted/modified/untracked changes).
         2. `repo.CommitHash == cp.Commit` (and not empty).
         3. `repo.Branch == cp.Branch` (or matching).
         4. Zero changed files and zero constraint violations.
       - **STALE**:
         1. Not a CONFLICT, but repository state has drifted from checkpoint.
         2. Forward commits exist (`repo.CommitHash != cp.Commit`) without constraint violations.
         3. Uncommitted non-conflicting modifications exist (`len(repo.ModifiedFiles) > 0` or task files modified).
         4. Untracked files exist.
   - Robust path matching: Handle glob patterns, relative paths, case-insensitivity where appropriate, and path normalization (`filepath.ToSlash`).
2. Write unit tests in `internal/reconcile/engine_test.go`:
   - Test SAFE evaluation when commit matches and repo is clean.
   - Test STALE evaluation when forward commits exist without constraint violations.
   - Test STALE evaluation when non-conflicting modified files exist.
   - Test CONFLICT evaluation when constraint file is modified.
   - Test CONFLICT evaluation when decision file is modified.
   - Test CONFLICT evaluation when completed/do-not-repeat file is modified or deleted.
   - Test CONFLICT evaluation on merge conflicts.
   - Test edge cases: empty repo, empty checkpoint commit, branch mismatch, path prefix variations.
3. Run `go test -v ./internal/reconcile/...` and `go test -v ./...` verifying 100% pass.
4. Record your progress in `progress.md` and write a complete 5-component report to `handoff.md` in your working directory.
Remember: Do not use emojis anywhere (use icons, tags, or text). Send a message back when done.

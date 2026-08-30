package reconcile

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/state"
)

// Engine evaluates a Checkpoint against live Git repository state.
type Engine interface {
	Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult
}

type engine struct{}

// NewEngine constructs a default Engine instance.
func NewEngine() Engine {
	return &engine{}
}

// Reconcile implements the Engine interface.
func (e *engine) Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult {
	return Reconcile(cp, repo, taskFiles)
}

// Reconcile evaluates a state Checkpoint against a RepositoryState snapshot.
func Reconcile(cp state.Checkpoint, repo git.RepositoryState, taskFiles []string) ReconciliationResult {
	result := ReconciliationResult{
		Status:               StatusSafe,
		Reason:               "",
		CheckpointCommit:     cp.Commit,
		CurrentCommit:        repo.CommitHash,
		BranchMatch:          true,
		ChangedFiles:         make([]string, 0),
		TaskRelatedChanges:   make([]string, 0),
		UnrelatedChanges:     make([]string, 0),
		ConstraintViolations: make([]string, 0),
		InvalidatedClaims:    make([]string, 0),
		ConfidenceLevel:      state.ConfidenceHigh,
	}

	// Evaluate branch compatibility
	cpBranch := strings.TrimSpace(cp.Branch)
	repoBranch := strings.TrimSpace(repo.Branch)
	if cpBranch != "" && repoBranch != "" && !strings.EqualFold(cpBranch, "HEAD") && !strings.EqualFold(repoBranch, "HEAD") {
		if !strings.EqualFold(cpBranch, repoBranch) {
			result.BranchMatch = false
		}
	}

	// Consolidate all changed files from repository state
	allChangesMap := make(map[string]struct{})
	for _, p := range repo.ModifiedFiles {
		cleaned := normalizePath(p)
		if cleaned != "" && !isInternalMetadataPath(cleaned) {
			allChangesMap[cleaned] = struct{}{}
		}
	}
	for _, p := range repo.UntrackedFiles {
		cleaned := normalizePath(p)
		if cleaned != "" && !isInternalMetadataPath(cleaned) {
			allChangesMap[cleaned] = struct{}{}
		}
	}
	for _, p := range repo.UnmergedFiles {
		cleaned := normalizePath(p)
		if cleaned != "" && !isInternalMetadataPath(cleaned) {
			allChangesMap[cleaned] = struct{}{}
		}
	}
	for _, f := range repo.StagedFiles {
		if c := normalizePath(f.Path); c != "" && !isInternalMetadataPath(c) {
			allChangesMap[c] = struct{}{}
		}
		if c := normalizePath(f.OrigPath); c != "" && !isInternalMetadataPath(c) {
			allChangesMap[c] = struct{}{}
		}
	}
	for _, f := range repo.UnstagedFiles {
		if c := normalizePath(f.Path); c != "" && !isInternalMetadataPath(c) {
			allChangesMap[c] = struct{}{}
		}
		if c := normalizePath(f.OrigPath); c != "" && !isInternalMetadataPath(c) {
			allChangesMap[c] = struct{}{}
		}
	}

	for p := range allChangesMap {
		result.ChangedFiles = append(result.ChangedFiles, p)
	}
	sort.Strings(result.ChangedFiles)

	// Categorize into TaskRelatedChanges vs UnrelatedChanges
	normalizedTaskFiles := make([]string, 0, len(taskFiles))
	for _, tf := range taskFiles {
		if c := normalizePath(tf); c != "" {
			normalizedTaskFiles = append(normalizedTaskFiles, c)
		}
	}

	for _, file := range result.ChangedFiles {
		if len(normalizedTaskFiles) > 0 {
			if matchesAnyTaskFile(file, normalizedTaskFiles) {
				result.TaskRelatedChanges = append(result.TaskRelatedChanges, file)
			} else {
				result.UnrelatedChanges = append(result.UnrelatedChanges, file)
			}
		} else {
			result.TaskRelatedChanges = append(result.TaskRelatedChanges, file)
		}
	}

	// 1. CONFLICT CHECK: Unresolved merge conflicts
	if repo.HasMergeConflicts || len(repo.UnmergedFiles) > 0 {
		result.Status = StatusConflict
		result.ConfidenceLevel = state.ConfidenceNone
		result.Reason = "Working tree has unresolved merge conflicts"
	}

	// 2. CONFLICT CHECK: Constraint violations
	for _, file := range result.ChangedFiles {
		for _, constraint := range cp.StateData.Constraints {
			if matchesConstraint(file, constraint) {
				violation := fmt.Sprintf("Constraint '%s' violated by modified file '%s'", constraint, file)
				result.ConstraintViolations = append(result.ConstraintViolations, violation)
			}
		}
	}

	// 3. CONFLICT CHECK: Decision violations (Active decisions)
	for _, file := range result.ChangedFiles {
		for _, decision := range cp.StateData.Decisions {
			if isActiveDecision(decision) {
				if matchesDecision(file, decision) {
					violation := fmt.Sprintf("Active decision '%s' violated by modified file '%s'", decision.Description, file)
					result.ConstraintViolations = append(result.ConstraintViolations, violation)
				}
			}
		}
	}

	// Build a map of renamed files to avoid false-positive conflicts
	renameMap := make(map[string]string)
	for _, f := range repo.StagedFiles {
		if f.StagingStatus == git.StatusRenamed && f.OrigPath != "" {
			renameMap[normalizePath(f.OrigPath)] = normalizePath(f.Path)
			renameMap[normalizePath(f.Path)] = normalizePath(f.OrigPath)
		}
	}
	for _, f := range repo.UnstagedFiles {
		if f.WorkTreeStatus == git.StatusRenamed && f.OrigPath != "" {
			renameMap[normalizePath(f.OrigPath)] = normalizePath(f.Path)
			renameMap[normalizePath(f.Path)] = normalizePath(f.OrigPath)
		}
	}

	// 4. CONFLICT CHECK: Invalidation of Completed / DoNotRepeat claims
	for _, file := range result.ChangedFiles {
		if _, isRename := renameMap[file]; isRename {
			// Skip conflict generation for renamed files.
			// We treat them as safely moved rather than tampered/deleted.
			continue
		}

		for _, completed := range cp.StateData.Completed {
			if matchesCompletedOrDoNotRepeat(file, completed) {
				invalidation := fmt.Sprintf("Completed milestone artifact '%s' was modified or altered: %s", completed, file)
				result.InvalidatedClaims = append(result.InvalidatedClaims, invalidation)
			}
		}
		for _, dnr := range cp.StateData.DoNotRepeat {
			if matchesCompletedOrDoNotRepeat(file, dnr) {
				invalidation := fmt.Sprintf("Do-Not-Repeat protected artifact '%s' was modified or altered: %s", dnr, file)
				result.InvalidatedClaims = append(result.InvalidatedClaims, invalidation)
			}
		}
	}

	// Invalidation from deleted status in worktree or index
	deletedFiles := getDeletedFiles(repo)
	for _, delFile := range deletedFiles {
		if _, isRename := renameMap[delFile]; isRename {
			// Skip conflict generation for renamed files.
			// We treat them as safely moved rather than tampered/deleted.
			continue
		}

		for _, completed := range cp.StateData.Completed {
			if matchesCompletedOrDoNotRepeat(delFile, completed) {
				invalidation := fmt.Sprintf("Completed milestone artifact '%s' was deleted: %s", completed, delFile)
				if !containsString(result.InvalidatedClaims, invalidation) {
					result.InvalidatedClaims = append(result.InvalidatedClaims, invalidation)
				}
			}
		}
		for _, dnr := range cp.StateData.DoNotRepeat {
			if matchesCompletedOrDoNotRepeat(delFile, dnr) {
				invalidation := fmt.Sprintf("Do-Not-Repeat protected artifact '%s' was deleted: %s", dnr, delFile)
				if !containsString(result.InvalidatedClaims, invalidation) {
					result.InvalidatedClaims = append(result.InvalidatedClaims, invalidation)
				}
			}
		}
	}

	// 5. CONFLICT CHECK: Repository has no commits but Checkpoint references a commit
	if cp.Commit != "" && !repo.HasCommits {
		result.Status = StatusConflict
		result.ConfidenceLevel = state.ConfidenceNone
		if result.Reason == "" {
			result.Reason = "Checkpoint references commit but repository has no commits"
		}
	}

	// If any constraint violations or invalidated claims were detected, mark as CONFLICT
	if len(result.ConstraintViolations) > 0 {
		result.Status = StatusConflict
		result.ConfidenceLevel = state.ConfidenceNone
		if result.Reason == "" {
			result.Reason = fmt.Sprintf("Constraint violation detected: %s", result.ConstraintViolations[0])
		}
	}
	if len(result.InvalidatedClaims) > 0 {
		result.Status = StatusConflict
		result.ConfidenceLevel = state.ConfidenceNone
		if result.Reason == "" {
			result.Reason = fmt.Sprintf("Claim invalidation detected: %s", result.InvalidatedClaims[0])
		}
	}

	if result.Status == StatusConflict {
		return result
	}

	// 6. SAFE EVALUATION:
	// - repo is clean (no uncommitted/modified/untracked changes)
	// - repo commit matches checkpoint commit (and neither is empty)
	// - branch matches
	// - zero changed files and zero constraint violations
	isRepoClean := len(result.ChangedFiles) == 0
	hasMatchingNonEmptyCommit := cp.Commit != "" && repo.CommitHash != "" && cp.Commit == repo.CommitHash
	isNonGitMatch := cp.Commit == "" && repo.CommitHash == ""
	isBranchValid := result.BranchMatch

	if isRepoClean && (hasMatchingNonEmptyCommit || isNonGitMatch) && isBranchValid && len(result.ConstraintViolations) == 0 && len(result.InvalidatedClaims) == 0 {
		result.Status = StatusSafe
		result.ConfidenceLevel = state.ConfidenceHigh
		if isNonGitMatch {
			result.Reason = "Workspace exactly matches checkpoint state and is clean"
		} else {
			result.Reason = "Repository exactly matches checkpoint commit and working tree is clean"
		}
		return result
	}

	// 7. STALE EVALUATION:
	// If not CONFLICT and not SAFE, state has drifted
	result.Status = StatusStale
	result.ConfidenceLevel = state.ConfidenceLow

	if !result.BranchMatch {
		result.Reason = fmt.Sprintf("Repository branch '%s' does not match checkpoint branch '%s'", repoBranch, cpBranch)
	} else if cp.Commit == "" || repo.CommitHash == "" {
		if len(result.ChangedFiles) > 0 {
			result.Reason = fmt.Sprintf("Workspace has %d locally modified file(s)", len(result.ChangedFiles))
		} else {
			result.Reason = "Workspace state has drifted from checkpoint"
		}
	} else if cp.Commit != repo.CommitHash {
		result.Reason = fmt.Sprintf("Repository commit '%s' differs from checkpoint commit '%s'", repo.CommitHash, cp.Commit)
	} else if len(result.ChangedFiles) > 0 {
		result.Reason = fmt.Sprintf("Repository has %d uncommitted changed file(s)", len(result.ChangedFiles))
	} else {
		result.Reason = "Repository state has drifted from checkpoint"
	}

	return result
}

// ReconcileRepo inspects a live Git repository on disk and evaluates it against the Checkpoint.
func ReconcileRepo(ctx context.Context, cp state.Checkpoint, gitClient git.Client, repoPath string, taskFiles []string) (ReconciliationResult, error) {
	if gitClient == nil {
		gitClient = git.NewClient(nil)
	}

	repoState, err := gitClient.GetState(ctx, repoPath)
	if err != nil {
		return ReconciliationResult{}, err
	}

	// If checkpoint has a commit and repo has commits, check commit history relationship
	var commitChangedFiles []string
	if cp.Commit != "" && repoState.CommitHash != "" && cp.Commit != repoState.CommitHash {
		// Check if cp.Commit exists in local repo
		exists, err := gitClient.CommitExists(ctx, repoPath, cp.Commit)
		if err != nil {
			return ReconciliationResult{}, fmt.Errorf("failed to verify checkpoint commit existence: %w", err)
		}
		if !exists {
			res := Reconcile(cp, *repoState, taskFiles)
			res.Status = StatusConflict
			res.ConfidenceLevel = state.ConfidenceNone
			res.Reason = fmt.Sprintf("Checkpoint commit %s does not exist in repository", cp.Commit)
			return res, nil
		}

		// Check if cp.Commit is an ancestor of the current commit
		isAncestor, err := gitClient.IsAncestor(ctx, repoPath, cp.Commit, repoState.CommitHash)
		if err != nil {
			return ReconciliationResult{}, fmt.Errorf("failed to verify git commit ancestry: %w", err)
		}
		if !isAncestor {
			res := Reconcile(cp, *repoState, taskFiles)
			res.Status = StatusConflict
			res.ConfidenceLevel = state.ConfidenceNone
			res.Reason = fmt.Sprintf("Checkpoint commit %s has diverged from current commit %s", cp.Commit, repoState.CommitHash)
			return res, nil
		}

		// Commit is an ancestor: retrieve changed files between checkpoint commit and current commit
		changed, err := gitClient.GetChangedFilesBetween(ctx, repoPath, cp.Commit, repoState.CommitHash)
		if err != nil {
			return ReconciliationResult{}, fmt.Errorf("failed to retrieve changed files between commits: %w", err)
		}
		if len(changed) > 0 {
			commitChangedFiles = changed
		}
	}

	// Local file drift for non-git workspaces
	if repoState.CommitHash == "" && cp.StateData.Files != nil {
		if currentFiles, err := ScanDirectory(repoState.RootPath); err == nil {
			var localChanged []string
			for file, mod := range currentFiles {
				if oldMod, exists := cp.StateData.Files[file]; !exists || oldMod != mod {
					localChanged = append(localChanged, file)
				}
			}
			for oldFile := range cp.StateData.Files {
				if _, exists := currentFiles[oldFile]; !exists {
					localChanged = append(localChanged, oldFile)
					repoState.UnstagedFiles = append(repoState.UnstagedFiles, git.FileStatus{
						Path:           oldFile,
						WorkTreeStatus: git.StatusDeleted,
					})
				}
			}
			repoState.ModifiedFiles = append(repoState.ModifiedFiles, localChanged...)
			if len(localChanged) > 0 {
				repoState.IsClean = false
			}
		}
	}

	// Merge commit-level changes with working-tree modified files
	allModified := append([]string{}, repoState.ModifiedFiles...)
	allModified = append(allModified, commitChangedFiles...)

	augmentedRepoState := *repoState
	augmentedRepoState.ModifiedFiles = deduplicateStrings(allModified)

	result := Reconcile(cp, augmentedRepoState, taskFiles)

	// Check if any claimed Completed or DoNotRepeat file is physically missing from disk
	root := repoState.RootPath
	if root == "" {
		root = repoPath
	}
	if root == "" {
		root = "."
	}

	for _, claimed := range append(cp.StateData.Completed, cp.StateData.DoNotRepeat...) {
		extractedPaths := extractCandidatePaths(claimed)
		for _, p := range extractedPaths {
			cleanP := strings.TrimRight(p, ".,;:!?)")
			if !looksLikeFilePath(cleanP) {
				continue
			}

			fullPath, ok := resolveSafeRepoPath(root, cleanP)
			if !ok {
				continue
			}

			if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
				msg := fmt.Sprintf("Claimed file '%s' does not exist on disk", cleanP)
				if !containsString(result.InvalidatedClaims, msg) {
					result.InvalidatedClaims = append(result.InvalidatedClaims, msg)
				}
				result.Status = StatusConflict
				result.ConfidenceLevel = state.ConfidenceNone
				if result.Reason == "" || result.Reason == "Repository exactly matches checkpoint commit and working tree is clean" {
					result.Reason = msg
				}
			}
		}
	}

	// 4.5 Filter out false-positive InvalidatedClaims using Semantic Diff Analyzer
	if repoState.HasCommits && len(result.InvalidatedClaims) > 0 {
		var finalInvalidations []string
		for _, invalidation := range result.InvalidatedClaims {
			// Extract file path from invalidation message
			parts := strings.Split(invalidation, ": ")
			if len(parts) == 2 {
				file := strings.TrimSpace(parts[1])

				isSemantic := true

				if strings.HasSuffix(file, ".go") {
					// 100% accurate AST parser for Go files (0 tokens, fully local)
					oldCode, _ := gitClient.GetFileAtCommit(ctx, repoPath, file, cp.Commit)

					// Get the current file contents from disk
					fullPath, ok := resolveSafeRepoPath(root, file)
					if ok {
						// #nosec G304
						if newCodeBytes, err := os.ReadFile(fullPath); err == nil {
							newCode := string(newCodeBytes)
							isSemantic = IsSemanticChangeGoAST(oldCode, newCode)
						}
					}
				} else {
					// 90% heuristic diff parser for all other languages
					diff, err := gitClient.GetFileDiff(ctx, repoPath, file)
					if err == nil {
						isSemantic = IsSemanticChange(diff)
					}
				}

				if isSemantic {
					finalInvalidations = append(finalInvalidations, invalidation)
				}
			} else {
				finalInvalidations = append(finalInvalidations, invalidation)
			}
		}

		result.InvalidatedClaims = finalInvalidations

		// If we dropped all invalidations, downgrade the CONFLICT to STALE
		if len(result.InvalidatedClaims) == 0 && len(result.ConstraintViolations) == 0 && result.Status == StatusConflict {
			result.Status = StatusStale
			result.ConfidenceLevel = state.ConfidenceLow
			result.Reason = "Workspace has locally modified file(s) (semantic checks passed)"
		}
	}

	return result, nil
}

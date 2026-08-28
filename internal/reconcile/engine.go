package reconcile

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
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

	// 4. CONFLICT CHECK: Invalidation of Completed / DoNotRepeat claims
	for _, file := range result.ChangedFiles {
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
	isBranchValid := result.BranchMatch

	if isRepoClean && hasMatchingNonEmptyCommit && isBranchValid && len(result.ConstraintViolations) == 0 && len(result.InvalidatedClaims) == 0 {
		result.Status = StatusSafe
		result.ConfidenceLevel = state.ConfidenceHigh
		result.Reason = "Repository exactly matches checkpoint commit and working tree is clean"
		return result
	}

	// 7. STALE EVALUATION:
	// If not CONFLICT and not SAFE, state has drifted
	result.Status = StatusStale
	result.ConfidenceLevel = state.ConfidenceLow

	if !result.BranchMatch {
		result.Reason = fmt.Sprintf("Repository branch '%s' does not match checkpoint branch '%s'", repoBranch, cpBranch)
	} else if cp.Commit == "" || repo.CommitHash == "" {
		result.Reason = "Repository or checkpoint has no recorded commit"
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
		if err == nil && !exists {
			res := Reconcile(cp, *repoState, taskFiles)
			res.Status = StatusConflict
			res.ConfidenceLevel = state.ConfidenceNone
			res.Reason = fmt.Sprintf("Checkpoint commit %s does not exist in repository", cp.Commit)
			return res, nil
		}

		// Check if cp.Commit is an ancestor of the current commit
		isAncestor, err := gitClient.IsAncestor(ctx, repoPath, cp.Commit, repoState.CommitHash)
		if err == nil && !isAncestor {
			res := Reconcile(cp, *repoState, taskFiles)
			res.Status = StatusConflict
			res.ConfidenceLevel = state.ConfidenceNone
			res.Reason = fmt.Sprintf("Checkpoint commit %s has diverged from current commit %s", cp.Commit, repoState.CommitHash)
			return res, nil
		}

		// Commit is an ancestor: retrieve changed files between checkpoint commit and current commit
		changed, err := gitClient.GetChangedFilesBetween(ctx, repoPath, cp.Commit, repoState.CommitHash)
		if err == nil && len(changed) > 0 {
			commitChangedFiles = changed
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

	for _, claimed := range append(cp.StateData.Completed, cp.StateData.DoNotRepeat...) {
		extractedPaths := extractCandidatePaths(claimed)
		for _, p := range extractedPaths {
			if looksLikeFilePath(p) {
				fullPath := filepath.Join(root, filepath.FromSlash(p))
				if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
					msg := fmt.Sprintf("Claimed file '%s' does not exist on disk", p)
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
	}

	return result, nil
}

// normalizePath converts backslashes to forward slashes, cleans the path, and strips leading ./ or /
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"'`")
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, "\\", "/")
	cleaned := path.Clean(p)
	cleaned = strings.TrimPrefix(cleaned, "./")
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "." {
		return ""
	}
	return cleaned
}

// isInternalMetadataPath identifies Sentinel and Git internal working files.
func isInternalMetadataPath(p string) bool {
	p = normalizePath(p)
	if strings.HasPrefix(p, ".sentinel/") || p == ".sentinel" || strings.HasPrefix(p, ".git/") || p == ".git" {
		return true
	}
	return false
}

// matchSinglePattern checks if a normalized file path matches a pattern (exact, directory prefix, or glob).
func matchSinglePattern(pattern, filePath string) bool {
	pattern = normalizePath(pattern)
	filePath = normalizePath(filePath)
	if pattern == "" || filePath == "" {
		return false
	}

	// 1. Exact match (case-insensitive)
	if strings.EqualFold(pattern, filePath) {
		return true
	}

	// 2. Directory prefix match (e.g. pattern "auth" or "auth/" matches "auth/session.go")
	cleanPatternDir := strings.TrimSuffix(pattern, "/")
	cleanPatternDir = strings.TrimSuffix(cleanPatternDir, "/*")
	cleanPatternDir = strings.TrimSuffix(cleanPatternDir, "/**")
	if cleanPatternDir != "" {
		if strings.HasPrefix(strings.ToLower(filePath), strings.ToLower(cleanPatternDir)+"/") {
			return true
		}
		if strings.EqualFold(cleanPatternDir, filePath) {
			return true
		}
	}

	// 3. Glob matching
	if strings.ContainsAny(pattern, "*?[") {
		// Try matching full path
		if matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(filePath)); err == nil && matched {
			return true
		}
		// Try matching base name (e.g. "*.sql" matches "schema/migration.sql")
		if matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(path.Base(filePath))); err == nil && matched {
			return true
		}
	}

	// 4. Path component match (e.g. directory segment "auth" matches "auth/session.go")
	segments := strings.Split(filePath, "/")
	for _, seg := range segments {
		if strings.EqualFold(seg, pattern) {
			return true
		}
	}

	return false
}

// matchesAnyTaskFile checks if filePath matches any of the taskFiles.
func matchesAnyTaskFile(filePath string, taskFiles []string) bool {
	for _, tf := range taskFiles {
		if matchSinglePattern(tf, filePath) {
			return true
		}
	}
	return false
}

// stopWords represents common English words that should not be treated as file or directory names.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "in": true, "on": true,
	"of": true, "for": true, "with": true, "at": true, "by": true, "from": true, "to": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "being": true,
	"do": true, "does": true, "did": true, "not": true, "no": true, "never": true, "none": true,
	"touch": true, "edit": true, "change": true, "changes": true, "modify": true, "modified": true,
	"delete": true, "deleted": true, "remove": true, "removed": true, "update": true, "updated": true,
	"keep": true, "leave": true, "protect": true, "protected": true, "must": true, "should": true,
	"cannot": true, "cant": true, "wont": true, "will": true, "any": true, "all": true,
	"file": true, "files": true, "dir": true, "directory": true, "package": true, "pkg": true,
	"module": true, "code": true, "source": true, "folder": true, "path": true, "legacy": true,
	"use": true, "using": true, "always": true, "active": true, "unmodified": true, "unchanged": true,
}

// matchesConstraint determines if a changed file violates a given constraint string.
func matchesConstraint(filePath, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if constraint == "" {
		return false
	}

	// Check if constraint is directly a path or pattern without whitespace
	if !strings.ContainsAny(constraint, " \t\r\n") {
		return matchSinglePattern(constraint, filePath)
	}

	// Extract candidate tokens/paths from text
	tokens := extractTokens(constraint)
	for _, token := range tokens {
		if stopWords[strings.ToLower(token)] {
			continue
		}
		if len(token) >= 2 {
			if matchSinglePattern(token, filePath) {
				return true
			}
		}
	}

	return false
}

// isActiveDecision checks if a decision is in ACTIVE status.
func isActiveDecision(d state.Decision) bool {
	status := strings.ToUpper(strings.TrimSpace(d.Status))
	if status == "" || status == "ACTIVE" {
		return true
	}
	return false
}

// matchesDecision checks if a changed file violates an active decision.
func matchesDecision(filePath string, d state.Decision) bool {
	desc := strings.TrimSpace(d.Description)
	if desc == "" {
		return false
	}
	return matchesConstraint(filePath, desc)
}

// matchesCompletedOrDoNotRepeat checks if a changed file corresponds to a completed or do-not-repeat item.
func matchesCompletedOrDoNotRepeat(filePath, claim string) bool {
	claim = strings.TrimSpace(claim)
	if claim == "" {
		return false
	}

	// Direct match if claim is a single path or pattern
	if !strings.ContainsAny(claim, " \t\r\n") {
		return matchSinglePattern(claim, filePath)
	}

	// Extract tokens looking for explicit path patterns
	candidates := extractCandidatePaths(claim)
	for _, candidate := range candidates {
		if matchSinglePattern(candidate, filePath) {
			return true
		}
	}

	return false
}

// extractTokens splits a string by whitespace and punctuation delimiters.
func extractTokens(s string) []string {
	delims := regexp.MustCompile(`[\s,;:()[\]"'\x60]+`)
	rawTokens := delims.Split(s, -1)
	tokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		t = strings.TrimSpace(t)
		if t != "" {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

// extractCandidatePaths finds tokens that resemble file or directory paths.
func extractCandidatePaths(s string) []string {
	tokens := extractTokens(s)
	candidates := make([]string, 0)
	for _, t := range tokens {
		if stopWords[strings.ToLower(t)] {
			continue
		}
		if looksLikeFilePath(t) || len(t) >= 3 {
			candidates = append(candidates, t)
		}
	}
	return candidates
}

// looksLikeFilePath checks if a string contains path separators, extensions, or globs.
func looksLikeFilePath(s string) bool {
	if strings.ContainsAny(s, "/\\*.") {
		return true
	}
	return false
}

// getDeletedFiles extracts files with deleted status ('D') from staged or unstaged status.
func getDeletedFiles(repo git.RepositoryState) []string {
	deleted := make([]string, 0)
	for _, f := range repo.StagedFiles {
		if f.StagingStatus == git.StatusDeleted || f.WorkTreeStatus == git.StatusDeleted {
			if c := normalizePath(f.Path); c != "" {
				deleted = append(deleted, c)
			}
		}
	}
	for _, f := range repo.UnstagedFiles {
		if f.StagingStatus == git.StatusDeleted || f.WorkTreeStatus == git.StatusDeleted {
			if c := normalizePath(f.Path); c != "" {
				deleted = append(deleted, c)
			}
		}
	}
	return deduplicateStrings(deleted)
}

// containsString checks if slice contains target.
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// deduplicateStrings removes duplicates and sorts a slice of strings.
func deduplicateStrings(slice []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		cleaned := normalizePath(s)
		if cleaned != "" {
			if _, exists := seen[cleaned]; !exists {
				seen[cleaned] = struct{}{}
				result = append(result, cleaned)
			}
		}
	}
	sort.Strings(result)
	return result
}

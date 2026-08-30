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

var (
	versionRegex     = regexp.MustCompile(`^(?i)v?\d+(\.\d+)+([-_a-z0-9\.]*)?$`)
	stepCounterRegex = regexp.MustCompile(`^(#?\d+(\.\d+)*\.?|\d+\.)$`)
	urlPrefixRegex   = regexp.MustCompile(`^(?i)(https?|ftp|file|ws|wss|git)://`)
	validExtRegex    = regexp.MustCompile(`^\.[a-zA-Z][a-zA-Z0-9_-]{0,9}$`)
)

var knownAbbreviations = map[string]bool{
	"e.g.": true, "i.e.": true, "etc.": true, "ex.": true, "vs.": true, "al.": true,
	"ref.": true, "fig.": true, "no.": true, "dr.": true, "mr.": true, "ms.": true,
	"dept.": true, "approx.": true, "est.": true, "min.": true, "max.": true,
}

var knownStandaloneFiles = map[string]bool{
	"makefile": true, "dockerfile": true, "readme": true, "license": true,
	"procfile": true, "jenkinsfile": true, "gemfile": true, "rakefile": true,
}

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

// isInternalMetadataPath identifies Wake, Sentinel, and Git internal working files.
func isInternalMetadataPath(p string) bool {
	p = normalizePath(p)
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, ".wake/") || lower == ".wake" ||
		strings.HasPrefix(lower, ".sentinel/") || lower == ".sentinel" ||
		strings.HasPrefix(lower, ".git/") || lower == ".git" {
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
	candidates := extractCandidatePaths(constraint)
	for _, candidate := range candidates {
		if matchSinglePattern(candidate, filePath) {
			return true
		}
	}

	// If no explicit candidate path matched, check if any non-stopword token matches the root directory segment
	tokens := extractTokens(constraint)
	firstSeg := strings.Split(normalizePath(filePath), "/")[0]
	for _, token := range tokens {
		if stopWords[strings.ToLower(token)] {
			continue
		}
		if strings.EqualFold(token, firstSeg) {
			return true
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
		if looksLikeFilePath(t) || (strings.Contains(t, "/") && !strings.Contains(t, "://") && isSafeRelativePath(strings.ReplaceAll(t, "*", "x"))) {
			candidates = append(candidates, t)
		}
	}
	return candidates
}

// isSafeRelativePath validates that a path string is strictly relative and contained,
// rejecting path traversals (..), absolute paths, Windows drive letters, and UNC paths.
func isSafeRelativePath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || p == "." {
		return false
	}

	// Reject UNC network paths (e.g. \\server\share or //server/share)
	if strings.HasPrefix(p, `\\`) || strings.HasPrefix(p, "//") {
		return false
	}

	// Reject Windows drive letter prefixes (e.g. C:\... or C:/...)
	if filepath.VolumeName(p) != "" {
		return false
	}
	if len(p) >= 2 && ((p[0] >= 'a' && p[0] <= 'z') || (p[0] >= 'A' && p[0] <= 'Z')) && p[1] == ':' {
		return false
	}

	// Reject absolute paths
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return false
	}

	// Reject directory traversal segments (..)
	normalized := strings.ReplaceAll(p, "\\", "/")
	segments := strings.Split(normalized, "/")
	for _, seg := range segments {
		if seg == ".." {
			return false
		}
	}

	return true
}

// resolveSafeRepoPath joins root and relative path p, verifying containment within root via filepath.Rel.
func resolveSafeRepoPath(root, p string) (string, bool) {
	if !isSafeRelativePath(p) {
		return "", false
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	absRoot = filepath.Clean(absRoot)

	fullPath := filepath.Clean(filepath.Join(absRoot, filepath.FromSlash(p)))

	rel, err := filepath.Rel(absRoot, fullPath)
	if err != nil {
		return "", false
	}

	// Verify rel does not escape root (no ".." prefix and not absolute)
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", false
	}

	return fullPath, true
}

// looksLikeFilePath determines if a token represents a plausible concrete file or directory path,
// safely excluding wildcards (*, ?), URLs, version numbers, step counters, and abbreviations.
func looksLikeFilePath(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`()[]{}<>")
	s = strings.TrimRight(s, ".,;:!?")
	if s == "" {
		return false
	}

	// 1. Exclude wildcards and glob characters
	if strings.ContainsAny(s, "*?[]{}") {
		return false
	}

	// 2. Exclude URLs
	if strings.Contains(s, "://") || urlPrefixRegex.MatchString(s) {
		return false
	}

	// 3. Exclude known abbreviations
	lower := strings.ToLower(s)
	if knownAbbreviations[lower] || knownAbbreviations[lower+"."] {
		return false
	}

	// 4. Exclude version numbers (e.g. v1.0.0, 2.1, v2.0)
	if versionRegex.MatchString(s) {
		return false
	}

	// 5. Exclude step counters and numeric labels (e.g. 1., 2.1, #1)
	if stepCounterRegex.MatchString(s) {
		return false
	}

	// 6. Exclude path traversal and unsafe path formats
	if !isSafeRelativePath(s) {
		return false
	}

	// 7. Positive match: Has valid file extension starting with a letter
	ext := path.Ext(strings.ReplaceAll(s, "\\", "/"))
	if ext != "" && validExtRegex.MatchString(ext) {
		return true
	}

	// 8. Positive match: Contains directory separators with non-empty segments
	normalized := strings.ReplaceAll(s, "\\", "/")
	if strings.Contains(normalized, "/") {
		parts := strings.Split(normalized, "/")
		validParts := 0
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part != "" && part != "." && part != ".." {
				validParts++
			}
		}
		if validParts >= 2 {
			return true
		}
	}

	// 9. Positive match: Known standalone root filenames (Makefile, Dockerfile, etc.)
	if knownStandaloneFiles[lower] {
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

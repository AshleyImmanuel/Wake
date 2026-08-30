package guard

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"wake/internal/git"
)

var ErrUnreviewedChanges = errors.New("unreviewed human modifications detected in working tree")

// GuardViolation captures details about uncommitted, untracked, or conflicted working tree state.
type GuardViolation struct {
	UntrackedFiles    []string `json:"untracked_files,omitempty"`
	ModifiedFiles     []string `json:"modified_files,omitempty"`
	UnreviewedFiles   []string `json:"unreviewed_files,omitempty"`
	HasMergeConflicts bool     `json:"has_merge_conflicts,omitempty"`
}

func (v *GuardViolation) HasViolations() bool {
	return len(v.UntrackedFiles) > 0 ||
		len(v.ModifiedFiles) > 0 ||
		len(v.UnreviewedFiles) > 0 ||
		v.HasMergeConflicts
}

func (v *GuardViolation) Error() string {
	var sb strings.Builder
	sb.WriteString("PRE-CHECKPOINT GUARD FATAL: unreviewed human modifications detected in working tree\n")
	if v.HasMergeConflicts {
		sb.WriteString("  [!] Unresolved merge conflicts present\n")
	}
	if len(v.UntrackedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("  [!] %d untracked file(s):\n", len(v.UntrackedFiles)))
		for _, f := range v.UntrackedFiles {
			sb.WriteString(fmt.Sprintf("      - %s\n", f))
		}
	}
	if len(v.ModifiedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("  [!] %d modified/uncommitted file(s):\n", len(v.ModifiedFiles)))
		for _, f := range v.ModifiedFiles {
			sb.WriteString(fmt.Sprintf("      - %s\n", f))
		}
	}
	if len(v.UnreviewedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("  [!] %d unreviewed file(s) outside task scope:\n", len(v.UnreviewedFiles)))
		for _, f := range v.UnreviewedFiles {
			sb.WriteString(fmt.Sprintf("      - %s\n", f))
		}
	}
	sb.WriteString("\nAction: Review, stage, commit, or stash changes before checkpointing, or use --force to override.")
	return sb.String()
}

// CheckpointGuardOptions specifies configuration options for pre-checkpoint validation.
type CheckpointGuardOptions struct {
	Force        bool
	TrackedFiles []string
	RepoRoot     string
}

// ValidatePreCheckpoint inspects the working tree state to prevent blind checkpoints
// when unreviewed human modifications or untracked files are present.
func ValidatePreCheckpoint(ctx context.Context, repoState *git.RepositoryState, opts CheckpointGuardOptions) error {
	if opts.Force {
		return nil
	}
	if repoState == nil {
		return errors.New("repository state is nil")
	}

	violation := &GuardViolation{
		UntrackedFiles:  make([]string, 0),
		ModifiedFiles:   make([]string, 0),
		UnreviewedFiles: make([]string, 0),
	}

	if repoState.HasMergeConflicts || len(repoState.UnmergedFiles) > 0 {
		violation.HasMergeConflicts = true
	}

	// Filter untracked files (ignoring Wake and Git metadata)
	for _, u := range repoState.UntrackedFiles {
		clean := normalizePath(u)
		if clean != "" && !isInternalMetadataPath(clean) {
			violation.UntrackedFiles = append(violation.UntrackedFiles, clean)
		}
	}

	// Filter modified files (ignoring metadata)
	for _, m := range repoState.ModifiedFiles {
		clean := normalizePath(m)
		if clean != "" && !isInternalMetadataPath(clean) {
			violation.ModifiedFiles = append(violation.ModifiedFiles, clean)
		}
	}

	// Also check staged / unstaged status files
	for _, f := range repoState.StagedFiles {
		if c := normalizePath(f.Path); c != "" && !isInternalMetadataPath(c) {
			if !containsString(violation.ModifiedFiles, c) {
				violation.ModifiedFiles = append(violation.ModifiedFiles, c)
			}
		}
		if c := normalizePath(f.OrigPath); c != "" && !isInternalMetadataPath(c) {
			if !containsString(violation.ModifiedFiles, c) {
				violation.ModifiedFiles = append(violation.ModifiedFiles, c)
			}
		}
	}
	for _, f := range repoState.UnstagedFiles {
		if c := normalizePath(f.Path); c != "" && !isInternalMetadataPath(c) {
			if !containsString(violation.ModifiedFiles, c) {
				violation.ModifiedFiles = append(violation.ModifiedFiles, c)
			}
		}
		if c := normalizePath(f.OrigPath); c != "" && !isInternalMetadataPath(c) {
			if !containsString(violation.ModifiedFiles, c) {
				violation.ModifiedFiles = append(violation.ModifiedFiles, c)
			}
		}
	}

	violation.UntrackedFiles = deduplicateStrings(violation.UntrackedFiles)
	violation.ModifiedFiles = deduplicateStrings(violation.ModifiedFiles)

	// If specific TrackedFiles are provided, evaluate scope
	if len(opts.TrackedFiles) > 0 {
		normalizedTracked := make([]string, 0, len(opts.TrackedFiles))
		for _, tf := range opts.TrackedFiles {
			if c := normalizePath(tf); c != "" {
				normalizedTracked = append(normalizedTracked, c)
			}
		}

		allDirty := append([]string{}, violation.UntrackedFiles...)
		allDirty = append(allDirty, violation.ModifiedFiles...)
		allDirty = deduplicateStrings(allDirty)

		for _, dirtyFile := range allDirty {
			if !matchesAny(dirtyFile, normalizedTracked) {
				violation.UnreviewedFiles = append(violation.UnreviewedFiles, dirtyFile)
			}
		}

		// When TrackedFiles are supplied, if there are unreviewed files outside tracked scope, fail
		if len(violation.UnreviewedFiles) > 0 || violation.HasMergeConflicts {
			return violation
		}

		// All dirty files are within tracked scope and no conflicts -> permitted
		return nil
	}

	if violation.HasViolations() {
		return violation
	}

	return nil
}

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

func isInternalMetadataPath(p string) bool {
	p = normalizePath(p)
	if p == "" {
		return false
	}
	lower := strings.ToLower(p)
	if strings.HasPrefix(lower, ".wake/") || lower == ".wake" ||
		strings.HasPrefix(lower, ".git/") || lower == ".git" {
		return true
	}
	return false
}

func matchSinglePattern(pattern, filePath string) bool {
	pattern = normalizePath(pattern)
	filePath = normalizePath(filePath)
	if pattern == "" || filePath == "" {
		return false
	}

	if strings.EqualFold(pattern, filePath) {
		return true
	}

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

	if strings.ContainsAny(pattern, "*?[") {
		if matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(filePath)); err == nil && matched {
			return true
		}
		if matched, err := path.Match(strings.ToLower(pattern), strings.ToLower(path.Base(filePath))); err == nil && matched {
			return true
		}
	}

	return false
}

func matchesAny(filePath string, patterns []string) bool {
	for _, p := range patterns {
		if matchSinglePattern(p, filePath) {
			return true
		}
	}
	return false
}

func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

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

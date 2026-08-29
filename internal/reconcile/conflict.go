package reconcile

import (
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/state"
	"strings"
)

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

package git

import (
	"path/filepath"
	"sort"
	"strings"
)

// ParsePorcelainStatus parses the raw output of `git status --porcelain=v1 -uall`.
func ParsePorcelainStatus(output string) *StatusResult {
	result := &StatusResult{
		StagedFiles:    make([]FileStatus, 0),
		UnstagedFiles:  make([]FileStatus, 0),
		UntrackedFiles: make([]string, 0),
		UnmergedFiles:  make([]string, 0),
	}

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if len(line) < 3 {
			continue
		}

		x := line[0]
		y := line[1]
		rawPath := line[3:]

		// Handle unmerged / merge conflict states
		if isUnmerged(x, y) {
			origPath, newPath := parseRenamePath(rawPath)
			targetPath := newPath
			if targetPath == "" {
				targetPath = origPath
			}
			if targetPath != "" {
				result.UnmergedFiles = append(result.UnmergedFiles, targetPath)
			}
			continue
		}

		// Handle untracked files
		if x == '?' && y == '?' {
			p := cleanPath(rawPath)
			if p != "" {
				result.UntrackedFiles = append(result.UntrackedFiles, p)
			}
			continue
		}

		// Handle ignored files
		if x == '!' && y == '!' {
			continue
		}

		// Handle staged changes (index)
		if x != ' ' && x != '?' {
			origPath, newPath := parseRenamePath(rawPath)
			result.StagedFiles = append(result.StagedFiles, FileStatus{
				Path:           newPath,
				OrigPath:       origPath,
				StagingStatus:  StatusCode(string(x)),
				WorkTreeStatus: StatusCode(string(y)),
			})
		}

		// Handle unstaged changes (worktree)
		if y != ' ' && y != '?' {
			origPath, newPath := parseRenamePath(rawPath)
			result.UnstagedFiles = append(result.UnstagedFiles, FileStatus{
				Path:           newPath,
				OrigPath:       origPath,
				StagingStatus:  StatusCode(string(x)),
				WorkTreeStatus: StatusCode(string(y)),
			})
		}
	}

	result.IsClean = len(result.StagedFiles) == 0 &&
		len(result.UnstagedFiles) == 0 &&
		len(result.UntrackedFiles) == 0 &&
		len(result.UnmergedFiles) == 0

	return result
}

// isUnmerged returns true if the status pair represents a merge conflict.
func isUnmerged(x, y byte) bool {
	// Standard git unmerged combinations: DD, AU, UD, UA, DU, AA, UU
	if x == 'U' || y == 'U' {
		return true
	}
	if x == 'A' && y == 'A' {
		return true
	}
	if x == 'D' && y == 'D' {
		return true
	}
	return false
}

// parseRenamePath splits a rename/copy path string ("old_path -> new_path") into individual paths.
func parseRenamePath(raw string) (origPath, newPath string) {
	if idx := strings.Index(raw, " -> "); idx != -1 {
		orig := cleanPath(raw[:idx])
		dest := cleanPath(raw[idx+4:])
		return orig, dest
	}
	return "", cleanPath(raw)
}

// cleanPath unquotes and normalizes a file path using forward slashes.
func cleanPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, "\"")
	if p == "" {
		return ""
	}
	cleaned := filepath.ToSlash(filepath.Clean(p))
	cleaned = strings.TrimPrefix(cleaned, "./")
	return cleaned
}

// ExtractModifiedFiles extracts all unique modified, staged, unstaged, untracked, and unmerged file paths.
func ExtractModifiedFiles(status *StatusResult) []string {
	if status == nil {
		return []string{}
	}

	seen := make(map[string]struct{})

	addPath := func(p string) {
		if p != "" {
			seen[p] = struct{}{}
		}
	}

	for _, f := range status.StagedFiles {
		addPath(f.Path)
		addPath(f.OrigPath)
	}

	for _, f := range status.UnstagedFiles {
		addPath(f.Path)
		addPath(f.OrigPath)
	}

	for _, p := range status.UntrackedFiles {
		addPath(p)
	}

	for _, p := range status.UnmergedFiles {
		addPath(p)
	}

	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}

	sort.Strings(result)
	return result
}

// ParseNameOnlyList parses output from `git diff --name-only` into normalized paths.
func ParseNameOnlyList(output string) []string {
	lines := strings.Split(output, "\n")
	seen := make(map[string]struct{})
	result := make([]string, 0)

	for _, line := range lines {
		p := cleanPath(line)
		if p != "" {
			if _, exists := seen[p]; !exists {
				seen[p] = struct{}{}
				result = append(result, p)
			}
		}
	}

	sort.Strings(result)
	return result
}

// ParseDiffNameStatus parses output from `git diff --name-status` into FileChange structs.
func ParseDiffNameStatus(output string) []FileChange {
	lines := strings.Split(output, "\n")
	changes := make([]FileChange, 0)

	for _, line := range lines {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 2 {
			continue
		}

		statusCode := StatusCode(strings.TrimSpace(parts[0]))
		if len(statusCode) > 1 && (statusCode[0] == 'R' || statusCode[0] == 'C') {
			statusCode = StatusCode(string(statusCode[0]))
		}

		if len(parts) >= 3 && (statusCode == StatusRenamed || statusCode == StatusCopied) {
			origPath := cleanPath(parts[1])
			newPath := cleanPath(parts[2])
			changes = append(changes, FileChange{
				Path:     newPath,
				OrigPath: origPath,
				Status:   statusCode,
			})
		} else {
			path := cleanPath(parts[1])
			changes = append(changes, FileChange{
				Path:   path,
				Status: statusCode,
			})
		}
	}

	return changes
}

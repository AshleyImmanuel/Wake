package reconcile

import (
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
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

// isInternalMetadataPath identifies Wake and Git internal working files.
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
	for i, t := range tokens {
		if stopWords[strings.ToLower(t)] {
			continue
		}
		isPath := looksLikeFilePath(t) || (strings.Contains(t, "/") && !strings.Contains(t, "://") && isSafeRelativePath(strings.ReplaceAll(t, "*", "x")))

		if !isPath && i+1 < len(tokens) {
			next := strings.ToLower(tokens[i+1])
			if next == "directory" || next == "dir" || next == "folder" || next == "package" || next == "pkg" || next == "module" {
				isPath = true
			}
		}

		if isPath {
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

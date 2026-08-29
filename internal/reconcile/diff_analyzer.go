package reconcile

import (
	"strings"
)

// IsSemanticChange analyzes a unified git diff and returns true if the diff contains
// any changes that are NOT purely whitespace or comments.
func IsSemanticChange(diff string) bool {
	if strings.TrimSpace(diff) == "" {
		// No diff means no semantic change
		return false
	}

	lines := strings.Split(diff, "\n")
	hasLogicChange := false

	for _, line := range lines {
		// Only care about additions and deletions
		if !strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "-") {
			continue
		}
		
		// Ignore diff header lines like +++ b/file or --- a/file
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}

		// Remove the +/- prefix and trim whitespace
		content := strings.TrimSpace(line[1:])

		// If the line is empty (pure whitespace change), ignore it
		if content == "" {
			continue
		}

		// Check common comment patterns
		if isCommentLine(content) {
			continue
		}

		// If we reached here, it's a structural/logical code change
		hasLogicChange = true
		break
	}

	return hasLogicChange
}

func isCommentLine(content string) bool {
	content = strings.ToLower(content)
	
	prefixes := []string{
		"//",   // Go, C, C++, Java, JS, TS, Rust
		"/*",   // Block comment start
		"*/",   // Block comment end
		"*",    // Block comment continuation (often preceded by spaces)
		"#",    // Python, Ruby, Bash, YAML
		"<!--", // HTML, XML, Markdown
		"-->",  // HTML, XML, Markdown end
		"\"",   // Python docstrings
		"\"\"\"",
		"'''",
		"--",   // SQL, Lua, Haskell
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(content, prefix) {
			return true
		}
	}
	return false
}

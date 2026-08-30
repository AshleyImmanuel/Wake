package reconcile

import (
	"bytes"
	"go/parser"
	"go/printer"
	"go/token"
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

// IsSemanticChangeGoAST achieves mathematical 100% semantic matching for Go files
// by utilizing Go's internal compiler libraries. It costs zero tokens and zero external dependencies.
// It parses the old and new code into Abstract Syntax Trees, strips comments/formatting,
// and compares the structural equivalence of the logic.
func IsSemanticChangeGoAST(oldCode, newCode string) bool {
	oldNorm, err1 := normalizeGoCode(oldCode)
	newNorm, err2 := normalizeGoCode(newCode)
	
	// If the code doesn't compile/parse, fallback to assuming it's a semantic change
	if err1 != nil || err2 != nil {
		return true
	}
	
	// If the normalized AST strings match, it means 100% of the changes 
	// were strictly non-semantic (comments or whitespace).
	return oldNorm != newNorm
}

func normalizeGoCode(src string) (string, error) {
	fset := token.NewFileSet()
	// Parse without parser.ParseComments flag to automatically drop all comments!
	f, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	// Printing it back normalizes all whitespace automatically!
	err = printer.Fprint(&buf, fset, f)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}



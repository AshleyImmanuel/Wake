package guard

import (
	"context"
	"strings"
	"testing"

	"github.com/wake/wake/internal/git"
)

func TestValidatePreCheckpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("Force=true bypasses all validation", func(t *testing.T) {
		opts := CheckpointGuardOptions{Force: true}
		err := ValidatePreCheckpoint(ctx, nil, opts)
		if err != nil {
			t.Errorf("Expected nil error with Force=true, got %v", err)
		}
	})

	t.Run("Nil repoState returns error", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		err := ValidatePreCheckpoint(ctx, nil, opts)
		if err == nil || err.Error() != "repository state is nil" {
			t.Errorf("Expected repository state is nil error, got %v", err)
		}
	})

	t.Run("Clean repository returns nil", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		repoState := &git.RepositoryState{}
		err := ValidatePreCheckpoint(ctx, repoState, opts)
		if err != nil {
			t.Errorf("Expected nil error for clean repo, got %v", err)
		}
	})

	t.Run("Untracked files trigger violation", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		repoState := &git.RepositoryState{
			UntrackedFiles: []string{"new.txt"},
		}
		err := ValidatePreCheckpoint(ctx, repoState, opts)
		if err == nil {
			t.Fatal("Expected error for untracked files, got nil")
		}
		violation, ok := err.(*GuardViolation)
		if !ok || len(violation.UntrackedFiles) != 1 {
			t.Errorf("Expected GuardViolation with 1 untracked file, got %v", err)
		}
	})

	t.Run("Modified files trigger violation", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		repoState := &git.RepositoryState{
			ModifiedFiles: []string{"modified.txt"},
		}
		err := ValidatePreCheckpoint(ctx, repoState, opts)
		if err == nil {
			t.Fatal("Expected error for modified files, got nil")
		}
		violation, ok := err.(*GuardViolation)
		if !ok || len(violation.ModifiedFiles) != 1 {
			t.Errorf("Expected GuardViolation with 1 modified file, got %v", err)
		}
	})

	t.Run("Merge conflicts trigger violation", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		repoState := &git.RepositoryState{
			HasMergeConflicts: true,
		}
		err := ValidatePreCheckpoint(ctx, repoState, opts)
		if err == nil {
			t.Fatal("Expected error for merge conflicts, got nil")
		}
		violation, ok := err.(*GuardViolation)
		if !ok || !violation.HasMergeConflicts {
			t.Errorf("Expected GuardViolation with HasMergeConflicts=true, got %v", err)
		}
	})

	t.Run("Internal metadata paths are ignored", func(t *testing.T) {
		opts := CheckpointGuardOptions{}
		repoState := &git.RepositoryState{
			UntrackedFiles: []string{".wake/state.json", ".sentinel/config", ".git/info/exclude"},
			ModifiedFiles:  []string{".wake", ".git"},
		}
		err := ValidatePreCheckpoint(ctx, repoState, opts)
		if err != nil {
			t.Errorf("Expected nil error for internal metadata paths, got %v", err)
		}
	})

	t.Run("TrackedFiles scope: dirty files within scope pass, outside scope fail", func(t *testing.T) {
		repoState := &git.RepositoryState{
			ModifiedFiles:  []string{"src/main.go", "src/other.go"},
			UntrackedFiles: []string{"test/test.go"},
		}

		// Within scope (partial) - should fail because other.go and test.go are outside
		opts1 := CheckpointGuardOptions{TrackedFiles: []string{"src/main.go"}}
		err := ValidatePreCheckpoint(ctx, repoState, opts1)
		if err == nil {
			t.Fatal("Expected error for out of scope files, got nil")
		}
		violation, ok := err.(*GuardViolation)
		if !ok || len(violation.UnreviewedFiles) != 2 {
			t.Errorf("Expected 2 unreviewed files, got %v", err)
		}

		// All within scope - should pass
		opts2 := CheckpointGuardOptions{TrackedFiles: []string{"src/", "test/"}}
		err = ValidatePreCheckpoint(ctx, repoState, opts2)
		if err != nil {
			t.Errorf("Expected nil error when all files are tracked, got %v", err)
		}
	})
}

func TestGuardViolationMethods(t *testing.T) {
	v := &GuardViolation{}
	if v.HasViolations() {
		t.Errorf("Expected empty violation to have no violations")
	}

	v.UntrackedFiles = []string{"a"}
	if !v.HasViolations() {
		t.Errorf("Expected violation with untracked files to be true")
	}

	v = &GuardViolation{
		HasMergeConflicts: true,
		UntrackedFiles:    []string{"untracked1", "untracked2"},
		ModifiedFiles:     []string{"mod1"},
		UnreviewedFiles:   []string{"unrev1"},
	}
	errStr := v.Error()
	if !strings.Contains(errStr, "PRE-CHECKPOINT GUARD FATAL") ||
		!strings.Contains(errStr, "Unresolved merge conflicts present") ||
		!strings.Contains(errStr, "2 untracked file(s):") ||
		!strings.Contains(errStr, "1 modified/uncommitted file(s):") ||
		!strings.Contains(errStr, "1 unreviewed file(s) outside task scope:") {
		t.Errorf("Error string format mismatch. Got:\n%s", errStr)
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Run("normalizePath", func(t *testing.T) {
		tests := []struct {
			input    string
			expected string
		}{
			{`.\foo\bar`, "foo/bar"},
			{`./foo/bar`, "foo/bar"},
			{`/foo/bar`, "foo/bar"},
			{`"foo/bar"`, "foo/bar"},
			{`''`, ""},
			{`.`, ""},
			{`./`, ""},
		}
		for _, tt := range tests {
			res := normalizePath(tt.input)
			if res != tt.expected {
				t.Errorf("normalizePath(%q) = %q, want %q", tt.input, res, tt.expected)
			}
		}
	})

	t.Run("isInternalMetadataPath", func(t *testing.T) {
		tests := []struct {
			input    string
			expected bool
		}{
			{".wake", true},
			{".wake/file", true},
			{".sentinel", true},
			{".git", true},
			{".git/config", true},
			{"normal/path", false},
			{"", false},
		}
		for _, tt := range tests {
			res := isInternalMetadataPath(tt.input)
			if res != tt.expected {
				t.Errorf("isInternalMetadataPath(%q) = %v, want %v", tt.input, res, tt.expected)
			}
		}
	})

	t.Run("matchSinglePattern", func(t *testing.T) {
		tests := []struct {
			pattern  string
			filePath string
			expected bool
		}{
			{"foo.txt", "foo.txt", true},
			{"foo.txt", "FOO.txt", true}, // case insensitive match
			{"dir/", "dir/file.txt", true},
			{"dir/*", "dir/file.txt", true},
			{"*.txt", "dir/file.txt", true}, // matches base
			{"*.txt", "file.txt", true},
			{"dir", "dir/file.txt", true}, // directory prefix
		}
		for _, tt := range tests {
			res := matchSinglePattern(tt.pattern, tt.filePath)
			if res != tt.expected {
				t.Errorf("matchSinglePattern(%q, %q) = %v, want %v", tt.pattern, tt.filePath, res, tt.expected)
			}
		}
	})

	t.Run("matchesAny", func(t *testing.T) {
		patterns := []string{"*.txt", "dir/"}
		if !matchesAny("foo.txt", patterns) {
			t.Errorf("Expected matchesAny to return true for foo.txt")
		}
		if matchesAny("foo.go", patterns) {
			t.Errorf("Expected matchesAny to return false for foo.go")
		}
	})

	t.Run("deduplicateStrings", func(t *testing.T) {
		input := []string{"b", "a", "c", "a", "b", "", "."}
		expected := []string{"a", "b", "c"}
		res := deduplicateStrings(input)
		if strings.Join(res, ",") != strings.Join(expected, ",") {
			t.Errorf("deduplicateStrings mismatch: got %v, want %v", res, expected)
		}
	})
}

func TestSecurityFocused(t *testing.T) {
	t.Run("Path traversal attempts", func(t *testing.T) {
		repoState := &git.RepositoryState{
			ModifiedFiles: []string{"../../../etc/passwd"},
		}
		opts := CheckpointGuardOptions{TrackedFiles: []string{"src/"}}
		err := ValidatePreCheckpoint(context.Background(), repoState, opts)
		if err == nil {
			t.Fatal("Expected path traversal to be rejected as out of scope")
		}
		violation := err.(*GuardViolation)
		if len(violation.UnreviewedFiles) != 1 {
			t.Errorf("Expected 1 unreviewed file for path traversal")
		}
	})

	t.Run("Absolute paths are rejected", func(t *testing.T) {
		repoState := &git.RepositoryState{
			ModifiedFiles: []string{"/etc/passwd"},
		}
		opts := CheckpointGuardOptions{TrackedFiles: []string{"src/"}}
		err := ValidatePreCheckpoint(context.Background(), repoState, opts)
		if err == nil {
			t.Fatal("Expected absolute path to be rejected")
		}
	})

	t.Run("UNC paths are rejected", func(t *testing.T) {
		repoState := &git.RepositoryState{
			ModifiedFiles: []string{`\\server\share\file`},
		}
		opts := CheckpointGuardOptions{TrackedFiles: []string{"src/"}}
		err := ValidatePreCheckpoint(context.Background(), repoState, opts)
		if err == nil {
			t.Fatal("Expected UNC path to be rejected")
		}
	})

	t.Run("Windows drive letter paths rejected", func(t *testing.T) {
		repoState := &git.RepositoryState{
			ModifiedFiles: []string{`C:\Windows\System32\cmd.exe`},
		}
		opts := CheckpointGuardOptions{TrackedFiles: []string{"src/"}}
		err := ValidatePreCheckpoint(context.Background(), repoState, opts)
		if err == nil {
			t.Fatal("Expected drive letter path to be rejected")
		}
	})
}

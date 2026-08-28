package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
)

// TestChallenger_PathTraversalMatrix tests path traversal patterns across all reconcile functions.
func TestChallenger_PathTraversalMatrix(t *testing.T) {
	traversals := []string{
		"../",
		"../../",
		"../../../etc/passwd",
		"../../../../windows/system32/cmd.exe",
		"..\\..\\windows\\system32",
		"foo/../../bar",
		"foo/bar/../../../baz",
		"pkg/../../../secret.txt",
		"./../../escape.go",
		"dir/..",
		"..",
		"/etc/passwd",
		"/root/.ssh/id_rsa",
		"\\Windows\\System32\\cmd.exe",
		`\\?\C:\Windows`,
		`\\.\COM1`,
		`\\localhost\c$\secret`,
		`\\192.168.1.1\share\test`,
		`//server/share/payload`,
		`C:\secret.txt`,
		`C:/secret.txt`,
		`D:\project\file.go`,
		`d:/project/file.go`,
		`Z:\confidential`,
		`C:file_in_current_dir.go`,
	}

	for _, path := range traversals {
		t.Run("isSafeRelativePath_"+path, func(t *testing.T) {
			if isSafeRelativePath(path) {
				t.Errorf("isSafeRelativePath(%q) should be false", path)
			}
		})

		t.Run("looksLikeFilePath_"+path, func(t *testing.T) {
			if looksLikeFilePath(path) {
				t.Errorf("looksLikeFilePath(%q) should be false", path)
			}
		})
	}
}

// TestChallenger_WildcardAndNoiseMatrix tests that wildcards, URLs, step counters, versions, and abbreviations
// do NOT cause false CONFLICTs when present in claims or constraints.
func TestChallenger_WildcardAndNoiseMatrix(t *testing.T) {
	nonFileTokens := []struct {
		category string
		token    string
	}{
		{"wildcard", "*.go"},
		{"wildcard", "pkg/*.go"},
		{"wildcard", "?est.go"},
		{"wildcard", "[a-z]*.go"},
		{"wildcard", "{a,b}.go"},
		{"wildcard", "src/**/file.go"},
		{"url", "http://example.com/api"},
		{"url", "https://github.com/wake/wake/releases"},
		{"url", "ftp://files.org/pub"},
		{"url", "file:///C:/local/path"},
		{"url", "git://github.com/wake/wake"},
		{"url", "ws://localhost:8080/events"},
		{"url", "wss://secure.org/stream"},
		{"version", "v1.0.0"},
		{"version", "v2.0"},
		{"version", "v0.1.2-alpha"},
		{"version", "1.0.0"},
		{"version", "2.1"},
		{"version", "1.1.2"},
		{"version", "v10.12.33"},
		{"step", "1."},
		{"step", "2."},
		{"step", "1.1."},
		{"step", "1.1.2."},
		{"step", "#1"},
		{"step", "#12"},
		{"abbreviation", "e.g."},
		{"abbreviation", "i.e."},
		{"abbreviation", "etc."},
		{"abbreviation", "ex."},
		{"abbreviation", "vs."},
		{"abbreviation", "approx."},
		{"abbreviation", "est."},
	}

	for _, tc := range nonFileTokens {
		t.Run(tc.category+"_"+tc.token, func(t *testing.T) {
			if looksLikeFilePath(tc.token) {
				t.Errorf("looksLikeFilePath(%q) in category %s should be false", tc.token, tc.category)
			}
		})
	}
}

// TestChallenger_ReconcileRepo_AdversarialClaims stress-tests ReconcileRepo against arbitrary adversarial claim strings.
func TestChallenger_ReconcileRepo_AdversarialClaims(t *testing.T) {
	tmpDir := t.TempDir()
	commit := "abcdef1234567890abcdef1234567890abcdef12"

	// Create valid file in repo
	validFile := "valid/service.go"
	fullValidPath := filepath.Join(tmpDir, filepath.FromSlash(validFile))
	if err := os.MkdirAll(filepath.Dir(fullValidPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(fullValidPath, []byte("package valid\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	mockClient := &mockGitClient{
		state: &git.RepositoryState{
			RootPath:   tmpDir,
			Branch:     "main",
			CommitHash: commit,
			HasCommits: true,
			IsClean:    true,
		},
		commitExists: true,
		isAncestor:   true,
	}

	adversarialClaims := []string{
		"../../../../etc/passwd",
		`C:\Windows\System32\cmd.exe`,
		`\\host\share\data.db`,
		"Created *.go interface bindings",
		"Upgraded framework to v2.0.1",
		"1.1.2 Configured security headers, e.g. HSTS",
		"Refer to documentation at https://sentinel.dev/guide",
		"valid/service.go", // This one exists on disk!
	}

	cp := state.Checkpoint{
		ID:         uuid.New(),
		TaskID:     uuid.New(),
		Branch:     "main",
		Commit:     commit,
		Repository: "github.com/wake/wake",
		StateData: state.State{
			Completed: adversarialClaims,
		},
	}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusSafe {
		t.Fatalf("expected StatusSafe when all noise/traversal claims are safely filtered and valid file exists, got %s (reason: %s, invalidated: %v)",
			result.Status, result.Reason, result.InvalidatedClaims)
	}

	if len(result.InvalidatedClaims) != 0 {
		t.Errorf("expected 0 invalidated claims, got: %v", result.InvalidatedClaims)
	}
}

// TestChallenger_ValidFilePatterns confirms that legitimate file patterns are recognized.
func TestChallenger_ValidFilePatterns(t *testing.T) {
	validFiles := []string{
		"main.go",
		"cmd/root.go",
		"internal/reconcile/engine.go",
		"pkg/auth/session_test.go",
		"README.md",
		"Makefile",
		"Dockerfile",
		"go.mod",
		"go.sum",
		"config.json",
		"schema.sql",
		"sub/dir/nested/deep/file.ts",
	}

	for _, vf := range validFiles {
		t.Run("valid_"+vf, func(t *testing.T) {
			if !looksLikeFilePath(vf) {
				t.Errorf("looksLikeFilePath(%q) should be true", vf)
			}
			if !isSafeRelativePath(vf) {
				t.Errorf("isSafeRelativePath(%q) should be true", vf)
			}
		})
	}
}

// TestChallenger_ContainmentBoundary tests directory containment boundary conditions.
func TestChallenger_ContainmentBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// Safe inner paths
	if _, ok := resolveSafeRepoPath(tmpDir, "a/b/c.go"); !ok {
		t.Errorf("expected resolveSafeRepoPath to succeed for a/b/c.go")
	}

	// Escape via subfolder ..
	if _, ok := resolveSafeRepoPath(tmpDir, "sub/../../escape.txt"); ok {
		t.Errorf("expected resolveSafeRepoPath to reject sub/../../escape.txt")
	}

	// Escape via absolute drive
	if _, ok := resolveSafeRepoPath(tmpDir, `C:\Windows\System32`); ok {
		t.Errorf("expected resolveSafeRepoPath to reject C:\\Windows\\System32")
	}

	// Escape via UNC
	if _, ok := resolveSafeRepoPath(tmpDir, `\\server\share\file`); ok {
		t.Errorf("expected resolveSafeRepoPath to reject UNC path")
	}
}

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

// TestAdversarial_ComplexConstraintPunctuation tests multi-word constraints with complex punctuation,
// brackets, quotes, abbreviations, and nested paths.
func TestAdversarial_ComplexConstraintPunctuation(t *testing.T) {
	testCases := []struct {
		name       string
		filePath   string
		constraint string
		expected   bool
	}{
		{
			name:       "Bracketed path in sentence",
			filePath:   "pkg/auth/session.go",
			constraint: "Do NOT edit [pkg/auth/session.go] under any circumstances!",
			expected:   true,
		},
		{
			name:       "Quoted path with exclamation",
			filePath:   "internal/db/store.go",
			constraint: "Refrain from touching \"internal/db/store.go\"!",
			expected:   true,
		},
		{
			name:       "Backtick quoted path",
			filePath:   "cmd/root.go",
			constraint: "Never modify `cmd/root.go` without review.",
			expected:   true,
		},
		{
			name:       "Parenthetical path with commas",
			filePath:   "internal/git/client.go",
			constraint: "Protected files: (internal/git/client.go, internal/git/parser.go).",
			expected:   true,
		},
		{
			name:       "Multi-word with abbreviations - should match actual path",
			filePath:   "pkg/auth/session.go",
			constraint: "Ensure security headers, e.g. in pkg/auth/session.go vs. legacy code.",
			expected:   true,
		},
		{
			name:       "Multi-word with abbreviation and unrelated file - should NOT match",
			filePath:   "cmd/status.go",
			constraint: "Ensure security headers, e.g. in pkg/auth/session.go vs. legacy code.",
			expected:   false,
		},
		{
			name:       "Constraint mentions version number - should NOT match unrelated file",
			filePath:   "v2.0",
			constraint: "Upgrade framework to v2.0 for security compliance.",
			expected:   false,
		},
		{
			name:       "Constraint mentions step counter - should NOT match numeric named file",
			filePath:   "1.2",
			constraint: "1.2. Complete the database migrations.",
			expected:   false,
		},
		{
			name:       "Constraint mentions URL - should NOT match URL string file",
			filePath:   "https://wake.dev/guide",
			constraint: "Refer to guidelines at https://wake.dev/guide for instructions.",
			expected:   false,
		},
		{
			name:       "Nested directory prefix match",
			filePath:   "internal/service/task_service.go",
			constraint: "Do not touch internal/service directory",
			expected:   true,
		},
		{
			name:       "Windows backslash format in constraint",
			filePath:   "internal/db/db.go",
			constraint: `Do not modify internal\db\db.go`,
			expected:   true,
		},
		{
			name:       "Case-insensitive constraint match",
			filePath:   "INTERNAL/GUARD/GUARD.GO",
			constraint: "Protected: internal/guard/guard.go",
			expected:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchesConstraint(tc.filePath, tc.constraint)
			if got != tc.expected {
				t.Errorf("matchesConstraint(%q, %q) = %v; expected %v", tc.filePath, tc.constraint, got, tc.expected)
			}
		})
	}
}

// TestAdversarial_ActiveVsSupersededDecisions tests that only ACTIVE decisions trigger CONFLICT.
func TestAdversarial_ActiveVsSupersededDecisions(t *testing.T) {
	cp := state.Checkpoint{
		Commit: "commit1",
		Branch: "main",
		StateData: state.State{
			Decisions: []state.Decision{
				{
					ID:          "dec-1",
					Description: "Preserve legacy database schema in pkg/db/schema.sql",
					Status:      "SUPERSEDED",
				},
				{
					ID:          "dec-2",
					Description: "Do not alter internal/crypto/keys.go",
					Status:      "ACTIVE",
				},
			},
		},
	}

	// 1. Modifying file from SUPERSEDED decision -> Should NOT cause CONFLICT
	repo1 := git.RepositoryState{
		CommitHash:    "commit1",
		Branch:        "main",
		ModifiedFiles: []string{"pkg/db/schema.sql"},
	}
	res1 := Reconcile(cp, repo1, nil)
	if res1.Status == StatusConflict {
		t.Fatalf("modifying superseded decision file should NOT trigger CONFLICT, got: %s (reason: %s)", res1.Status, res1.Reason)
	}

	// 2. Modifying file from ACTIVE decision -> MUST cause CONFLICT
	repo2 := git.RepositoryState{
		CommitHash:    "commit1",
		Branch:        "main",
		ModifiedFiles: []string{"internal/crypto/keys.go"},
	}
	res2 := Reconcile(cp, repo2, nil)
	if res2.Status != StatusConflict {
		t.Fatalf("modifying active decision file MUST trigger CONFLICT, got: %s", res2.Status)
	}
	if len(res2.ConstraintViolations) == 0 {
		t.Errorf("expected constraint violations to record active decision violation")
	}
}

// TestAdversarial_DoNotRepeatAndCompletedClaims tests invalidation of completed/do-not-repeat claims.
func TestAdversarial_DoNotRepeatAndCompletedClaims(t *testing.T) {
	cp := state.Checkpoint{
		Commit: "commit1",
		Branch: "main",
		StateData: state.State{
			Completed: []string{
				"Created internal/guard/guard.go implementation",
			},
			DoNotRepeat: []string{
				"Refactored cmd/checkpoint.go",
			},
		},
	}

	// Case 1: Modifying completed file -> CONFLICT
	repo1 := git.RepositoryState{
		CommitHash:    "commit1",
		Branch:        "main",
		ModifiedFiles: []string{"internal/guard/guard.go"},
	}
	res1 := Reconcile(cp, repo1, nil)
	if res1.Status != StatusConflict {
		t.Fatalf("modifying completed artifact must result in CONFLICT, got: %s", res1.Status)
	}
	if len(res1.InvalidatedClaims) == 0 {
		t.Errorf("expected InvalidatedClaims for completed file")
	}

	// Case 2: Deleting do-not-repeat file -> CONFLICT
	repo2 := git.RepositoryState{
		CommitHash: "commit1",
		Branch:     "main",
		StagedFiles: []git.FileStatus{
			{Path: "cmd/checkpoint.go", StagingStatus: git.StatusDeleted},
		},
	}
	res2 := Reconcile(cp, repo2, nil)
	if res2.Status != StatusConflict {
		t.Fatalf("deleting do-not-repeat artifact must result in CONFLICT, got: %s", res2.Status)
	}
}

// TestAdversarial_PhysicalMissingFileCheck tests ReconcileRepo detecting missing claimed files on disk.
func TestAdversarial_PhysicalMissingFileCheck(t *testing.T) {
	tmpDir := t.TempDir()
	commit := "abc12345"

	// Create real file in repo
	existingFile := filepath.Join(tmpDir, "internal", "service.go")
	_ = os.MkdirAll(filepath.Dir(existingFile), 0755)
	_ = os.WriteFile(existingFile, []byte("package internal\n"), 0644)

	cp := state.Checkpoint{
		ID:         uuid.New(),
		TaskID:     uuid.New(),
		Commit:     commit,
		Branch:     "main",
		Repository: tmpDir,
		StateData: state.State{
			Completed: []string{
				"Created internal/service.go",       // Exists!
				"Created pkg/deleted/missing.go",     // Does NOT exist on disk!
			},
		},
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

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	// Must detect missing file and mark as CONFLICT
	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT on physically missing claimed file, got: %s (reason: %s)", result.Status, result.Reason)
	}
	if len(result.InvalidatedClaims) == 0 {
		t.Errorf("expected InvalidatedClaims to contain missing file notice")
	}
}

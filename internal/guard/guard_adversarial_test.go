package guard

import (
	"context"
	"testing"

	"github.com/wake/wake/internal/git"
)

// TestAdversarial_UntrackedAndHiddenFiles tests that untracked and hidden files
// (other than .wake, .sentinel, .git) are strictly caught by the guard.
func TestAdversarial_UntrackedAndHiddenFiles(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name          string
		untracked     []string
		shouldViolate bool
	}{
		{
			name:          "DotEnv file",
			untracked:     []string{".env"},
			shouldViolate: true,
		},
		{
			name:          "DotEnv local file",
			untracked:     []string{".env.local"},
			shouldViolate: true,
		},
		{
			name:          "Gitignore file",
			untracked:     []string{".gitignore"},
			shouldViolate: true,
		},
		{
			name:          "Gitattributes file",
			untracked:     []string{".gitattributes"},
			shouldViolate: true,
		},
		{
			name:          "Github workflow file",
			untracked:     []string{".github/workflows/ci.yml"},
			shouldViolate: true,
		},
		{
			name:          "Gitlab CI file",
			untracked:     []string{".gitlab-ci.yml"},
			shouldViolate: true,
		},
		{
			name:          "Wake backup directory",
			untracked:     []string{".wake_backup/state.db"},
			shouldViolate: true,
		},
		{
			name:          "Sentinel old directory",
			untracked:     []string{".sentinel_old/config.json"},
			shouldViolate: true,
		},
		{
			name:          "Nested wake directory inside src",
			untracked:     []string{"src/.wake/cache.json"},
			shouldViolate: true,
		},
		{
			name:          "Nested sentinel directory inside pkg",
			untracked:     []string{"pkg/.sentinel/state.json"},
			shouldViolate: true,
		},
		{
			name:          "Nested git directory inside vendor",
			untracked:     []string{"vendor/module/.git/config"},
			shouldViolate: true,
		},
		{
			name:          "File with spaces",
			untracked:     []string{"my secret plan.txt"},
			shouldViolate: true,
		},
		{
			name:          "File with unicode",
			untracked:     []string{"résumé_notes.md"},
			shouldViolate: true,
		},
		{
			name:          "Deeply nested file",
			untracked:     []string{"a/b/c/d/e/f/g/file.go"},
			shouldViolate: true,
		},
		{
			name:          "Legitimate metadata - wake state",
			untracked:     []string{".wake/state.db"},
			shouldViolate: false,
		},
		{
			name:          "Legitimate metadata - sentinel config",
			untracked:     []string{".sentinel/config.json"},
			shouldViolate: false,
		},
		{
			name:          "Legitimate metadata - git HEAD",
			untracked:     []string{".git/HEAD"},
			shouldViolate: false,
		},
		{
			name:          "Legitimate metadata - Windows backslash",
			untracked:     []string{`.wake\state.db`, `.sentinel\db.sqlite`, `.git\index`},
			shouldViolate: false,
		},
		{
			name:          "Legitimate metadata - uppercase variation",
			untracked:     []string{".WAKE/state.db", ".SENTINEL/audit.log", ".GIT/config"},
			shouldViolate: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoState := &git.RepositoryState{
				RootPath:       "/repo",
				Branch:         "main",
				CommitHash:     "abc1234",
				IsClean:        false,
				UntrackedFiles: tc.untracked,
			}

			err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{})
			if tc.shouldViolate {
				if err == nil {
					t.Fatalf("expected violation for %v, got nil", tc.untracked)
				}
				v, ok := err.(*GuardViolation)
				if !ok {
					t.Fatalf("expected *GuardViolation, got %T", err)
				}
				if len(v.UntrackedFiles) != len(tc.untracked) {
					t.Errorf("expected %d untracked violation files, got %d (%v)", len(tc.untracked), len(v.UntrackedFiles), v.UntrackedFiles)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no violation for metadata files %v, got: %v", tc.untracked, err)
				}
			}
		})
	}
}

// TestAdversarial_StagedAndUnstagedStatusTypes tests that staged adds, deletes, renames, copies,
// and unstaged changes are all caught.
func TestAdversarial_StagedAndUnstagedStatusTypes(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name          string
		state         *git.RepositoryState
		expectedFiles []string
	}{
		{
			name: "Staged addition",
			state: &git.RepositoryState{
				CommitHash: "c1",
				StagedFiles: []git.FileStatus{
					{Path: "pkg/newfile.go", StagingStatus: git.StatusAdded, WorkTreeStatus: git.StatusUnmodified},
				},
			},
			expectedFiles: []string{"pkg/newfile.go"},
		},
		{
			name: "Staged deletion",
			state: &git.RepositoryState{
				CommitHash: "c1",
				StagedFiles: []git.FileStatus{
					{Path: "pkg/oldfile.go", StagingStatus: git.StatusDeleted, WorkTreeStatus: git.StatusUnmodified},
				},
			},
			expectedFiles: []string{"pkg/oldfile.go"},
		},
		{
			name: "Staged rename",
			state: &git.RepositoryState{
				CommitHash: "c1",
				StagedFiles: []git.FileStatus{
					{Path: "pkg/renamed.go", OrigPath: "pkg/original.go", StagingStatus: git.StatusRenamed, WorkTreeStatus: git.StatusUnmodified},
				},
			},
			expectedFiles: []string{"pkg/original.go", "pkg/renamed.go"},
		},
		{
			name: "Unstaged deletion",
			state: &git.RepositoryState{
				CommitHash: "c1",
				UnstagedFiles: []git.FileStatus{
					{Path: "cmd/deleted.go", StagingStatus: git.StatusUnmodified, WorkTreeStatus: git.StatusDeleted},
				},
			},
			expectedFiles: []string{"cmd/deleted.go"},
		},
		{
			name: "Unstaged type change",
			state: &git.RepositoryState{
				CommitHash: "c1",
				UnstagedFiles: []git.FileStatus{
					{Path: "bin/symlink", StagingStatus: git.StatusUnmodified, WorkTreeStatus: git.StatusModified},
				},
			},
			expectedFiles: []string{"bin/symlink"},
		},
		{
			name: "Staged metadata files should be ignored",
			state: &git.RepositoryState{
				CommitHash: "c1",
				StagedFiles: []git.FileStatus{
					{Path: ".wake/state.db", StagingStatus: git.StatusModified},
					{Path: ".sentinel/store.sqlite", StagingStatus: git.StatusAdded},
					{Path: ".git/HEAD", StagingStatus: git.StatusModified},
				},
			},
			expectedFiles: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePreCheckpoint(ctx, tc.state, CheckpointGuardOptions{})
			if len(tc.expectedFiles) > 0 {
				if err == nil {
					t.Fatalf("expected guard violation, got nil")
				}
				v, ok := err.(*GuardViolation)
				if !ok {
					t.Fatalf("expected *GuardViolation, got %T", err)
				}
				for _, exp := range tc.expectedFiles {
					if !containsString(v.ModifiedFiles, exp) {
						t.Errorf("expected modified files to contain %q, got: %v", exp, v.ModifiedFiles)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("expected no violation, got: %v", err)
				}
			}
		})
	}
}

// TestAdversarial_MergeConflictCombinations tests merge conflicts in various formats.
func TestAdversarial_MergeConflictCombinations(t *testing.T) {
	ctx := context.Background()

	// Scenario 1: HasMergeConflicts flag true
	s1 := &git.RepositoryState{
		CommitHash:        "c1",
		HasMergeConflicts: true,
	}
	err1 := ValidatePreCheckpoint(ctx, s1, CheckpointGuardOptions{})
	if err1 == nil {
		t.Fatalf("expected violation for HasMergeConflicts=true")
	}

	// Scenario 2: UnmergedFiles non-empty
	s2 := &git.RepositoryState{
		CommitHash:    "c1",
		UnmergedFiles: []string{"pkg/conflict.go"},
	}
	err2 := ValidatePreCheckpoint(ctx, s2, CheckpointGuardOptions{})
	if err2 == nil {
		t.Fatalf("expected violation for UnmergedFiles")
	}
	v2 := err2.(*GuardViolation)
	if !v2.HasMergeConflicts {
		t.Errorf("expected HasMergeConflicts=true in violation")
	}

	// Scenario 3: Merge conflict with TrackedFiles supplied - MUST still fail!
	s3 := &git.RepositoryState{
		CommitHash:        "c1",
		HasMergeConflicts: true,
		ModifiedFiles:     []string{"pkg/conflict.go"},
	}
	err3 := ValidatePreCheckpoint(ctx, s3, CheckpointGuardOptions{
		TrackedFiles: []string{"pkg/conflict.go"},
	})
	if err3 == nil {
		t.Fatalf("expected violation even if conflicting file is in TrackedFiles")
	}

	// Scenario 4: Merge conflict with Force=true - MUST succeed!
	err4 := ValidatePreCheckpoint(ctx, s3, CheckpointGuardOptions{
		Force:        true,
		TrackedFiles: []string{"pkg/conflict.go"},
	})
	if err4 != nil {
		t.Fatalf("expected Force=true to override merge conflict, got: %v", err4)
	}
}

// TestAdversarial_TrackedFilesPatternMatrix tests edge cases in pattern matching for TrackedFiles.
func TestAdversarial_TrackedFilesPatternMatrix(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name         string
		dirtyFiles   []string
		trackedFiles []string
		expectPass   bool
	}{
		{
			name:         "Exact match",
			dirtyFiles:   []string{"internal/guard/guard.go"},
			trackedFiles: []string{"internal/guard/guard.go"},
			expectPass:   true,
		},
		{
			name:         "Case insensitive match",
			dirtyFiles:   []string{"INTERNAL/GUARD/GUARD.GO"},
			trackedFiles: []string{"internal/guard/guard.go"},
			expectPass:   true,
		},
		{
			name:         "Directory prefix match without trailing slash",
			dirtyFiles:   []string{"internal/guard/guard.go", "internal/guard/guard_test.go"},
			trackedFiles: []string{"internal/guard"},
			expectPass:   true,
		},
		{
			name:         "Directory prefix match with trailing slash",
			dirtyFiles:   []string{"internal/guard/guard.go"},
			trackedFiles: []string{"internal/guard/"},
			expectPass:   true,
		},
		{
			name:         "Directory glob wildcard",
			dirtyFiles:   []string{"internal/guard/guard.go"},
			trackedFiles: []string{"internal/guard/*"},
			expectPass:   true,
		},
		{
			name:         "Deep directory recursive wildcard",
			dirtyFiles:   []string{"a/b/c/d/e.go"},
			trackedFiles: []string{"a/b/**"},
			expectPass:   true,
		},
		{
			name:         "Windows backslash in tracked pattern",
			dirtyFiles:   []string{"internal/guard/guard.go"},
			trackedFiles: []string{`internal\guard\guard.go`},
			expectPass:   true,
		},
		{
			name:         "Quoted pattern in tracked files",
			dirtyFiles:   []string{"cmd/root.go"},
			trackedFiles: []string{`"cmd/root.go"`},
			expectPass:   true,
		},
		{
			name:         "Extension wildcard",
			dirtyFiles:   []string{"main.go"},
			trackedFiles: []string{"*.go"},
			expectPass:   true,
		},
		{
			name:         "Multiple tracked patterns covering all dirty files",
			dirtyFiles:   []string{"cmd/root.go", "pkg/auth/login.go", "docs/README.md"},
			trackedFiles: []string{"cmd/*", "pkg/auth/login.go", "docs/README.md"},
			expectPass:   true,
		},
		{
			name:         "Partial coverage - one file uncovered",
			dirtyFiles:   []string{"cmd/root.go", "pkg/auth/login.go", "secret/leak.txt"},
			trackedFiles: []string{"cmd/*", "pkg/auth/*"},
			expectPass:   false,
		},
		{
			name:         "Trailing dots or clean paths",
			dirtyFiles:   []string{"pkg/auth/login.go"},
			trackedFiles: []string{"./pkg/auth/login.go"},
			expectPass:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repoState := &git.RepositoryState{
				CommitHash:    "c1",
				ModifiedFiles: tc.dirtyFiles,
			}

			err := ValidatePreCheckpoint(ctx, repoState, CheckpointGuardOptions{
				TrackedFiles: tc.trackedFiles,
			})

			if tc.expectPass {
				if err != nil {
					t.Fatalf("expected pass for tracked files %v on dirty %v, got: %v", tc.trackedFiles, tc.dirtyFiles, err)
				}
			} else {
				if err == nil {
					t.Fatalf("expected violation for tracked files %v on dirty %v, got nil", tc.trackedFiles, tc.dirtyFiles)
				}
				v, ok := err.(*GuardViolation)
				if !ok {
					t.Fatalf("expected *GuardViolation, got %T", err)
				}
				if len(v.UnreviewedFiles) == 0 {
					t.Errorf("expected UnreviewedFiles in violation, got none")
				}
			}
		})
	}
}

// TestAdversarial_ForceOverrideExhaustive verifies --force overrides every single possible guard condition.
func TestAdversarial_ForceOverrideExhaustive(t *testing.T) {
	ctx := context.Background()

	dirtyRepo := &git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "feature/stress",
		CommitHash:        "deadbeef",
		IsClean:           false,
		HasMergeConflicts: true,
		UntrackedFiles:    []string{"untracked1.txt", "untracked2.go", ".env"},
		ModifiedFiles:     []string{"mod1.go", "mod2.ts"},
		UnmergedFiles:     []string{"conflict.go"},
		StagedFiles: []git.FileStatus{
			{Path: "staged.go", StagingStatus: git.StatusAdded},
		},
		UnstagedFiles: []git.FileStatus{
			{Path: "unstaged.go", WorkTreeStatus: git.StatusModified},
		},
	}

	err := ValidatePreCheckpoint(ctx, dirtyRepo, CheckpointGuardOptions{
		Force: true,
	})

	if err != nil {
		t.Fatalf("expected Force=true to cleanly override all dirty conditions, got: %v", err)
	}
}

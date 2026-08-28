package git

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

// TestAdversarial_EmptyRepoStates verifies behavior with 0-commit repositories under various working tree conditions.
func TestAdversarial_EmptyRepoStates(t *testing.T) {
	ctx := context.Background()

	t.Run("Clean empty repository with 0 commits", func(t *testing.T) {
		mock := NewMockRunner()
		mock.Register("rev-parse --show-toplevel", "C:/repos/empty\n", "", nil)
		mock.Register("rev-parse HEAD", "", "fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.\n",
			classifyGitError("git", []string{"rev-parse", "HEAD"}, 128, "fatal: ambiguous argument 'HEAD'", errors.New("exit 128")))
		mock.Register("branch --show-current", "main\n", "", nil)
		mock.Register("status --porcelain=v1 -uall", "", "", nil)

		c := NewClient(mock)
		state, err := c.GetState(ctx, "C:/repos/empty")
		if err != nil {
			t.Fatalf("GetState failed on clean empty repo: %v", err)
		}

		if state.HasCommits {
			t.Errorf("expected HasCommits=false, got true")
		}
		if state.CommitHash != "" {
			t.Errorf("expected empty CommitHash, got %q", state.CommitHash)
		}
		if !state.IsClean {
			t.Errorf("expected IsClean=true, got false")
		}
		if state.IsDetached {
			t.Errorf("expected IsDetached=false on main branch, got true")
		}
		if state.Branch != "main" {
			t.Errorf("expected Branch='main', got %q", state.Branch)
		}
		if len(state.ModifiedFiles) != 0 {
			t.Errorf("expected 0 modified files, got %v", state.ModifiedFiles)
		}
	})

	t.Run("Empty repository with untracked and staged files before initial commit", func(t *testing.T) {
		mock := NewMockRunner()
		mock.Register("rev-parse --show-toplevel", "C:/repos/empty_staged\n", "", nil)
		mock.Register("rev-parse HEAD", "", "fatal: your current branch 'master' does not have any commits yet\n",
			classifyGitError("git", []string{"rev-parse", "HEAD"}, 128, "does not have any commits yet", errors.New("exit 128")))
		mock.Register("branch --show-current", "master\n", "", nil)
		mock.Register("status --porcelain=v1 -uall", "A  README.md\n?? scratch.txt\n", "", nil)

		c := NewClient(mock)
		state, err := c.GetState(ctx, "C:/repos/empty_staged")
		if err != nil {
			t.Fatalf("GetState failed on empty repo with staged files: %v", err)
		}

		if state.HasCommits {
			t.Errorf("expected HasCommits=false, got true")
		}
		if state.IsClean {
			t.Errorf("expected IsClean=false, got true")
		}
		if len(state.StagedFiles) != 1 || state.StagedFiles[0].Path != "README.md" {
			t.Errorf("expected 1 staged file README.md, got %+v", state.StagedFiles)
		}
		if len(state.UntrackedFiles) != 1 || state.UntrackedFiles[0] != "scratch.txt" {
			t.Errorf("expected 1 untracked file scratch.txt, got %+v", state.UntrackedFiles)
		}
		expectedModified := []string{"README.md", "scratch.txt"}
		if !reflect.DeepEqual(state.ModifiedFiles, expectedModified) {
			t.Errorf("expected modified files %v, got %v", expectedModified, state.ModifiedFiles)
		}
	})
}

// TestAdversarial_DetachedHeadMatrix tests various detached HEAD variations.
func TestAdversarial_DetachedHeadMatrix(t *testing.T) {
	ctx := context.Background()

	t.Run("Detached HEAD via commit checkout", func(t *testing.T) {
		mock := NewMockRunner()
		mock.Register("rev-parse --show-toplevel", "C:/repos/detached\n", "", nil)
		mock.Register("rev-parse HEAD", "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b\n", "", nil)
		// git branch --show-current returns empty string when detached
		mock.Register("branch --show-current", "", "", nil)
		mock.Register("rev-parse --abbrev-ref HEAD", "HEAD\n", "", nil)
		mock.Register("status --porcelain=v1 -uall", "", "", nil)

		c := NewClient(mock)
		state, err := c.GetState(ctx, "C:/repos/detached")
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}

		if !state.IsDetached {
			t.Errorf("expected IsDetached=true, got false")
		}
		if state.Branch != "HEAD" {
			t.Errorf("expected Branch='HEAD', got %q", state.Branch)
		}
		if state.CommitHash != "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b" {
			t.Errorf("expected commit hash match, got %q", state.CommitHash)
		}
	})

	t.Run("Detached HEAD fallback when abbrev-ref fails", func(t *testing.T) {
		mock := NewMockRunner()
		mock.Register("rev-parse --show-toplevel", "C:/repos/detached\n", "", nil)
		mock.Register("rev-parse HEAD", "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b\n", "", nil)
		mock.Register("branch --show-current", "", "", nil)
		mock.Register("rev-parse --abbrev-ref HEAD", "", "", errors.New("abbrev-ref error"))
		mock.Register("symbolic-ref --short HEAD", "", "fatal: ref HEAD is not a symbolic ref", errors.New("not symbolic"))
		mock.Register("status --porcelain=v1 -uall", "", "", nil)

		c := NewClient(mock)
		state, err := c.GetState(ctx, "C:/repos/detached")
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}

		if !state.IsDetached {
			t.Errorf("expected IsDetached=true, got false")
		}
		if state.Branch != "HEAD" {
			t.Errorf("expected Branch='HEAD', got %q", state.Branch)
		}
	})
}

// TestAdversarial_FilenamesWithSpacesAndUnicode tests complex paths, whitespace, quotes, and international characters.
func TestAdversarial_FilenamesWithSpacesAndUnicode(t *testing.T) {
	porcelainOutput := "M  \"path with spaces/my file.txt\"\n" +
		"A  \"deeply/nested/path with spaces/another file.go\"\n" +
		"?? \"unicode_日本語_test.txt\"\n" +
		"?? \"unicode_üñîçødé_файл.md\"\n" +
		" D normal_deleted.txt\n" +
		"R  \"old name with space.txt\" -> \"new name with space.txt\"\n"

	status := ParsePorcelainStatus(porcelainOutput)

	if status.IsClean {
		t.Errorf("expected IsClean=false")
	}

	// Verify staged files
	if len(status.StagedFiles) != 3 {
		t.Fatalf("expected 3 staged files, got %d: %+v", len(status.StagedFiles), status.StagedFiles)
	}

	expectedStaged := []struct {
		path     string
		origPath string
		status   StatusCode
	}{
		{"path with spaces/my file.txt", "", StatusModified},
		{"deeply/nested/path with spaces/another file.go", "", StatusAdded},
		{"new name with space.txt", "old name with space.txt", StatusRenamed},
	}

	for i, exp := range expectedStaged {
		actual := status.StagedFiles[i]
		if actual.Path != exp.path {
			t.Errorf("staged[%d] path: expected %q, got %q", i, exp.path, actual.Path)
		}
		if actual.OrigPath != exp.origPath {
			t.Errorf("staged[%d] origPath: expected %q, got %q", i, exp.origPath, actual.OrigPath)
		}
		if actual.StagingStatus != exp.status {
			t.Errorf("staged[%d] status: expected %v, got %v", i, exp.status, actual.StagingStatus)
		}
	}

	// Verify unstaged files
	if len(status.UnstagedFiles) != 1 || status.UnstagedFiles[0].Path != "normal_deleted.txt" {
		t.Errorf("unstaged files mismatch: %+v", status.UnstagedFiles)
	}

	// Verify untracked files
	expectedUntracked := []string{
		"unicode_日本語_test.txt",
		"unicode_üñîçødé_файл.md",
	}
	if !reflect.DeepEqual(status.UntrackedFiles, expectedUntracked) {
		t.Errorf("untracked files mismatch: expected %v, got %v", expectedUntracked, status.UntrackedFiles)
	}

	// Verify ExtractModifiedFiles
	modified := ExtractModifiedFiles(status)
	expectedModified := []string{
		"deeply/nested/path with spaces/another file.go",
		"new name with space.txt",
		"normal_deleted.txt",
		"old name with space.txt",
		"path with spaces/my file.txt",
		"unicode_日本語_test.txt",
		"unicode_üñîçødé_файл.md",
	}
	if !reflect.DeepEqual(modified, expectedModified) {
		t.Errorf("ExtractModifiedFiles mismatch:\nexpected: %v\ngot:      %v", expectedModified, modified)
	}
}

// TestAdversarial_FullConflictMatrix tests all 7 git unmerged states.
func TestAdversarial_FullConflictMatrix(t *testing.T) {
	porcelainOutput := "UU conflict_both_modified.txt\n" +
		"AA conflict_both_added.txt\n" +
		"DD conflict_both_deleted.txt\n" +
		"AU conflict_added_by_us.txt\n" +
		"UD conflict_deleted_by_them.txt\n" +
		"UA conflict_added_by_them.txt\n" +
		"DU conflict_deleted_by_us.txt\n"

	status := ParsePorcelainStatus(porcelainOutput)

	if status.IsClean {
		t.Errorf("expected IsClean=false on merge conflicts")
	}

	expectedConflicts := []string{
		"conflict_both_modified.txt",
		"conflict_both_added.txt",
		"conflict_both_deleted.txt",
		"conflict_added_by_us.txt",
		"conflict_deleted_by_them.txt",
		"conflict_added_by_them.txt",
		"conflict_deleted_by_us.txt",
	}

	if !reflect.DeepEqual(status.UnmergedFiles, expectedConflicts) {
		t.Errorf("unmerged files mismatch:\nexpected: %v\ngot:      %v", expectedConflicts, status.UnmergedFiles)
	}

	if len(status.StagedFiles) != 0 {
		t.Errorf("unmerged entries should not be in StagedFiles: %+v", status.StagedFiles)
	}
	if len(status.UnstagedFiles) != 0 {
		t.Errorf("unmerged entries should not be in UnstagedFiles: %+v", status.UnstagedFiles)
	}
	if len(status.UntrackedFiles) != 0 {
		t.Errorf("unmerged entries should not be in UntrackedFiles: %+v", status.UntrackedFiles)
	}

	// Modified files must include all 7 conflicted files
	modified := ExtractModifiedFiles(status)
	if len(modified) != 7 {
		t.Errorf("expected 7 modified files from conflicts, got %d: %v", len(modified), modified)
	}
}

// TestAdversarial_DualStagedUnstagedCombinations tests two-letter status combinations.
func TestAdversarial_DualStagedUnstagedCombinations(t *testing.T) {
	// MM: staged mod + unstaged mod
	// AM: staged add + unstaged mod
	// AD: staged add + unstaged del
	// MD: staged mod + unstaged del
	// RM: staged rename + unstaged mod
	// RD: staged rename + unstaged del
	porcelainOutput := "MM staged_and_worktree_mod.go\n" +
		"AM staged_add_worktree_mod.go\n" +
		"AD staged_add_worktree_del.go\n" +
		"MD staged_mod_worktree_del.go\n" +
		"RM old_renamed.go -> new_renamed.go\n"

	status := ParsePorcelainStatus(porcelainOutput)

	if len(status.StagedFiles) != 5 {
		t.Fatalf("expected 5 staged files, got %d: %+v", len(status.StagedFiles), status.StagedFiles)
	}
	if len(status.UnstagedFiles) != 5 {
		t.Fatalf("expected 5 unstaged files, got %d: %+v", len(status.UnstagedFiles), status.UnstagedFiles)
	}

	// Verify RM rename handling
	stagedRename := status.StagedFiles[4]
	if stagedRename.Path != "new_renamed.go" || stagedRename.OrigPath != "old_renamed.go" || stagedRename.StagingStatus != StatusRenamed || stagedRename.WorkTreeStatus != StatusModified {
		t.Errorf("staged rename mismatch: %+v", stagedRename)
	}

	unstagedRenameMod := status.UnstagedFiles[4]
	if unstagedRenameMod.Path != "new_renamed.go" || unstagedRenameMod.OrigPath != "old_renamed.go" || unstagedRenameMod.WorkTreeStatus != StatusModified {
		t.Errorf("unstaged rename mod mismatch: %+v", unstagedRenameMod)
	}
}

// TestAdversarial_CommitValidationAndAncestry tests invalid hashes, edge cases, and ancestry logic.
func TestAdversarial_CommitValidationAndAncestry(t *testing.T) {
	ctx := context.Background()
	mock := NewMockRunner()
	repoDir := "C:/repos/test_repo"

	// Mock valid and invalid commits
	validCommit := "abcdef0123456789abcdef0123456789abcdef01"
	invalidCommit := "ffffffffffffffffffffffffffffffffffffffff"
	notACommit := "blob_hash_123456"

	mock.Register(fmt.Sprintf("cat-file -e %s^{commit}", validCommit), "", "", nil)
	mock.Register(fmt.Sprintf("cat-file -e %s^{commit}", invalidCommit), "", "fatal: Not a valid object name",
		classifyGitError("git", []string{"cat-file", "-e"}, 128, "fatal: Not a valid object name", errors.New("exit 128")))
	mock.Register(fmt.Sprintf("cat-file -e %s^{commit}", notACommit), "", "fatal: Not a valid commit name",
		classifyGitError("git", []string{"cat-file", "-e"}, 128, "fatal: Not a valid commit name", errors.New("exit 128")))

	// Ancestry mocks
	c1 := "c111111111111111111111111111111111111111"
	c2 := "c222222222222222222222222222222222222222"
	c3 := "c333333333333333333333333333333333333333"

	mock.Register(fmt.Sprintf("merge-base --is-ancestor %s %s", c1, c2), "", "", nil)
	mock.Register(fmt.Sprintf("merge-base --is-ancestor %s %s", c2, c3), "", "", nil)
	mock.Register(fmt.Sprintf("merge-base --is-ancestor %s %s", c1, c3), "", "", nil)
	mock.Register(fmt.Sprintf("merge-base --is-ancestor %s %s", c3, c1), "", "",
		&GitError{ExitCode: 1, Command: "git", Args: []string{"merge-base", "--is-ancestor", c3, c1}})
	mock.Register(fmt.Sprintf("merge-base --is-ancestor %s %s", c1, invalidCommit), "", "fatal: Not a valid object name",
		&GitError{ExitCode: 128, Command: "git", Args: []string{"merge-base", "--is-ancestor", c1, invalidCommit}, Stderr: "fatal: Not a valid object name"})

	c := NewClient(mock)

	// CommitExists checks
	exists, err := c.CommitExists(ctx, repoDir, validCommit)
	if err != nil || !exists {
		t.Errorf("expected valid commit exists=true, got %v, err: %v", exists, err)
	}

	exists, err = c.CommitExists(ctx, repoDir, invalidCommit)
	if err != nil || exists {
		t.Errorf("expected invalid commit exists=false, got %v, err: %v", exists, err)
	}

	exists, err = c.CommitExists(ctx, repoDir, notACommit)
	if err != nil || exists {
		t.Errorf("expected non-commit exists=false, got %v, err: %v", exists, err)
	}

	// Empty string inputs
	exists, err = c.CommitExists(ctx, repoDir, "")
	if err != nil || exists {
		t.Errorf("expected empty string commit exists=false, got %v, err: %v", exists, err)
	}

	exists, err = c.CommitExists(ctx, repoDir, "   \t\n")
	if err != nil || exists {
		t.Errorf("expected whitespace commit exists=false, got %v, err: %v", exists, err)
	}

	// Ancestry checks
	// Reflexive ancestry (same commit is ancestor of itself)
	isAnc, err := c.IsAncestor(ctx, repoDir, c1, c1)
	if err != nil || !isAnc {
		t.Errorf("expected reflexive IsAncestor(c1, c1)=true, got %v, err: %v", isAnc, err)
	}

	// Direct ancestry
	isAnc, err = c.IsAncestor(ctx, repoDir, c1, c2)
	if err != nil || !isAnc {
		t.Errorf("expected IsAncestor(c1, c2)=true, got %v, err: %v", isAnc, err)
	}

	// Transitive ancestry
	isAnc, err = c.IsAncestor(ctx, repoDir, c1, c3)
	if err != nil || !isAnc {
		t.Errorf("expected IsAncestor(c1, c3)=true, got %v, err: %v", isAnc, err)
	}

	// Reverse ancestry (c3 is child of c1, not ancestor)
	isAnc, err = c.IsAncestor(ctx, repoDir, c3, c1)
	if err != nil || isAnc {
		t.Errorf("expected IsAncestor(c3, c1)=false, got %v, err: %v", isAnc, err)
	}

	// Ancestry with invalid commit -> should return error (exit 128)
	_, err = c.IsAncestor(ctx, repoDir, c1, invalidCommit)
	if err == nil {
		t.Errorf("expected error when checking ancestry with invalid commit, got nil")
	}

	// Empty inputs
	isAnc, err = c.IsAncestor(ctx, repoDir, "", c1)
	if err != nil || isAnc {
		t.Errorf("expected empty ancestor to return false, got %v, err: %v", isAnc, err)
	}
	isAnc, err = c.IsAncestor(ctx, repoDir, c1, "")
	if err != nil || isAnc {
		t.Errorf("expected empty descendant to return false, got %v, err: %v", isAnc, err)
	}
}

// TestAdversarial_ErrorClassificationEdgeCases verifies proper classification of all domain error sentinels.
func TestAdversarial_ErrorClassificationEdgeCases(t *testing.T) {
	testCases := []struct {
		name        string
		stderr      string
		exitCode    int
		expectedErr error
	}{
		{
			name:        "Not a git repo",
			stderr:      "fatal: not a git repository (or any of the parent directories): .git",
			exitCode:    128,
			expectedErr: ErrNotGitRepo,
		},
		{
			name:        "No commits yet - ambiguous head",
			stderr:      "fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.",
			exitCode:    128,
			expectedErr: ErrNoCommits,
		},
		{
			name:        "No commits yet - explicit branch message",
			stderr:      "fatal: your current branch 'main' does not have any commits yet",
			exitCode:    128,
			expectedErr: ErrNoCommits,
		},
		{
			name:        "Invalid commit - bad object",
			stderr:      "fatal: bad object deadbeef1234",
			exitCode:    128,
			expectedErr: ErrInvalidCommit,
		},
		{
			name:        "Invalid commit - not a valid object name",
			stderr:      "fatal: Not a valid object name: foo",
			exitCode:    128,
			expectedErr: ErrInvalidCommit,
		},
		{
			name:        "Index lock exists",
			stderr:      "fatal: Unable to create 'C:/repo/.git/index.lock': File exists.",
			exitCode:    128,
			expectedErr: ErrGitLockExists,
		},
		{
			name:        "Dubious ownership detected",
			stderr:      "fatal: detected dubious ownership in repository at 'C:/repo'",
			exitCode:    128,
			expectedErr: ErrDubiousOwnership,
		},
		{
			name:        "Merge conflict active",
			stderr:      "error: you need to resolve your current index first (unmerged paths)",
			exitCode:    128,
			expectedErr: ErrMergeConflict,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyGitError("git", []string{"cmd"}, tc.exitCode, tc.stderr, errors.New("command failed"))
			if !errors.Is(err, tc.expectedErr) {
				t.Errorf("expected error to wrap %v, got %v", tc.expectedErr, err)
			}
		})
	}
}

// TestAdversarial_ConcurrentClientUsage tests thread-safety of MockRunner and Client operations under high concurrency.
func TestAdversarial_ConcurrentClientUsage(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/repos/concurrent_repo"

	mock.Register("rev-parse --show-toplevel", "C:/repos/concurrent_repo\n", "", nil)
	mock.Register("rev-parse HEAD", "abcdef1234567890abcdef1234567890abcdef12\n", "", nil)
	mock.Register("branch --show-current", "main\n", "", nil)
	mock.Register("status --porcelain=v1 -uall", "M  file1.go\n?? file2.go\n", "", nil)
	mock.Register("cat-file -e abcdef1234567890abcdef1234567890abcdef12^{commit}", "", "", nil)

	c := NewClient(mock)

	var wg sync.WaitGroup
	numGoroutines := 50

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Call GetState
			state, err := c.GetState(ctx, repoDir)
			if err != nil {
				t.Errorf("goroutine %d GetState failed: %v", idx, err)
				return
			}
			if state.Branch != "main" {
				t.Errorf("goroutine %d expected branch main, got %s", idx, state.Branch)
			}

			// Call CommitExists
			exists, err := c.CommitExists(ctx, repoDir, "abcdef1234567890abcdef1234567890abcdef12")
			if err != nil || !exists {
				t.Errorf("goroutine %d CommitExists failed: %v, exists: %v", idx, err, exists)
			}

			// Call IsClean
			clean, err := c.IsClean(ctx, repoDir)
			if err != nil || clean {
				t.Errorf("goroutine %d IsClean failed or returned true: %v, clean: %v", idx, err, clean)
			}
		}(i)
	}

	wg.Wait()
}

// TestAdversarial_ParserEdgeCases tests malformed or boundary condition lines in status output.
func TestAdversarial_ParserEdgeCases(t *testing.T) {
	t.Run("Empty and whitespace only lines", func(t *testing.T) {
		output := "\n\r\n   \n\t\n"
		status := ParsePorcelainStatus(output)
		if !status.IsClean {
			t.Errorf("expected IsClean=true for whitespace-only status, got false")
		}
	})

	t.Run("Short lines (< 3 characters)", func(t *testing.T) {
		output := "M\n??\n A\n"
		status := ParsePorcelainStatus(output)
		if !status.IsClean {
			t.Errorf("expected IsClean=true for truncated lines, got false")
		}
	})

	t.Run("Ignored files", func(t *testing.T) {
		output := "!! node_modules/\n!! .env.local\n"
		status := ParsePorcelainStatus(output)
		if !status.IsClean {
			t.Errorf("expected ignored files to not make repo dirty, got IsClean=false")
		}
		if len(status.UntrackedFiles) != 0 || len(status.StagedFiles) != 0 || len(status.UnstagedFiles) != 0 {
			t.Errorf("ignored files should not be listed in tracked/untracked slices")
		}
	})

	t.Run("ParseDiffNameStatus variations", func(t *testing.T) {
		output := "M\tmodified.go\n" +
			"A\tadded.go\n" +
			"D\tdeleted.go\n" +
			"R100\told_renamed.go\tnew_renamed.go\n" +
			"C080\tsrc_copied.go\tdst_copied.go\n" +
			"invalid line without tab\n" +
			"\n"

		changes := ParseDiffNameStatus(output)
		if len(changes) != 5 {
			t.Fatalf("expected 5 parsed changes, got %d: %+v", len(changes), changes)
		}

		// Check rename
		if changes[3].Status != StatusRenamed || changes[3].Path != "new_renamed.go" || changes[3].OrigPath != "old_renamed.go" {
			t.Errorf("rename mismatch: %+v", changes[3])
		}
		// Check copy
		if changes[4].Status != StatusCopied || changes[4].Path != "dst_copied.go" || changes[4].OrigPath != "src_copied.go" {
			t.Errorf("copy mismatch: %+v", changes[4])
		}
	})
}

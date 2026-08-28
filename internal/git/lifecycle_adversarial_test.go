package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runGitCommand is a test helper to execute git in a given directory and fail the test on error.
func runGitCommand(t *testing.T, ctx context.Context, dir string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed in %s: %v (output: %s)", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// runGitCommandAllowError executes git allowing non-zero exit codes (e.g. merge conflicts).
func runGitCommandAllowError(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// TestIntegration_AdversarialLifecycle tests end-to-end Git client behaviors on a real Git repository.
func TestIntegration_AdversarialLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available in PATH, skipping live repository integration tests")
	}

	tempDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Initialize repo
	runGitCommand(t, ctx, tempDir, "init")
	runGitCommand(t, ctx, tempDir, "config", "user.name", "Adversarial Tester")
	runGitCommand(t, ctx, tempDir, "config", "user.email", "tester@sentinel.dev")
	runGitCommand(t, ctx, tempDir, "config", "commit.gpgsign", "false")
	// Disable quotepath to test standard UTF8 output handling
	runGitCommand(t, ctx, tempDir, "config", "core.quotepath", "false")

	c := NewClient(NewOSRunner())

	// 1. Fresh Empty Repo Probe
	t.Run("Empty repository probes", func(t *testing.T) {
		state, err := c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState on empty repo failed: %v", err)
		}
		if state.HasCommits {
			t.Errorf("expected HasCommits=false, got true")
		}
		if state.CommitHash != "" {
			t.Errorf("expected CommitHash='', got %q", state.CommitHash)
		}
		if !state.IsClean {
			t.Errorf("expected IsClean=true on clean empty repo, got false")
		}
		if state.HasMergeConflicts {
			t.Errorf("expected HasMergeConflicts=false on empty repo, got true")
		}

		_, err = c.GetCurrentCommit(ctx, tempDir)
		if !errors.Is(err, ErrNoCommits) {
			t.Errorf("expected ErrNoCommits on empty repo, got: %v", err)
		}

		// Untracked file in empty repo
		untrackedFile := filepath.Join(tempDir, "untracked.txt")
		if err := os.WriteFile(untrackedFile, []byte("untracked content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		state, err = c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if state.IsClean {
			t.Errorf("expected IsClean=false after adding untracked file")
		}
		if len(state.UntrackedFiles) != 1 || state.UntrackedFiles[0] != "untracked.txt" {
			t.Errorf("expected UntrackedFiles=['untracked.txt'], got: %v", state.UntrackedFiles)
		}

		// Stage file in empty repo (0 commits)
		runGitCommand(t, ctx, tempDir, "add", "untracked.txt")
		state, err = c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if state.HasCommits {
			t.Errorf("expected HasCommits=false before first commit")
		}
		if len(state.StagedFiles) != 1 || state.StagedFiles[0].Path != "untracked.txt" {
			t.Errorf("expected StagedFiles=['untracked.txt'], got: %+v", state.StagedFiles)
		}

		// Reset stage for next tests
		runGitCommand(t, ctx, tempDir, "rm", "-f", "untracked.txt")
	})

	// 2. Spaces, Unicode, and Renames Probe
	t.Run("Spaces, Unicode filenames and renames", func(t *testing.T) {
		// Create files with spaces and unicode
		spacedDir := filepath.Join(tempDir, "spaced folder")
		if err := os.MkdirAll(spacedDir, 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}

		file1 := filepath.Join(spacedDir, "my document.txt")
		file2 := filepath.Join(tempDir, "unicode_file.txt")

		if err := os.WriteFile(file1, []byte("document text\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		if err := os.WriteFile(file2, []byte("unicode content\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		runGitCommand(t, ctx, tempDir, "add", "-A")
		runGitCommand(t, ctx, tempDir, "commit", "-m", "Commit with spaces and unicode")

		state, err := c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		if !state.HasCommits {
			t.Errorf("expected HasCommits=true")
		}
		if !state.IsClean {
			t.Errorf("expected IsClean=true after initial commit")
		}
		baseCommit := state.CommitHash

		// Rename file using git mv
		runGitCommand(t, ctx, tempDir, "mv", "unicode_file.txt", "renamed_unicode_file.txt")

		status, err := c.GetStatus(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.IsClean {
			t.Errorf("expected IsClean=false after git mv")
		}
		if len(status.StagedFiles) != 1 {
			t.Fatalf("expected 1 staged rename, got %d: %+v", len(status.StagedFiles), status.StagedFiles)
		}
		renamedFile := status.StagedFiles[0]
		if renamedFile.StagingStatus != StatusRenamed {
			t.Errorf("expected StagingStatus='R', got '%s'", renamedFile.StagingStatus)
		}
		if renamedFile.Path != "renamed_unicode_file.txt" {
			t.Errorf("expected Path='renamed_unicode_file.txt', got '%s'", renamedFile.Path)
		}
		if renamedFile.OrigPath != "unicode_file.txt" {
			t.Errorf("expected OrigPath='unicode_file.txt', got '%s'", renamedFile.OrigPath)
		}

		// Also modify the renamed file in worktree (RM status)
		renamedPath := filepath.Join(tempDir, "renamed_unicode_file.txt")
		if err := os.WriteFile(renamedPath, []byte("modified after rename\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}

		status, err = c.GetStatus(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if len(status.StagedFiles) != 1 || len(status.UnstagedFiles) != 1 {
			t.Fatalf("expected 1 staged and 1 unstaged for RM status, got staged=%d, unstaged=%d", len(status.StagedFiles), len(status.UnstagedFiles))
		}

		// Commit the rename and modification
		runGitCommand(t, ctx, tempDir, "add", "-A")
		runGitCommand(t, ctx, tempDir, "commit", "-m", "Commit rename")

		state, err = c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState failed: %v", err)
		}
		commit2 := state.CommitHash

		// Verify GetChangedFilesBetween
		changed, err := c.GetChangedFilesBetween(ctx, tempDir, baseCommit, commit2)
		if err != nil {
			t.Fatalf("GetChangedFilesBetween failed: %v", err)
		}
		if len(changed) == 0 {
			t.Errorf("expected changed files between commits, got none")
		}
	})

	// 3. Unstaged Deletions Probe
	t.Run("Unstaged deletions probe", func(t *testing.T) {
		tempFilePath := filepath.Join(tempDir, "to_be_deleted.txt")
		if err := os.WriteFile(tempFilePath, []byte("delete me\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		runGitCommand(t, ctx, tempDir, "add", "to_be_deleted.txt")
		runGitCommand(t, ctx, tempDir, "commit", "-m", "Add file to delete")

		// Physical file deletion in filesystem without git rm
		if err := os.Remove(tempFilePath); err != nil {
			t.Fatalf("Remove failed: %v", err)
		}

		status, err := c.GetStatus(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetStatus failed: %v", err)
		}
		if status.IsClean {
			t.Errorf("expected IsClean=false with unstaged deletion")
		}
		if len(status.UnstagedFiles) != 1 {
			t.Fatalf("expected 1 unstaged deletion, got %d: %+v", len(status.UnstagedFiles), status.UnstagedFiles)
		}
		if status.UnstagedFiles[0].WorkTreeStatus != StatusDeleted {
			t.Errorf("expected WorkTreeStatus='D', got '%s'", status.UnstagedFiles[0].WorkTreeStatus)
		}
		if status.UnstagedFiles[0].Path != "to_be_deleted.txt" {
			t.Errorf("expected Path='to_be_deleted.txt', got '%s'", status.UnstagedFiles[0].Path)
		}

		// Stage the deletion
		runGitCommand(t, ctx, tempDir, "add", "-u")
		runGitCommand(t, ctx, tempDir, "commit", "-m", "Commit deletion")
	})

	// 4. Real Merge Conflict Probe (UU state)
	t.Run("Real merge conflict state", func(t *testing.T) {
		// Create a file on main branch
		conflictFilePath := filepath.Join(tempDir, "conflict.txt")
		if err := os.WriteFile(conflictFilePath, []byte("line 1: original\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		runGitCommand(t, ctx, tempDir, "add", "conflict.txt")
		runGitCommand(t, ctx, tempDir, "commit", "-m", "Base for conflict")
		mainBranch := runGitCommand(t, ctx, tempDir, "rev-parse", "--abbrev-ref", "HEAD")

		// Create conflict-branch
		runGitCommand(t, ctx, tempDir, "checkout", "-b", "conflict-branch")
		if err := os.WriteFile(conflictFilePath, []byte("line 1: branch edit\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		runGitCommand(t, ctx, tempDir, "commit", "-am", "Edit on conflict-branch")

		// Switch back to main and edit same line differently
		runGitCommand(t, ctx, tempDir, "checkout", mainBranch)
		if err := os.WriteFile(conflictFilePath, []byte("line 1: main edit\n"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
		runGitCommand(t, ctx, tempDir, "commit", "-am", "Edit on main branch")

		// Trigger merge conflict
		_, _ = runGitCommandAllowError(ctx, tempDir, "merge", "conflict-branch")

		state, err := c.GetState(ctx, tempDir)
		if err != nil {
			t.Fatalf("GetState on conflicted repo failed: %v", err)
		}

		if !state.HasMergeConflicts {
			t.Errorf("expected HasMergeConflicts=true on active merge conflict")
		}
		if state.IsClean {
			t.Errorf("expected IsClean=false during merge conflict")
		}
		if len(state.UnmergedFiles) != 1 || state.UnmergedFiles[0] != "conflict.txt" {
			t.Errorf("expected UnmergedFiles=['conflict.txt'], got: %v", state.UnmergedFiles)
		}

		// Abort merge to restore clean state
		_, _ = runGitCommandAllowError(ctx, tempDir, "merge", "--abort")
	})

	// 5. Ancestry and Non-Existent Commits Probe
	t.Run("Ancestry and invalid commit probe", func(t *testing.T) {
		headCommit := runGitCommand(t, ctx, tempDir, "rev-parse", "HEAD")

		// Valid commit existence
		exists, err := c.CommitExists(ctx, tempDir, headCommit)
		if err != nil || !exists {
			t.Errorf("expected headCommit exists=true, got %v, err: %v", exists, err)
		}

		// Invalid commit hash
		exists, err = c.CommitExists(ctx, tempDir, "0000000000000000000000000000000000000000")
		if err != nil || exists {
			t.Errorf("expected 0000... commit exists=false, got %v, err: %v", exists, err)
		}

		// Malicious input strings
		exists, err = c.CommitExists(ctx, tempDir, "--version")
		if err != nil || exists {
			t.Errorf("expected --version commit exists=false, got %v, err: %v", exists, err)
		}

		// Ancestry check with self
		isAnc, err := c.IsAncestor(ctx, tempDir, headCommit, headCommit)
		if err != nil || !isAnc {
			t.Errorf("expected headCommit is ancestor of itself, got %v, err: %v", isAnc, err)
		}
	})
}

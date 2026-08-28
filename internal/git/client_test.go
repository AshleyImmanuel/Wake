package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestClient_GetState_Normal(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/repo"

	mock.Register("rev-parse --show-toplevel", "C:/test/repo\n", "", nil)
	mock.Register("rev-parse HEAD", "abcdef1234567890abcdef1234567890abcdef12\n", "", nil)
	mock.Register("branch --show-current", "feature/reconcile\n", "", nil)
	mock.Register("status --porcelain=v1 -uall", "M  main.go\n?? new_file.txt\n", "", nil)

	c := NewClient(mock)
	state, err := c.GetState(ctx, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if state.RootPath != "C:/test/repo" {
		t.Errorf("expected RootPath 'C:/test/repo', got '%s'", state.RootPath)
	}
	if state.Branch != "feature/reconcile" {
		t.Errorf("expected Branch 'feature/reconcile', got '%s'", state.Branch)
	}
	if state.CommitHash != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Errorf("expected commit hash 'abcdef1234567890abcdef1234567890abcdef12', got '%s'", state.CommitHash)
	}
	if state.IsDetached {
		t.Errorf("expected IsDetached=false, got true")
	}
	if !state.HasCommits {
		t.Errorf("expected HasCommits=true, got false")
	}
	if state.IsClean {
		t.Errorf("expected IsClean=false, got true")
	}
	if state.HasMergeConflicts {
		t.Errorf("expected HasMergeConflicts=false, got true")
	}
	if len(state.StagedFiles) != 1 || state.StagedFiles[0].Path != "main.go" {
		t.Errorf("staged files mismatch: %+v", state.StagedFiles)
	}
	if len(state.UntrackedFiles) != 1 || state.UntrackedFiles[0] != "new_file.txt" {
		t.Errorf("untracked files mismatch: %+v", state.UntrackedFiles)
	}
	if len(state.ModifiedFiles) != 2 {
		t.Errorf("expected 2 modified files, got %d: %v", len(state.ModifiedFiles), state.ModifiedFiles)
	}
}

func TestClient_GetState_EmptyRepo(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/empty_repo"

	mock.Register("rev-parse --show-toplevel", "C:/test/empty_repo\n", "", nil)
	mock.Register("rev-parse HEAD", "", "fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.\n", classifyGitError("git", []string{"rev-parse", "HEAD"}, 128, "fatal: ambiguous argument 'HEAD'", errors.New("exit 128")))
	mock.Register("branch --show-current", "main\n", "", nil)
	mock.Register("status --porcelain=v1 -uall", "", "", nil)

	c := NewClient(mock)
	state, err := c.GetState(ctx, repoDir)
	if err != nil {
		t.Fatalf("GetState on empty repo should not fail, got: %v", err)
	}

	if state.HasCommits {
		t.Errorf("expected HasCommits=false on empty repo, got true")
	}
	if state.CommitHash != "" {
		t.Errorf("expected CommitHash='' on empty repo, got '%s'", state.CommitHash)
	}
	if !state.IsClean {
		t.Errorf("expected IsClean=true on clean empty repo, got false")
	}
	if state.Branch != "main" {
		t.Errorf("expected Branch 'main', got '%s'", state.Branch)
	}
}

func TestClient_GetState_DetachedHead(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/detached_repo"

	mock.Register("rev-parse --show-toplevel", "C:/test/detached_repo\n", "", nil)
	mock.Register("rev-parse HEAD", "1122334455667788990011223344556677889900\n", "", nil)
	mock.Register("branch --show-current", "", "", nil)
	mock.Register("rev-parse --abbrev-ref HEAD", "HEAD\n", "", nil)
	mock.Register("status --porcelain=v1 -uall", "", "", nil)

	c := NewClient(mock)
	state, err := c.GetState(ctx, repoDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !state.IsDetached {
		t.Errorf("expected IsDetached=true, got false")
	}
	if state.Branch != "HEAD" {
		t.Errorf("expected Branch 'HEAD', got '%s'", state.Branch)
	}
	if !state.HasCommits {
		t.Errorf("expected HasCommits=true, got false")
	}
	if !state.IsClean {
		t.Errorf("expected IsClean=true, got false")
	}
}

func TestClient_NotGitRepo(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/not_a_repo"

	mock.Register("rev-parse --show-toplevel", "", "fatal: not a git repository (or any of the parent directories): .git\n", classifyGitError("git", []string{"rev-parse", "--show-toplevel"}, 128, "fatal: not a git repository", errors.New("exit 128")))

	c := NewClient(mock)
	_, err := c.GetState(ctx, repoDir)
	if err == nil {
		t.Fatalf("expected error on non-git directory, got nil")
	}

	if !errors.Is(err, ErrNotGitRepo) {
		t.Errorf("expected ErrNotGitRepo, got: %v", err)
	}
}

func TestClient_DiffOperations(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/repo"

	mock.Register("diff", "diff --git a/file.go b/file.go\n+new line\n", "", nil)
	mock.Register("diff --staged", "diff --git a/staged.go b/staged.go\n+staged line\n", "", nil)
	mock.Register("diff commit1 commit2", "diff --git a/inter.go b/inter.go\n+inter commit line\n", "", nil)
	mock.Register("diff --name-only commit1 commit2", "fileA.go\nfileB.go\n", "", nil)

	c := NewClient(mock)

	unstagedDiff, err := c.GetDiff(ctx, repoDir, false)
	if err != nil || unstagedDiff == "" {
		t.Errorf("GetDiff(false) failed: %v, output: %s", err, unstagedDiff)
	}

	stagedDiff, err := c.GetDiff(ctx, repoDir, true)
	if err != nil || stagedDiff == "" {
		t.Errorf("GetDiff(true) failed: %v, output: %s", err, stagedDiff)
	}

	betweenDiff, err := c.GetDiffBetween(ctx, repoDir, "commit1", "commit2")
	if err != nil || betweenDiff == "" {
		t.Errorf("GetDiffBetween failed: %v, output: %s", err, betweenDiff)
	}

	changedFiles, err := c.GetChangedFilesBetween(ctx, repoDir, "commit1", "commit2")
	if err != nil || len(changedFiles) != 2 {
		t.Errorf("GetChangedFilesBetween failed: %v, files: %v", err, changedFiles)
	}
}

func TestClient_CommitExistsAndAncestry(t *testing.T) {
	mock := NewMockRunner()
	ctx := context.Background()
	repoDir := "C:/test/repo"

	mock.Register("cat-file -e c123456^{commit}", "", "", nil)
	mock.Register("cat-file -e invalid_hash^{commit}", "", "fatal: Not a valid object name", classifyGitError("git", []string{"cat-file"}, 128, "fatal: Not a valid object name", errors.New("exit 128")))

	mock.Register("merge-base --is-ancestor c1 c2", "", "", nil)
	mock.Register("merge-base --is-ancestor c2 c1", "", "", &GitError{ExitCode: 1, Command: "git", Args: []string{"merge-base", "--is-ancestor", "c2", "c1"}})

	c := NewClient(mock)

	exists, err := c.CommitExists(ctx, repoDir, "c123456")
	if err != nil || !exists {
		t.Errorf("expected commit exists=true, got %v, err: %v", exists, err)
	}

	exists, err = c.CommitExists(ctx, repoDir, "invalid_hash")
	if err != nil || exists {
		t.Errorf("expected commit exists=false, got %v, err: %v", exists, err)
	}

	// Empty commit hash should return false without running command
	exists, err = c.CommitExists(ctx, repoDir, "")
	if err != nil || exists {
		t.Errorf("expected empty commit exists=false, got %v, err: %v", exists, err)
	}

	// Same commit is always ancestor
	isAnc, err := c.IsAncestor(ctx, repoDir, "c1", "c1")
	if err != nil || !isAnc {
		t.Errorf("expected self ancestor=true, got %v, err: %v", isAnc, err)
	}

	// c1 is ancestor of c2
	isAnc, err = c.IsAncestor(ctx, repoDir, "c1", "c2")
	if err != nil || !isAnc {
		t.Errorf("expected isAncestor(c1, c2)=true, got %v, err: %v", isAnc, err)
	}

	// c2 is NOT ancestor of c1
	isAnc, err = c.IsAncestor(ctx, repoDir, "c2", "c1")
	if err != nil || isAnc {
		t.Errorf("expected isAncestor(c2, c1)=false, got %v, err: %v", isAnc, err)
	}
}

// Integration test using real temporary git repository
func TestIntegration_RealGitRepositoryLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git executable not found on host PATH, skipping integration test")
	}

	tempDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runGit := func(args ...string) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = tempDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git command %v failed: %v, output:\n%s", args, err, string(out))
		}
	}

	// 1. Initialize git repo
	runGit("init")
	runGit("config", "user.name", "Sentinel Test")
	runGit("config", "user.email", "test@sentinel.dev")
	runGit("config", "commit.gpgsign", "false")

	c := NewClient(NewOSRunner())

	// Test GetRepoRoot
	root, err := c.GetRepoRoot(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetRepoRoot failed: %v", err)
	}
	expectedRoot := filepath.ToSlash(filepath.Clean(tempDir))
	if root != expectedRoot {
		t.Errorf("expected root '%s', got '%s'", expectedRoot, root)
	}

	// 2. Initial empty state (0 commits)
	state, err := c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState on fresh repo failed: %v", err)
	}
	if state.HasCommits {
		t.Errorf("expected HasCommits=false, got true")
	}
	if state.CommitHash != "" {
		t.Errorf("expected CommitHash='', got '%s'", state.CommitHash)
	}
	if !state.IsClean {
		t.Errorf("expected IsClean=true, got false")
	}

	_, err = c.GetCurrentCommit(ctx, tempDir)
	if !errors.Is(err, ErrNoCommits) {
		t.Errorf("expected ErrNoCommits on empty repo, got: %v", err)
	}

	// 3. Create an untracked file
	file1Path := filepath.Join(tempDir, "file1.txt")
	if err := os.WriteFile(file1Path, []byte("hello sentinel\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.IsClean {
		t.Errorf("expected IsClean=false with untracked file, got true")
	}
	if len(state.UntrackedFiles) != 1 || state.UntrackedFiles[0] != "file1.txt" {
		t.Errorf("expected untracked files ['file1.txt'], got: %v", state.UntrackedFiles)
	}

	// 4. Stage and commit file1.txt
	runGit("add", "file1.txt")
	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if len(state.StagedFiles) != 1 || state.StagedFiles[0].Path != "file1.txt" {
		t.Errorf("expected staged file 'file1.txt', got: %+v", state.StagedFiles)
	}

	runGit("commit", "-m", "commit 1")

	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState after commit failed: %v", err)
	}
	if !state.HasCommits {
		t.Errorf("expected HasCommits=true, got false")
	}
	if state.CommitHash == "" {
		t.Errorf("expected non-empty commit hash")
	}
	if !state.IsClean {
		t.Errorf("expected IsClean=true after commit, got false")
	}

	commit1 := state.CommitHash

	exists, err := c.CommitExists(ctx, tempDir, commit1)
	if err != nil || !exists {
		t.Errorf("expected commit1 exists, got %v, err: %v", exists, err)
	}

	// 5. Worktree modification
	if err := os.WriteFile(file1Path, []byte("hello sentinel modified\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	if state.IsClean {
		t.Errorf("expected IsClean=false after worktree edit")
	}
	if len(state.UnstagedFiles) != 1 || state.UnstagedFiles[0].Path != "file1.txt" {
		t.Errorf("expected unstaged file 'file1.txt', got: %+v", state.UnstagedFiles)
	}

	diffText, err := c.GetDiff(ctx, tempDir, false)
	if err != nil {
		t.Fatalf("GetDiff failed: %v", err)
	}
	if diffText == "" {
		t.Errorf("expected non-empty diff for unstaged changes")
	}

	// 6. Create file2.txt, stage both and commit 2
	file2Path := filepath.Join(tempDir, "file2.txt")
	if err := os.WriteFile(file2Path, []byte("second file\n"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	runGit("add", "file1.txt", "file2.txt")
	runGit("commit", "-m", "commit 2")

	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState failed: %v", err)
	}
	commit2 := state.CommitHash

	// Check changed files between commit 1 and commit 2
	changedFiles, err := c.GetChangedFilesBetween(ctx, tempDir, commit1, commit2)
	if err != nil {
		t.Fatalf("GetChangedFilesBetween failed: %v", err)
	}
	if len(changedFiles) != 2 {
		t.Errorf("expected 2 changed files between commit1 and commit2, got %d: %v", len(changedFiles), changedFiles)
	}

	// Check ancestry
	isAnc, err := c.IsAncestor(ctx, tempDir, commit1, commit2)
	if err != nil || !isAnc {
		t.Errorf("expected commit1 is ancestor of commit2, got %v, err: %v", isAnc, err)
	}
	isAnc, err = c.IsAncestor(ctx, tempDir, commit2, commit1)
	if err != nil || isAnc {
		t.Errorf("expected commit2 is NOT ancestor of commit1, got %v, err: %v", isAnc, err)
	}

	// 7. Checkout commit1 directly to enter detached HEAD state
	runGit("checkout", commit1)
	state, err = c.GetState(ctx, tempDir)
	if err != nil {
		t.Fatalf("GetState on detached HEAD failed: %v", err)
	}
	if !state.IsDetached {
		t.Errorf("expected IsDetached=true on detached HEAD, got false")
	}
	if state.Branch != "HEAD" {
		t.Errorf("expected Branch 'HEAD' on detached HEAD, got '%s'", state.Branch)
	}
	if state.CommitHash != commit1 {
		t.Errorf("expected commit hash '%s', got '%s'", commit1, state.CommitHash)
	}
}

package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wake/wake/internal/git"
)

// GitRepo encapsulates a real, isolated Git repository in a temporary directory for testing.
type GitRepo struct {
	Dir     string
	GitPath string
	T       testing.TB
	client  git.Client
}

// locateGit finds the git executable across PATH and standard Windows installation paths.
func locateGit() string {
	if p, err := exec.LookPath("git"); err == nil && p != "" {
		return p
	}
	for _, candidate := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "git"
}

// NewGitRepo initializes a fresh, temporary Git repository configured for tests.
func NewGitRepo(t testing.TB) *GitRepo {
	t.Helper()
	gitBin := locateGit()
	if _, err := exec.LookPath(gitBin); err != nil && gitBin == "git" {
		t.Skip("git binary not available on host system, skipping git test")
	}

	tmpDir := t.TempDir()
	repo := &GitRepo{
		Dir:     tmpDir,
		GitPath: gitBin,
		T:       t,
		client:  git.NewClient(nil),
	}

	// Initialize repository with main branch
	if _, err := repo.RunGitAllowError("init", "-b", "main"); err != nil {
		repo.RunGit("init")
	}

	// Configure identity and flags
	repo.RunGit("config", "user.name", "Sentinel Test")
	repo.RunGit("config", "user.email", "test@sentinel.local")
	repo.RunGit("config", "commit.gpgsign", "false")
	repo.RunGit("config", "core.quotepath", "false")
	repo.RunGit("config", "init.defaultBranch", "main")

	return repo
}

// WriteFile writes content to a relative file path inside the repository, creating parent dirs if needed.
func (g *GitRepo) WriteFile(relPath, content string) {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		g.T.Fatalf("failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		g.T.Fatalf("failed to write file %s: %v", fullPath, err)
	}
}

// ReadFile reads and returns the string content of a file relative to repository root.
func (g *GitRepo) ReadFile(relPath string) string {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	data, err := os.ReadFile(fullPath)
	if err != nil {
		g.T.Fatalf("failed to read file %s: %v", fullPath, err)
	}
	return string(data)
}

// DeleteFile removes a physical file from the repository working tree.
func (g *GitRepo) DeleteFile(relPath string) {
	g.T.Helper()
	fullPath := filepath.Join(g.Dir, filepath.FromSlash(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		g.T.Fatalf("failed to delete file %s: %v", fullPath, err)
	}
}

// Stage stages one or more relative file paths. If no paths are provided, runs 'git add -A'.
func (g *GitRepo) Stage(relPaths ...string) {
	g.T.Helper()
	if len(relPaths) == 0 {
		g.RunGit("add", "-A")
		return
	}
	args := append([]string{"add"}, relPaths...)
	g.RunGit(args...)
}

// Commit stages all changes and creates a new commit, returning the full commit hash.
func (g *GitRepo) Commit(msg string) string {
	g.T.Helper()
	g.RunGit("add", "-A")
	g.RunGit("commit", "-m", msg)
	return g.CurrentCommit()
}

// CommitOnly stages specific files and commits them, returning the commit hash.
func (g *GitRepo) CommitOnly(msg string, relPaths ...string) string {
	g.T.Helper()
	g.Stage(relPaths...)
	g.RunGit("commit", "-m", msg)
	return g.CurrentCommit()
}

// Branch creates a new branch without switching to it.
func (g *GitRepo) Branch(name string) {
	g.T.Helper()
	g.RunGit("branch", name)
}

// CreateAndCheckoutBranch creates and checks out a new branch.
func (g *GitRepo) CreateAndCheckoutBranch(name string) {
	g.T.Helper()
	g.RunGit("checkout", "-b", name)
}

// Checkout switches to an existing branch or commit hash.
func (g *GitRepo) Checkout(branchOrCommit string) {
	g.T.Helper()
	g.RunGit("checkout", branchOrCommit)
}

// CurrentCommit returns the full commit hash of HEAD.
func (g *GitRepo) CurrentCommit() string {
	g.T.Helper()
	out, err := g.RunGitAllowError("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return out
}

// CurrentBranch returns the current active branch name.
func (g *GitRepo) CurrentBranch() string {
	g.T.Helper()
	out, err := g.RunGitAllowError("branch", "--show-current")
	if err == nil && out != "" {
		return out
	}
	out, err = g.RunGitAllowError("symbolic-ref", "--short", "HEAD")
	if err == nil && out != "" {
		return out
	}
	out, err = g.RunGitAllowError("rev-parse", "--abbrev-ref", "HEAD")
	if err == nil && out != "" {
		return out
	}
	return "main"
}

// Client returns a git.Client bound to the repository directory.
func (g *GitRepo) Client() git.Client {
	return g.client
}

// GetState returns the current live RepositoryState snapshot.
func (g *GitRepo) GetState() *git.RepositoryState {
	g.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := g.client.GetState(ctx, g.Dir)
	if err != nil {
		g.T.Fatalf("GetState failed: %v", err)
	}
	return state
}

// RunGit executes a git command in the repository directory and returns standard output trimmed.
func (g *GitRepo) RunGit(args ...string) string {
	g.T.Helper()
	cmd := exec.Command(g.GitPath, args...)
	cmd.Dir = g.Dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		g.T.Fatalf("git %v failed in %s: %v\nOutput: %s", args, g.Dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// RunGitAllowError executes a git command allowing non-zero exit codes.
func (g *GitRepo) RunGitAllowError(args ...string) (string, error) {
	g.T.Helper()
	cmd := exec.Command(g.GitPath, args...)
	cmd.Dir = g.Dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// CauseConflict simulates a merge conflict between current branch and targetBranch on conflictFile.
func (g *GitRepo) CauseConflict(targetBranch, conflictFile, baseContent, currentContent, targetContent string) {
	g.T.Helper()
	baseBranch := g.CurrentBranch()

	g.WriteFile(conflictFile, baseContent)
	g.Commit("Base commit for conflict")

	g.CreateAndCheckoutBranch(targetBranch)
	g.WriteFile(conflictFile, targetContent)
	g.Commit("Target branch conflicting edit")

	g.Checkout(baseBranch)
	g.WriteFile(conflictFile, currentContent)
	g.Commit("Current branch conflicting edit")

	_, _ = g.RunGitAllowError("merge", targetBranch)
}

// Cleanup performs repository cleanup.
func (g *GitRepo) Cleanup() {
	// Standard temp dir cleanup is handled by testing.TB
}

package git

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
)

// Client defines the high-level Git operations required by Sentinel.
type Client interface {
	// GetState extracts a complete snapshot of repository state (commit, branch, cleanliness, modified files).
	GetState(ctx context.Context, repoPath string) (*RepositoryState, error)

	// GetCurrentCommit returns the full commit hash of HEAD.
	GetCurrentCommit(ctx context.Context, repoPath string) (string, error)

	// GetCurrentBranch returns current branch name or "HEAD" if detached.
	GetCurrentBranch(ctx context.Context, repoPath string) (string, error)

	// GetStatus parses working tree and index status into StatusResult.
	GetStatus(ctx context.Context, repoPath string) (*StatusResult, error)

	// GetDiff returns textual diff for staged or unstaged changes.
	GetDiff(ctx context.Context, repoPath string, staged bool) (string, error)

	// GetDiffBetween returns textual diff between two commit hashes.
	GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error)

	// GetChangedFilesBetween returns list of files changed between two commits.
	GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error)

	// IsClean checks if repository has no unstaged, staged, or untracked changes.
	IsClean(ctx context.Context, repoPath string) (bool, error)

	// CommitExists checks whether a specific commit hash exists locally.
	CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error)

	// IsAncestor checks if ancestorCommit is an ancestor of descendantCommit.
	IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error)

	// GetRepoRoot returns the top-level path of the git working tree.
	GetRepoRoot(ctx context.Context, dir string) (string, error)
}

// client is the standard implementation of Client.
type client struct {
	runner Runner
}

// NewClient creates a new Client instance backed by the provided Runner.
func NewClient(runner Runner) Client {
	if runner == nil {
		runner = NewOSRunner()
	}
	return &client{
		runner: runner,
	}
}

// GetRepoRoot returns the top-level directory of the Git repository containing dir.
func (c *client) GetRepoRoot(ctx context.Context, dir string) (string, error) {
	stdout, _, err := c.runner.Run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(stdout))
	return filepath.ToSlash(filepath.Clean(root)), nil
}

// GetCurrentCommit returns the full commit hash of HEAD.
func (c *client) GetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	stdout, stderr, err := c.runner.Run(ctx, repoPath, "rev-parse", "HEAD")
	if err != nil {
		if errors.Is(err, ErrNoCommits) ||
			strings.Contains(strings.ToLower(string(stderr)), "ambiguous argument 'head'") ||
			strings.Contains(strings.ToLower(string(stderr)), "unknown revision") {
			return "", ErrNoCommits
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// GetCurrentBranch returns the current branch name, or "HEAD" if in detached HEAD state.
func (c *client) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	// First try: git branch --show-current
	stdout, _, err := c.runner.Run(ctx, repoPath, "branch", "--show-current")
	if err == nil {
		b := strings.TrimSpace(string(stdout))
		if b != "" {
			return b, nil
		}
	}

	// Fallback for detached HEAD or older git: git rev-parse --abbrev-ref HEAD
	stdout, _, err = c.runner.Run(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		b := strings.TrimSpace(string(stdout))
		if b != "" {
			return b, nil
		}
	}

	// Fallback for newly initialized repos: git symbolic-ref --short HEAD
	stdout, _, err = c.runner.Run(ctx, repoPath, "symbolic-ref", "--short", "HEAD")
	if err == nil {
		b := strings.TrimSpace(string(stdout))
		if b != "" {
			return b, nil
		}
	}

	return "HEAD", nil
}

// GetStatus parses working tree and index status into a structured StatusResult.
func (c *client) GetStatus(ctx context.Context, repoPath string) (*StatusResult, error) {
	stdout, _, err := c.runner.Run(ctx, repoPath, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return nil, err
	}
	return ParsePorcelainStatus(string(stdout)), nil
}

// GetState extracts a complete snapshot of repository state.
func (c *client) GetState(ctx context.Context, repoPath string) (*RepositoryState, error) {
	rootPath, err := c.GetRepoRoot(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	hasCommits := true
	commitHash, err := c.GetCurrentCommit(ctx, repoPath)
	if err != nil {
		if errors.Is(err, ErrNoCommits) {
			hasCommits = false
			commitHash = ""
		} else {
			return nil, err
		}
	}

	branch, err := c.GetCurrentBranch(ctx, repoPath)
	if err != nil {
		branch = "HEAD"
	}
	isDetached := branch == "HEAD" || branch == ""
	if branch == "" {
		branch = "HEAD"
	}

	status, err := c.GetStatus(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	modified := ExtractModifiedFiles(status)
	hasConflicts := len(status.UnmergedFiles) > 0

	state := &RepositoryState{
		RootPath:          rootPath,
		Branch:            branch,
		CommitHash:        commitHash,
		IsDetached:        isDetached,
		HasCommits:        hasCommits,
		IsClean:           status.IsClean,
		HasMergeConflicts: hasConflicts,
		StagedFiles:       status.StagedFiles,
		UnstagedFiles:     status.UnstagedFiles,
		UntrackedFiles:    status.UntrackedFiles,
		UnmergedFiles:     status.UnmergedFiles,
		ModifiedFiles:     modified,
	}

	return state, nil
}

// GetDiff returns textual diff for staged or unstaged changes.
func (c *client) GetDiff(ctx context.Context, repoPath string, staged bool) (string, error) {
	args := []string{"diff"}
	if staged {
		args = []string{"diff", "--staged"}
	}
	stdout, _, err := c.runner.Run(ctx, repoPath, args...)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

// GetDiffBetween returns textual diff between two commit hashes.
func (c *client) GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error) {
	if strings.TrimSpace(fromCommit) == "" || strings.TrimSpace(toCommit) == "" {
		return "", errors.New("both fromCommit and toCommit must be specified")
	}
	stdout, _, err := c.runner.Run(ctx, repoPath, "diff", fromCommit, toCommit)
	if err != nil {
		return "", err
	}
	return string(stdout), nil
}

// GetChangedFilesBetween returns the list of file paths changed between two commits.
func (c *client) GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error) {
	fromCommit = strings.TrimSpace(fromCommit)
	toCommit = strings.TrimSpace(toCommit)
	if fromCommit == "" || toCommit == "" || fromCommit == toCommit {
		return []string{}, nil
	}

	stdout, _, err := c.runner.Run(ctx, repoPath, "diff", "--name-only", fromCommit, toCommit)
	if err != nil {
		return nil, err
	}
	return ParseNameOnlyList(string(stdout)), nil
}

// IsClean checks if the repository has no unstaged, staged, untracked, or unmerged changes.
func (c *client) IsClean(ctx context.Context, repoPath string) (bool, error) {
	status, err := c.GetStatus(ctx, repoPath)
	if err != nil {
		return false, err
	}
	return status.IsClean, nil
}

// CommitExists checks whether a specific commit hash exists in the local repository object database.
func (c *client) CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error) {
	commitHash = strings.TrimSpace(commitHash)
	if commitHash == "" {
		return false, nil
	}
	_, _, err := c.runner.Run(ctx, repoPath, "cat-file", "-e", commitHash+"^{commit}")
	if err != nil {
		return false, nil
	}
	return true, nil
}

// IsAncestor checks if ancestorCommit is an ancestor of descendantCommit.
func (c *client) IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error) {
	ancestorCommit = strings.TrimSpace(ancestorCommit)
	descendantCommit = strings.TrimSpace(descendantCommit)
	if ancestorCommit == "" || descendantCommit == "" {
		return false, nil
	}
	if ancestorCommit == descendantCommit {
		return true, nil
	}

	_, _, err := c.runner.Run(ctx, repoPath, "merge-base", "--is-ancestor", ancestorCommit, descendantCommit)
	if err == nil {
		return true, nil
	}

	var gitErr *GitError
	if errors.As(err, &gitErr) {
		if gitErr.ExitCode == 1 {
			return false, nil
		}
	}

	return false, err
}

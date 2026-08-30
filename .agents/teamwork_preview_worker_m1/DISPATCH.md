## 2026-08-28T17:00:00Z

Objective: Implement Milestone 1 - Git CLI Wrapper (Requirement R1).
Write Ownership: Exclusive ownership of `C:/Users/USER/Desktop/Sentinel/internal/git/`.

Tasks:
1. Create `internal/git/` package implementing:
   - `models.go`: StatusCode, FileStatus, StatusResult, RepositoryState, FileChange.
   - `errors.go`: Structured error definitions (ErrGitNotFound, ErrNotGitRepo, ErrNoCommits, ErrInvalidCommit, ErrGitLockExists, ErrDubiousOwnership, ErrMergeConflict, and GitError wrapping command, args, exit code, and stderr).
   - `runner.go`: Runner interface (`Run(ctx context.Context, dir string, args ...string) (stdout []byte, stderr []byte, err error)`), OSRunner implementation, and MockRunner for testing.
   - `parser.go`: Porcelain status parser for `git status --porcelain=v1 -uall`, handling staged, unstaged, untracked, unmerged, and renamed files with path normalization (`filepath.ToSlash`).
   - `client.go`: Client interface and NewClient(runner Runner) implementation providing:
     - `GetState(ctx context.Context, repoPath string) (*RepositoryState, error)`
     - `GetCurrentCommit(ctx context.Context, repoPath string) (string, error)`
     - `GetCurrentBranch(ctx context.Context, repoPath string) (string, error)`
     - `GetStatus(ctx context.Context, repoPath string) (*StatusResult, error)`
     - `GetDiff(ctx context.Context, repoPath string, staged bool) (string, error)`
     - `GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error)`
     - `GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error)`
     - `IsClean(ctx context.Context, repoPath string) (bool, error)`
     - `CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error)`
     - `IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error)`
     - `GetRepoRoot(ctx context.Context, dir string) (string, error)`
     - Handling edge cases: empty repo with 0 commits (ErrNoCommits, empty commit hash without failing GetState), detached HEAD (IsDetached=true, Branch="HEAD").
2. Write unit tests:
   - `parser_test.go`: Comprehensive unit tests for status parsing.
   - `client_test.go`: Unit tests with MockRunner and integration tests with real temporary git repository via `t.TempDir()`.
3. Run `go test -v ./internal/git/...` and verify 100% passing tests.
4. Record progress in `progress.md` and write complete 5-component report to `handoff.md` in working directory.

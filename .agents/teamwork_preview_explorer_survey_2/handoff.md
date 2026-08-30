# Survey Report: Requirement R1 (Git CLI Wrapper) Analysis & Interface Design

**Agent:** teamwork_preview_explorer_survey_2  
**Working Directory:** `C:/Users/USER/Desktop/Sentinel/.agents/teamwork_preview_explorer_survey_2`  
**Scope:** Requirement R1 (Git CLI Wrapper) Architecture, Commands, Error Matrix, and Go Interface Models  
**Timestamp:** 2026-08-28T17:10:00Z  

---

## 1. Observation

### 1.1 Specification Requirements
From `C:/Users/USER/Desktop/Sentinel/.agents/ORIGINAL_REQUEST.md`:
- Lines 18-20:
  > `### R1. Git CLI Wrapper`  
  > `Build a utility layer that shells out to the local git binary to retrieve current repository information. It must be able to retrieve the current commit hash, list modified files, and list uncommitted changes.`

From `C:/Users/USER/Desktop/Sentinel/Project Sentinel.md`:
- Section 14 (lines 558-615):
  > Sentinel must compare:
  > - Git: checkpoint commit, current commit, branch, uncommitted files, modified files
  > - Files: expected files, changed task files, deleted files, unexpected modifications
  > - Task state: completed steps, blockers, remaining work, decisions, constraints
- Section 9 (lines 346-420):
  > Reconciliation produces `SAFE`, `STALE`, or `CONFLICT`.
  > Sentinel must prefer uncertainty over silently resuming from incorrect state.

### 1.2 Existing Codebase Integration Points
- `go.mod` (lines 1-10):
  - Module name: `github.com/sentinel/sentinel`
  - Go version: `1.27.0`
  - Dependencies: `github.com/google/uuid v1.6.0`, `github.com/spf13/cobra v1.10.2`, `modernc.org/sqlite v1.57.0`.
- `internal/state/models.go` (lines 54-64):
  - `Checkpoint` fields: `Commit string`, `Branch string`, `Repository string`, `StateData State`.
- `internal/state/models.go` (lines 25-38):
  - `State` field: `LastVerified string` (Git commit hash), `Constraints []string`, `Decisions []Decision`, `Completed []string`.

### 1.3 Direct CLI Command Verification Observations
Testing Git commands on Windows (Git version `2.55.0.windows.3`) under controlled conditions yielded the following direct observations:

1. **Commit Hash Resolution (`git rev-parse HEAD`)**:
   - Initialized empty repo (0 commits): Exits with code `128` (or `1` in subshell), stderr:
     `fatal: ambiguous argument 'HEAD': unknown revision or path not in the working tree.`
   - Repository with commits: Exits with code `0`, stdout: 40-character hexadecimal SHA string followed by newline (e.g. `3c6dafe29188a449d...`).

2. **Branch Name Resolution (`git branch --show-current` vs `git rev-parse --abbrev-ref HEAD`)**:
   - On standard branch (`main`): `git branch --show-current` outputs `main\n` (exit code `0`).
   - In detached HEAD state: `git branch --show-current` outputs empty string `""` (exit code `0`), while `git rev-parse --abbrev-ref HEAD` outputs `"HEAD\n"`.

3. **Status and File Listing (`git status --porcelain=v1 -uall`)**:
   - Untracked file: `?? filename.txt`
   - Staged newly added file: `A  filename.txt`
   - Staged modified file: `M  filename.txt`
   - Unstaged modified file: ` M filename.txt`
   - Staged and unstaged modification: `MM filename.txt`
   - Staged deleted file: `D  filename.txt`
   - Unstaged deleted file: ` D filename.txt`
   - Staged renamed file: `R  orig_path -> new_path`
   - Nested untracked files with `-uall`: Correctly emits each individual file path (e.g. `?? dir1/nested.txt`) instead of collapsing to directory `?? dir1/`.

4. **Diffs and Uncommitted Changes**:
   - Staged diff (index vs HEAD): `git diff --staged --name-status` / `git diff --staged`
   - Unstaged diff (worktree vs index): `git diff --name-status` / `git diff`
   - Combined diff (worktree vs HEAD): `git diff HEAD --name-status` / `git diff HEAD`
   - Inter-commit diff (checkpoint vs current): `git diff --name-status <commitA> <commitB>`

5. **Commit Ancestry (`git merge-base --is-ancestor <ancestor> <descendant>`)**:
   - Exit code `0`: `<ancestor>` is reachable in history of `<descendant>`.
   - Exit code `1`: `<ancestor>` is NOT an ancestor (diverged, rebased, or descendant is older).
   - Exit code `>1`: Object name error (e.g. unknown commit).

6. **Non-Git Directory Check**:
   - Executing in non-git directory: Exits with code `128`, stderr contains: `fatal: not a git repository (or any of the parent directories): .git`.

---

## 2. Logic Chain

### 2.1 Command Selection and Parsing Logic

```
+-----------------------------------------------------------------------------+
|                           SENTINEL GIT CLI ENGINE                           |
+-----------------------------------------------------------------------------+
         |
         +---> 1. Identity & Location
         |     |-- Repo Root:    git rev-parse --show-toplevel
         |     |-- Current Hash: git rev-parse HEAD
         |     `-- Branch Name:  git branch --show-current / rev-parse --abbrev-ref HEAD
         |
         +---> 2. Working Tree State & File Triage
         |     |-- Command:      git status --porcelain=v1 -uall
         |     |-- Index [X]:    'M', 'A', 'D', 'R', 'C' -> StagedFiles
         |     |-- Worktree [Y]: 'M', 'D'                -> UnstagedFiles
         |     |-- Untracked:    '??'                    -> UntrackedFiles
         |     `-- Conflicts:    'UU', 'AA', 'DD', etc.  -> UnmergedFiles
         |
         +---> 3. Diffs & Granular History
         |     |-- Staged Diff:  git diff --staged [--name-status]
         |     |-- Unstaged:     git diff [--name-status]
         |     `-- Inter-Commit: git diff --name-status <cp_commit> <head_commit>
         |
         `---> 4. Ancestry & Object Verification
               |-- Object Check: git cat-file -e <commit_hash>^{commit}
               `-- Ancestor:     git merge-base --is-ancestor <cp_commit> <head_commit>
```

#### Step 1: Commit Hash and Empty Repo Handling
- When querying `git rev-parse HEAD`, an exit code of `128` with stderr containing `ambiguous argument 'HEAD'` or `does not have any commits yet` must NOT trigger an unhandled error panic. Instead, it indicates an empty repository (0 commits).
- Model mapping: `CommitHash: ""`, `HasCommits: false`.

#### Step 2: Detached HEAD Detection
- Checking out a commit directly (detached HEAD) is a valid developer state.
- If `git branch --show-current` returns `""` and `git rev-parse HEAD` returns a valid hash, Sentinel flags `IsDetached = true` and `Branch = "HEAD"`.

#### Step 3: Granular File Status Parsing (`--porcelain=v1 -uall`)
- Parsing must read line-by-line:
  - First two bytes `[0:2]` are status codes `XY`.
  - Byte `[2]` is a space separator.
  - Byte `[3:]` is the file path (or `old_path -> new_path` for renames).
- Path normalization: On Windows, Git CLI returns forward-slash paths (`foo/bar.go`). Go code must normalize paths with `filepath.ToSlash(filepath.Clean(path))` for cross-platform consistency.

---

### 2.2 Comprehensive Error Matrix

| Error Condition | Triggering Event / Git Stderr | Exit Code | Sentinel Error Type | Handling Strategy |
|---|---|---|---|---|
| **Git Binary Missing** | `exec.LookPath("git")` fails | N/A | `ErrGitNotFound` | Fail fast with descriptive error stating Git CLI is required. |
| **Not a Git Repo** | Stderr: `fatal: not a git repository` | `128` | `ErrNotGitRepo` | Return structured error; caller cannot perform Git operations. |
| **Empty Repo (0 Commits)** | Stderr: `fatal: ambiguous argument 'HEAD'` | `128` | `ErrNoCommits` | Return `HasCommits: false`, `CommitHash: ""`, allow status parsing. |
| **Detached HEAD** | `git branch --show-current` is empty | `0` | None (Valid State) | Set `IsDetached: true`, `Branch: "HEAD"`. |
| **Invalid Commit Hash** | Stderr: `fatal: Not a valid object name` | `128` | `ErrInvalidCommit` | Invalidate checkpoint, trigger `CONFLICT` in reconciliation. |
| **Git Lock File Active** | Stderr: `fatal: Unable to create '.../.git/index.lock'` | `128` | `ErrGitLockExists` | Inform user of concurrent git process or stale lock file. |
| **Dubious Ownership** | Stderr: `fatal: detected dubious ownership` | `128` | `ErrDubiousOwnership` | Report `safe.directory` configuration requirement. |
| **Unmerged Conflicts** | Status codes `UU`, `AA`, `DD`, `UD`, `DU` | `0` | `ErrMergeConflict` | Populate `UnmergedFiles`, immediately escalate to `CONFLICT`. |
| **Context Timeout** | `ctx.Done()` received | N/A | `context.DeadlineExceeded` | Terminate process cleanly without blocking Sentinel CLI. |

---

### 2.3 Proposed Package Design (`internal/git`)

#### 1. Command Execution Abstraction (`internal/git/runner.go`)
Decouples process execution from business logic, allowing 100% in-memory unit tests without spawning external processes.

```go
package git

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner defines the interface for running Git CLI commands.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout []byte, stderr []byte, err error)
}

// OSRunner executes real git commands using os/exec.
type OSRunner struct{}

func NewOSRunner() *OSRunner {
	return &OSRunner{}
}

func (r *OSRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}
```

#### 2. Data Models (`internal/git/models.go`)
```go
package git

// StatusCode represents single-character Git porcelain status codes.
type StatusCode string

const (
	StatusUnmodified StatusCode = " "
	StatusModified   StatusCode = "M"
	StatusAdded      StatusCode = "A"
	StatusDeleted    StatusCode = "D"
	StatusRenamed    StatusCode = "R"
	StatusCopied     StatusCode = "C"
	StatusUntracked  StatusCode = "?"
	StatusIgnored    StatusCode = "!"
	StatusUnmerged   StatusCode = "U"
)

// FileStatus represents the status of an individual file in index and working tree.
type FileStatus struct {
	Path           string     `json:"path"`
	OrigPath       string     `json:"orig_path,omitempty"` // For renames
	StagingStatus  StatusCode `json:"staging_status"`      // Status in index ('M', 'A', 'D', 'R', etc.)
	WorkTreeStatus StatusCode `json:"worktree_status"`     // Status in working tree ('M', 'D', '?', etc.)
}

// StatusResult contains parsed output from git status.
type StatusResult struct {
	StagedFiles    []FileStatus `json:"staged_files"`
	UnstagedFiles  []FileStatus `json:"unstaged_files"`
	UntrackedFiles []string     `json:"untracked_files"`
	UnmergedFiles  []string     `json:"unmerged_files"`
	IsClean        bool         `json:"is_clean"`
}

// RepositoryState represents a complete live snapshot of the git repository.
type RepositoryState struct {
	RootPath          string       `json:"root_path"`
	Branch            string       `json:"branch"`
	CommitHash        string       `json:"commit_hash"`
	IsDetached        bool         `json:"is_detached"`
	HasCommits        bool         `json:"has_commits"`
	IsClean           bool         `json:"is_clean"`
	HasMergeConflicts bool         `json:"has_merge_conflicts"`
	StagedFiles       []FileStatus `json:"staged_files"`
	UnstagedFiles     []FileStatus `json:"unstaged_files"`
	UntrackedFiles    []string     `json:"untracked_files"`
	UnmergedFiles     []string     `json:"unmerged_files"`
	ModifiedFiles     []string     `json:"modified_files"` // Consolidated list of all altered paths
}

// FileChange represents a file modified between two commits.
type FileChange struct {
	Path     string     `json:"path"`
	OrigPath string     `json:"orig_path,omitempty"`
	Status   StatusCode `json:"status"` // 'M', 'A', 'D', 'R'
}
```

#### 3. Client Interface (`internal/git/client.go`)
```go
package git

import (
	"context"
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
	GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]FileChange, error)

	// IsClean checks if repository has no unstaged, staged, or untracked changes.
	IsClean(ctx context.Context, repoPath string) (bool, error)

	// CommitExists checks whether a specific commit hash exists locally.
	CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error)

	// IsAncestor checks if ancestorCommit is an ancestor of descendantCommit.
	IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error)

	// GetRepoRoot returns the top-level path of the git working tree.
	GetRepoRoot(ctx context.Context, dir string) (string, error)
}
```

#### 4. Error Definitions (`internal/git/errors.go`)
```go
package git

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrGitNotFound      = errors.New("git binary not found in PATH")
	ErrNotGitRepo       = errors.New("not a git repository")
	ErrNoCommits        = errors.New("repository has no commits")
	ErrInvalidCommit    = errors.New("invalid or unknown commit hash")
	ErrGitLockExists    = errors.New("git index lock file exists")
	ErrDubiousOwnership = errors.New("git detected dubious ownership in repository directory")
	ErrMergeConflict    = errors.New("repository has unresolved merge conflicts")
)

// GitError wraps exit code and stderr for rich diagnostic context.
type GitError struct {
	Command  string
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *GitError) Error() string {
	if e.Stderr != "" {
		return fmt.Sprintf("git %s failed (exit %d): %s", strings.Join(e.Args, " "), e.ExitCode, strings.TrimSpace(e.Stderr))
	}
	return fmt.Sprintf("git %s failed: %v", strings.Join(e.Args, " "), e.Err)
}

func (e *GitError) Unwrap() error {
	return e.Err
}
```

---

## 3. Caveats

1. **Path Separator Normalization**:
   - Git CLI returns paths in POSIX format (`a/b/c.txt`) on all operating systems including Windows.
   - When matching against Windows OS paths or user-provided constraint arguments, all paths MUST be converted using `filepath.ToSlash(filepath.Clean(path))`.
2. **Git Version Compatibility**:
   - `git status --porcelain=v1 -uall`, `git rev-parse HEAD`, and `git diff` are supported across all modern Git versions (1.7+ through 2.55+).
   - `git branch --show-current` was introduced in Git 2.22; fallback to `git rev-parse --abbrev-ref HEAD` ensures backward compatibility with older git distributions.
3. **Quoted / Escaped File Paths in Git Output**:
   - By default, Git quotes filenames containing spaces or non-ASCII characters (e.g. `"path/with space.go"`).
   - The parser must strip enclosing double quotes (`strings.Trim(path, "\"")`) or use `-z` null-delimited output to avoid filename parsing bugs.
4. **Submodule State**:
   - Submodules modified in the working tree produce ` S` or `M ` status. For the MVP, submodules can be treated as regular modified entries.

---

## 4. Conclusion

1. **Requirement R1 is fully mapped and architecturally defined**:
   - The exact command suite for commit resolution, branch detection, file status triage, diff generation, and ancestry checks has been verified.
   - All edge cases (empty repository, detached HEAD, missing git, index locks, dubious ownership, merge conflicts) are classified with dedicated Go error types.
2. **Seamless Integration with R2 (Reconciliation Engine)**:
   - `RepositoryState` provides the exact input required by `reconcile.Reconcile(checkpoint, repoState, taskFiles)` without requiring R2 to execute CLI commands directly.
3. **High Testability**:
   - The separation between `Runner` and `Client` enables comprehensive unit testing with `MockRunner` without needing a Git installation for basic tests, while integration tests can execute against isolated temporary repositories created via `t.TempDir()`.

---

## 5. Verification Method

To independently verify this specification and any future implementation:

1. **Unit Test Verification (Mocked Runner)**:
   - Test `status_parser.go` with synthetic porcelain outputs covering:
     - Clean state (`""`)
     - Modified staged (`M  file.go`)
     - Modified unstaged (` M file.go`)
     - Both staged & unstaged (`MM file.go`)
     - Renamed file (`R  old.go -> new.go`)
     - Untracked files with nested directories (`?? dir/file.go`)
     - Merge conflicts (`UU file.go`)
2. **Integration Test Verification (Real Git CLI in Temp Dir)**:
   - Run `go test -v ./internal/git/...` using temporary test repositories created with `t.TempDir()`.
   - Verify `GetState` correctly captures commit hash, branch, clean status, and modified files across real Git lifecycle transitions (init, add, commit, edit, branch, checkout detached).
3. **Invalidation Criteria**:
   - If `GetState` fails or panics on a repository with 0 commits, the implementation is invalid.
   - If `GetState` returns `IsClean: true` when untracked files exist, the implementation is invalid.
   - If `GetState` fails on a detached HEAD state, the implementation is invalid.

package git

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrGitNotFound is returned when the git executable is not available on PATH.
	ErrGitNotFound = errors.New("git binary not found in PATH")

	// ErrNotGitRepo is returned when a command is executed in a directory that is not a git repository.
	ErrNotGitRepo = errors.New("not a git repository")

	// ErrNoCommits is returned when a repository has 0 commits (e.g. freshly initialized).
	ErrNoCommits = errors.New("repository has no commits")

	// ErrInvalidCommit is returned when an invalid or unknown commit hash is referenced.
	ErrInvalidCommit = errors.New("invalid or unknown commit hash")

	// ErrGitLockExists is returned when git cannot proceed because an index.lock or other lock file exists.
	ErrGitLockExists = errors.New("git index lock file exists")

	// ErrDubiousOwnership is returned when git refuses to operate due to safe.directory ownership checks.
	ErrDubiousOwnership = errors.New("git detected dubious ownership in repository directory")

	// ErrMergeConflict is returned when there are unresolved merge conflicts in the working tree.
	ErrMergeConflict = errors.New("repository has unresolved merge conflicts")
)

// GitError wraps command invocation details, exit code, and stderr for rich diagnostic context.
type GitError struct {
	Command  string
	Args     []string
	ExitCode int
	Stderr   string
	Err      error
}

func (e *GitError) Error() string {
	cmdStr := e.Command
	if cmdStr == "" {
		cmdStr = "git"
	}
	if len(e.Args) > 0 {
		cmdStr += " " + strings.Join(e.Args, " ")
	}
	trimmedStderr := strings.TrimSpace(e.Stderr)
	if trimmedStderr != "" {
		return fmt.Sprintf("git command '%s' failed (exit %d): %s", cmdStr, e.ExitCode, trimmedStderr)
	}
	if e.Err != nil {
		return fmt.Sprintf("git command '%s' failed (exit %d): %v", cmdStr, e.ExitCode, e.Err)
	}
	return fmt.Sprintf("git command '%s' failed with exit code %d", cmdStr, e.ExitCode)
}

func (e *GitError) Unwrap() error {
	return e.Err
}

// classifyGitError analyzes the command stderr and underlying error, wrapping with a domain sentinel error if recognized.
func classifyGitError(command string, args []string, exitCode int, stderr string, underlyingErr error) error {
	s := strings.ToLower(stderr)
	var sentinel error

	switch {
	case strings.Contains(s, "not a git repository"):
		sentinel = ErrNotGitRepo
	case strings.Contains(s, "ambiguous argument 'head'") ||
		strings.Contains(s, "does not have any commits yet") ||
		strings.Contains(s, "unknown revision or path not in the working tree"):
		sentinel = ErrNoCommits
	case strings.Contains(s, "not a valid object name") ||
		strings.Contains(s, "unknown revision") ||
		strings.Contains(s, "bad object") ||
		strings.Contains(s, "needed a single revision"):
		sentinel = ErrInvalidCommit
	case strings.Contains(s, "index.lock") ||
		(strings.Contains(s, "unable to create") && strings.Contains(s, ".lock")):
		sentinel = ErrGitLockExists
	case strings.Contains(s, "detected dubious ownership"):
		sentinel = ErrDubiousOwnership
	case strings.Contains(s, "unmerged") ||
		strings.Contains(s, "merge conflict") ||
		strings.Contains(s, "conflict"):
		sentinel = ErrMergeConflict
	}

	if sentinel != nil {
		return &GitError{
			Command:  command,
			Args:     args,
			ExitCode: exitCode,
			Stderr:   stderr,
			Err:      sentinel,
		}
	}

	if underlyingErr == nil && exitCode != 0 {
		underlyingErr = fmt.Errorf("exit code %d", exitCode)
	}

	return &GitError{
		Command:  command,
		Args:     args,
		ExitCode: exitCode,
		Stderr:   stderr,
		Err:      underlyingErr,
	}
}

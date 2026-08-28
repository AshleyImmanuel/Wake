package git

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

// Runner abstracts execution of Git CLI commands.
type Runner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout []byte, stderr []byte, err error)
}

// OSRunner executes real git commands using the host OS exec facility.
type OSRunner struct {
	gitPath string
}

// NewOSRunner creates a new OSRunner.
func NewOSRunner() *OSRunner {
	return &OSRunner{
		gitPath: findGitBinary(),
	}
}

func findGitBinary() string {
	path, err := exec.LookPath("git")
	if err == nil && path != "" {
		return path
	}
	// Fallback to standard installation locations on Windows
	for _, candidate := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		cmd := exec.Command(candidate, "--version")
		if err := cmd.Run(); err == nil {
			return candidate
		}
	}
	return ""
}

// Run executes a git command in the specified directory with the given arguments.
func (r *OSRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error) {
	if r.gitPath == "" {
		r.gitPath = findGitBinary()
		if r.gitPath == "" {
			return nil, nil, ErrGitNotFound
		}
	}

	cmd := exec.CommandContext(ctx, r.gitPath, args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	stdout := stdoutBuf.Bytes()
	stderr := stderrBuf.Bytes()

	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return stdout, stderr, ctx.Err()
		}

		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		classified := classifyGitError("git", args, exitCode, string(stderr), err)
		return stdout, stderr, classified
	}

	return stdout, stderr, nil
}

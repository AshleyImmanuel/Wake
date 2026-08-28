package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func setupTempGitRepoForStatus(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	gitBin := findGit()

	run := func(args ...string) {
		cmd := exec.Command(gitBin, args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (output: %s)", args, err, string(out))
		}
	}

	run("init")
	run("config", "user.name", "Sentinel Tester")
	run("config", "user.email", "test@sentinel.local")
	run("config", "commit.gpgsign", "false")

	filePath := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	run("add", "main.go")
	run("commit", "-m", "Initial commit")

	return tmpDir
}

func TestStatus_NoCheckpoint(t *testing.T) {
	repoDir := setupTempGitRepoForStatus(t)
	ctx := context.Background()

	// Running status on repo with no checkpoints should succeed without error
	err := runStatus(ctx, repoDir, "", false)
	if err != nil {
		t.Fatalf("runStatus failed: %v", err)
	}

	// Running status with JSON output should succeed
	err = runStatus(ctx, repoDir, "", true)
	if err != nil {
		t.Fatalf("runStatus JSON failed: %v", err)
	}
}

func TestStatus_WithCheckpoint(t *testing.T) {
	repoDir := setupTempGitRepoForStatus(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// 1. Create checkpoint
	if err := runCheckpoint(ctx, repoDir, taskID, "Test Status CLI"); err != nil {
		t.Fatalf("runCheckpoint failed: %v", err)
	}

	// 2. Run status text
	if err := runStatus(ctx, repoDir, taskID, false); err != nil {
		t.Fatalf("runStatus text failed: %v", err)
	}

	// 3. Run status JSON
	if err := runStatus(ctx, repoDir, taskID, true); err != nil {
		t.Fatalf("runStatus JSON failed: %v", err)
	}
}

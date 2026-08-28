package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/db"
)

func findGit() string {
	if p, err := exec.LookPath("git"); err == nil && p != "" {
		return p
	}
	for _, c := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return "git"
}

func setupTempGitRepo(t *testing.T) string {
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

	// Create initial file & commit
	filePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(filePath, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	run("add", "README.md")
	run("commit", "-m", "Initial commit")

	return tmpDir
}

func TestCheckpoint_RunCheckpoint(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// 1. Create first checkpoint on clean repo
	err := runCheckpoint(ctx, repoDir, taskID, "Implement Sentinel CLI")
	if err != nil {
		t.Fatalf("runCheckpoint failed: %v", err)
	}

	// Verify database record
	database, err := db.InitDB(repoDir)
	if err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	defer database.Close()

	cp, err := db.GetLatestCheckpoint(ctx, database, taskID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint failed: %v", err)
	}

	if cp.StateVersion != 1 {
		t.Errorf("expected state version 1, got %d", cp.StateVersion)
	}
	if cp.Commit == "" {
		t.Errorf("expected non-empty commit hash")
	}
	if cp.StateData.Objective != "Implement Sentinel CLI" {
		t.Errorf("expected objective 'Implement Sentinel CLI', got '%s'", cp.StateData.Objective)
	}

	// 2. Create second checkpoint (should increment state version)
	err = runCheckpoint(ctx, repoDir, taskID, "Updated Objective")
	if err != nil {
		t.Fatalf("runCheckpoint 2 failed: %v", err)
	}

	cp2, err := db.GetLatestCheckpoint(ctx, database, taskID)
	if err != nil {
		t.Fatalf("GetLatestCheckpoint 2 failed: %v", err)
	}

	if cp2.StateVersion != 2 {
		t.Errorf("expected state version 2, got %d", cp2.StateVersion)
	}
	if cp2.StateData.Objective != "Updated Objective" {
		t.Errorf("expected updated objective, got '%s'", cp2.StateData.Objective)
	}
}

func TestCheckpoint_PreCheckpointGuard_BlocksDirty(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// Create an untracked file
	untracked := filepath.Join(repoDir, "unreviewed.txt")
	if err := os.WriteFile(untracked, []byte("human changes"), 0644); err != nil {
		t.Fatalf("failed to write untracked file: %v", err)
	}

	// Attempt checkpoint without force -> must be blocked
	err := runCheckpoint(ctx, repoDir, taskID, "Checkpoint with dirty tree")
	if err == nil {
		t.Fatalf("expected pre-checkpoint guard to block checkpoint with untracked file, got nil")
	}

	// Attempt checkpoint with force -> must succeed
	err = runCheckpointWithOpts(ctx, repoDir, taskID, "Forced checkpoint", true, nil)
	if err != nil {
		t.Fatalf("expected forced checkpoint to succeed, got: %v", err)
	}
}

func TestCheckpoint_PreCheckpointGuard_TrackedFiles(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// Modify tracked file
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Updated README\n"), 0644); err != nil {
		t.Fatalf("failed to update file: %v", err)
	}

	// Tracked files include README.md -> permitted
	err := runCheckpointWithOpts(ctx, repoDir, taskID, "Tracked file checkpoint", false, []string{"README.md"})
	if err != nil {
		t.Fatalf("expected checkpoint to succeed when dirty file is in tracked-files, got: %v", err)
	}

	// Add an unreviewed file outside tracked files
	otherFile := filepath.Join(repoDir, "other.txt")
	if err := os.WriteFile(otherFile, []byte("other"), 0644); err != nil {
		t.Fatalf("failed to create other file: %v", err)
	}

	// Tracked files only include README.md -> must fail due to other.txt
	err = runCheckpointWithOpts(ctx, repoDir, taskID, "Blocked by outside tracked files", false, []string{"README.md"})
	if err == nil {
		t.Fatalf("expected checkpoint to be blocked when dirty file outside tracked scope exists, got nil")
	}
}

func TestCheckpoint_InvalidTargetDir(t *testing.T) {
	tmpDir := t.TempDir()
	nonGitDir := filepath.Join(tmpDir, "nongit")
	_ = os.MkdirAll(nonGitDir, 0755)

	ctx := context.Background()
	err := runCheckpoint(ctx, nonGitDir, "", "")
	if err == nil {
		t.Errorf("expected error running checkpoint in non-git directory")
	}
}

func TestCheckpoint_InvalidTaskID(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	err := runCheckpoint(ctx, repoDir, "not-a-valid-uuid", "")
	if err == nil {
		t.Errorf("expected error with invalid task id")
	}
}

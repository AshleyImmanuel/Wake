package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/db"
)

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	gitBin := findGit()
	cmd := exec.Command(gitBin, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s failed: %v\nOutput: %s", args, dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func TestCheckpointAdversarial_UntrackedFilesBlocked(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// 1. Initially clean repo -> Checkpoint succeeds
	err := runCheckpoint(ctx, repoDir, taskID, "Clean Initial State")
	if err != nil {
		t.Fatalf("expected checkpoint to succeed on clean repo: %v", err)
	}

	// 2. Add untracked files
	untrackedFiles := []string{
		"new_script.py",
		"notes/todo.txt",
		"sub/deep/untracked.go",
	}
	for _, rel := range untrackedFiles {
		full := filepath.Join(repoDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatalf("MkdirAll failed: %v", err)
		}
		if err := os.WriteFile(full, []byte("content"), 0644); err != nil {
			t.Fatalf("WriteFile failed: %v", err)
		}
	}

	// 3. Checkpoint must fail due to untracked files
	err = runCheckpoint(ctx, repoDir, taskID, "Attempt with untracked")
	if err == nil {
		t.Fatalf("expected checkpoint to be blocked by pre-checkpoint guard, but it succeeded")
	}
	if !strings.Contains(err.Error(), "pre-checkpoint guard blocked checkpoint") {
		t.Errorf("expected error message to mention pre-checkpoint guard, got: %v", err)
	}

	// 4. Force override must succeed
	err = runCheckpointWithOpts(ctx, repoDir, taskID, "Forced with untracked", true, nil)
	if err != nil {
		t.Fatalf("expected force=true to bypass guard: %v", err)
	}
}

func TestCheckpointAdversarial_StagedAndUnstagedModificationsBlocked(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// Create and commit initial files
	f1 := filepath.Join(repoDir, "file1.txt")
	f2 := filepath.Join(repoDir, "file2.txt")
	_ = os.WriteFile(f1, []byte("v1"), 0644)
	_ = os.WriteFile(f2, []byte("v1"), 0644)
	runGitCmd(t, repoDir, "add", "file1.txt", "file2.txt")
	runGitCmd(t, repoDir, "commit", "-m", "add files")

	// Case A: Unstaged modification
	_ = os.WriteFile(f1, []byte("v2-unstaged"), 0644)
	err := runCheckpoint(ctx, repoDir, taskID, "Unstaged mod test")
	if err == nil {
		t.Fatalf("expected guard to block unstaged modification")
	}

	// Revert f1, stage f2 modification
	_ = os.WriteFile(f1, []byte("v1"), 0644)
	_ = os.WriteFile(f2, []byte("v2-staged"), 0644)
	runGitCmd(t, repoDir, "add", "file2.txt")

	// Case B: Staged modification
	err = runCheckpoint(ctx, repoDir, taskID, "Staged mod test")
	if err == nil {
		t.Fatalf("expected guard to block staged modification")
	}

	// Case C: Staged deletion
	runGitCmd(t, repoDir, "rm", "file1.txt")
	err = runCheckpoint(ctx, repoDir, taskID, "Staged rm test")
	if err == nil {
		t.Fatalf("expected guard to block staged deletion")
	}

	// Force override must work on all staged/unstaged changes
	err = runCheckpointWithOpts(ctx, repoDir, taskID, "Forced staged/unstaged", true, nil)
	if err != nil {
		t.Fatalf("expected force override to succeed on staged/unstaged changes: %v", err)
	}
}

func TestCheckpointAdversarial_MetadataIsolation(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// 1. Create .wake and .sentinel internal files
	wakeDir := filepath.Join(repoDir, ".wake")
	sentinelDir := filepath.Join(repoDir, ".sentinel")
	_ = os.MkdirAll(wakeDir, 0755)
	_ = os.MkdirAll(sentinelDir, 0755)
	_ = os.WriteFile(filepath.Join(wakeDir, "events.log"), []byte("log data"), 0644)
	_ = os.WriteFile(filepath.Join(sentinelDir, "custom.json"), []byte("{}"), 0644)

	// Guard must NOT block internal metadata files!
	err := runCheckpoint(ctx, repoDir, taskID, "Checkpoint with internal metadata")
	if err != nil {
		t.Fatalf("guard should NOT block internal .wake/.sentinel files, got error: %v", err)
	}

	// 2. Add non-metadata hidden file like .gitignore or .env
	envFile := filepath.Join(repoDir, ".env")
	_ = os.WriteFile(envFile, []byte("SECRET=123"), 0644)

	// Guard MUST block .env
	err = runCheckpoint(ctx, repoDir, taskID, "Checkpoint with .env")
	if err == nil {
		t.Fatalf("guard should block non-metadata dotfiles like .env")
	}

	// 3. Add .gitignore
	_ = os.Remove(envFile)
	gitIgnore := filepath.Join(repoDir, ".gitignore")
	_ = os.WriteFile(gitIgnore, []byte("*.tmp\n"), 0644)

	// Guard MUST block .gitignore
	err = runCheckpoint(ctx, repoDir, taskID, "Checkpoint with .gitignore")
	if err == nil {
		t.Fatalf("guard should block uncommitted .gitignore modifications")
	}
}

func TestCheckpointAdversarial_TrackedFilesScoping(t *testing.T) {
	repoDir := setupTempGitRepo(t)
	ctx := context.Background()
	taskID := uuid.New().String()

	// Modify two distinct files
	f1 := filepath.Join(repoDir, "pkg/auth/login.go")
	f2 := filepath.Join(repoDir, "pkg/billing/invoice.go")
	_ = os.MkdirAll(filepath.Dir(f1), 0755)
	_ = os.MkdirAll(filepath.Dir(f2), 0755)
	_ = os.WriteFile(f1, []byte("package auth\n"), 0644)
	_ = os.WriteFile(f2, []byte("package billing\n"), 0644)

	// Tracked files covers only auth -> billing should trigger block
	err := runCheckpointWithOpts(ctx, repoDir, taskID, "Partial tracked", false, []string{"pkg/auth/*"})
	if err == nil {
		t.Fatalf("expected guard block because billing/invoice.go is outside tracked scope")
	}

	// Tracked files covers both -> must succeed
	err = runCheckpointWithOpts(ctx, repoDir, taskID, "Full tracked", false, []string{"pkg/auth/*", "pkg/billing/*"})
	if err != nil {
		t.Fatalf("expected checkpoint to succeed when all modified files are in tracked scope: %v", err)
	}

	// Verify Checkpoint persisted in database
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
}

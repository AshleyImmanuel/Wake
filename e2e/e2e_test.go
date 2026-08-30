package e2e

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AshleyImmanuel/Wake/internal/testutil"
)

var wakeBin string

func TestMain(m *testing.M) {
	// Build the wake binary for e2e tests
	tmpDir, err := os.MkdirTemp("", "wake-e2e-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	wakeBin = filepath.Join(tmpDir, "wake.exe")
	cmd := exec.Command("go", "build", "-o", wakeBin, "../main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("failed to build wake binary: " + string(out))
	}

	os.Exit(m.Run())
}

func runWake(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(wakeBin, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("wake %v failed: %v\nOutput: %s", args, err, out.String())
	}
	return out.String()
}

func TestE2E_InitAndCheckpoint(t *testing.T) {
	repoDir := testutil.SetupTempRepo(t)

	// 1. Init workspace
	out := runWake(t, repoDir, "init")
	if !strings.Contains(out, "Successfully initialized Wake workspace") {
		t.Errorf("expected success message, got: %s", out)
	}

	// 2. Status should be empty/no checkpoint
	out = runWake(t, repoDir, "status")
	if !strings.Contains(out, "NO CHECKPOINT FOUND") {
		t.Errorf("expected NO CHECKPOINT FOUND, got: %s", out)
	}

	// 3. Make a change, record an event
	testFile := filepath.Join(repoDir, "test.txt")
	os.WriteFile(testFile, []byte("hello"), 0644)
	
	// Track the file
	cmd := exec.Command("git", "add", "test.txt")
	cmd.Dir = repoDir
	cmd.Run()

	// Create checkpoint (using --force to bypass human guard for uncommitted test file)
	out = runWake(t, repoDir, "checkpoint", "--objective", "Test e2e", "--force")
	if !strings.Contains(out, "Checkpoint created") {
		t.Errorf("expected checkpoint creation message, got: %s", out)
	}
}

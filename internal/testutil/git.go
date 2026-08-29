package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// SetupTempRepo creates a temporary directory, initializes it as a git repository,
// creates an initial commit so that there is a HEAD, and returns the path.
func SetupTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	// Create an initial file and commit it
	readmePath := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test Repo\n"), 0600); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "Initial commit")

	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	// #nosec G204
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, out)
	}
}

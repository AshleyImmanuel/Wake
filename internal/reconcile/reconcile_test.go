package reconcile

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
)

// gitTestRepo encapsulates an isolated Git repository initialized in a temporary directory.
type gitTestRepo struct {
	dir     string
	gitPath string
	t       *testing.T
	client  git.Client
}

// locateGitBinary searches for the git executable across PATH and standard Windows locations.
func locateGitBinary() string {
	if path, err := exec.LookPath("git"); err == nil && path != "" {
		return path
	}
	for _, candidate := range []string{
		`C:\Program Files\Git\cmd\git.exe`,
		`C:\Program Files\Git\bin\git.exe`,
		`C:\Program Files (x86)\Git\cmd\git.exe`,
		`C:\Program Files (x86)\Git\bin\git.exe`,
	} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "git"
}

// initGitTestRepo initializes a real, isolated Git repository with test identity configured.
func initGitTestRepo(t *testing.T) *gitTestRepo {
	t.Helper()
	tmpDir := t.TempDir()
	gitBin := locateGitBinary()

	repo := &gitTestRepo{
		dir:     tmpDir,
		gitPath: gitBin,
		t:       t,
		client:  git.NewClient(nil),
	}

	if _, err := repo.runGitAllowError("init", "-b", "main"); err != nil {
		repo.runGit("init")
	}
	repo.runGit("config", "user.name", "Sentinel Tester")
	repo.runGit("config", "user.email", "test@sentinel.local")
	repo.runGit("config", "commit.gpgsign", "false")

	return repo
}

// runGit executes a git command in the repository directory and returns standard output.
func (r *gitTestRepo) runGit(args ...string) string {
	r.t.Helper()
	cmd := exec.Command(r.gitPath, args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v failed in %s: %v (output: %s)", args, r.dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

// runGitAllowError executes a git command where non-zero exit code is expected (e.g. merge conflict).
func (r *gitTestRepo) runGitAllowError(args ...string) (string, error) {
	r.t.Helper()
	cmd := exec.Command(r.gitPath, args...)
	cmd.Dir = r.dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// writeFile writes content to a relative file path inside the test repository.
func (r *gitTestRepo) writeFile(relPath, content string) {
	r.t.Helper()
	fullPath := filepath.Join(r.dir, filepath.FromSlash(relPath))
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		r.t.Fatalf("failed to create directory %s: %v", dir, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		r.t.Fatalf("failed to write file %s: %v", fullPath, err)
	}
}

// deleteFile removes a file from the test repository filesystem.
func (r *gitTestRepo) deleteFile(relPath string) {
	r.t.Helper()
	fullPath := filepath.Join(r.dir, filepath.FromSlash(relPath))
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		r.t.Fatalf("failed to delete file %s: %v", fullPath, err)
	}
}

// commitAll stages all working tree changes and creates a new Git commit, returning the commit hash.
func (r *gitTestRepo) commitAll(message string) string {
	r.t.Helper()
	r.runGit("add", "-A")
	r.runGit("commit", "-m", message)
	return r.currentCommit()
}

// currentCommit returns the full commit hash of HEAD.
func (r *gitTestRepo) currentCommit() string {
	r.t.Helper()
	return r.runGit("rev-parse", "HEAD")
}

// currentBranch returns the current active branch name.
func (r *gitTestRepo) currentBranch() string {
	r.t.Helper()
	branch := r.runGit("rev-parse", "--abbrev-ref", "HEAD")
	return branch
}

// createBranch creates and checks out a new branch.
func (r *gitTestRepo) createBranch(branchName string) {
	r.t.Helper()
	r.runGit("checkout", "-b", branchName)
}

// checkoutBranch switches to an existing branch.
func (r *gitTestRepo) checkoutBranch(branchName string) {
	r.t.Helper()
	r.runGit("checkout", branchName)
}

// makeTestCheckpoint constructs a state.Checkpoint for verification suite tests.
func makeTestCheckpoint(repo *gitTestRepo, commit string) state.Checkpoint {
	taskID := uuid.New()
	return state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    repo.dir,
		Branch:        repo.currentBranch(),
		Commit:        commit,
		StateVersion:  1,
		EventPosition: 1,
		StateData: state.State{
			TaskID:       taskID,
			Objective:    "Reconciliation Verification Suite",
			Constraints:  make([]string, 0),
			Decisions:    make([]state.Decision, 0),
			Completed:    make([]string, 0),
			Remaining:    make([]string, 0),
			Blocked:      make([]state.Blocker, 0),
			DoNotRepeat:  make([]string, 0),
			LastVerified: commit,
			Confidence:   state.ConfidenceHigh,
		},
	}
}

// =========================================================================================
// TEST 1: TestReconciliationSuite_SAFE
// Acceptance Criteria: Verify that the reconciliation engine correctly returns SAFE when
// the simulated repository exactly matches the checkpoint commit with no uncommitted changes.
// =========================================================================================
func TestReconciliationSuite_SAFE(t *testing.T) {
	repo := initGitTestRepo(t)

	// Set up repository with initial clean files
	repo.writeFile("pkg/service.go", "package service\n\nfunc Run() string { return \"ok\" }\n")
	repo.writeFile("README.md", "# Sentinel Project\n")
	commitHash := repo.commitAll("Initial project setup")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.StateData.Constraints = []string{"auth/*"}
	cp.StateData.Completed = []string{"README.md"}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, []string{"pkg/service.go"})
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusSafe {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusSafe, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceHigh {
		t.Errorf("expected ConfidenceHigh, got %s", result.ConfidenceLevel)
	}
	if !result.BranchMatch {
		t.Errorf("expected BranchMatch to be true")
	}
	if len(result.ChangedFiles) != 0 {
		t.Errorf("expected 0 changed files, got %d: %v", len(result.ChangedFiles), result.ChangedFiles)
	}
	if len(result.ConstraintViolations) != 0 {
		t.Errorf("expected 0 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
	if len(result.InvalidatedClaims) != 0 {
		t.Errorf("expected 0 invalidated claims, got %d: %v", len(result.InvalidatedClaims), result.InvalidatedClaims)
	}
}

// =========================================================================================
// TEST 2: TestReconciliationSuite_STALE_ForwardCommits
// Acceptance Criteria: Verify STALE status when commits are added ahead of the checkpoint commit.
// =========================================================================================
func TestReconciliationSuite_STALE_ForwardCommits(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("core/engine.go", "package core\n\nfunc Init() {}\n")
	cpCommit := repo.commitAll("Initial core commit")

	cp := makeTestCheckpoint(repo, cpCommit)

	// Advance repository with forward commits
	repo.writeFile("core/utils.go", "package core\n\nfunc Helper() {}\n")
	forwardCommit := repo.commitAll("Forward commit: Added helper utilities")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusStale {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusStale, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceLow {
		t.Errorf("expected ConfidenceLow, got %s", result.ConfidenceLevel)
	}
	if result.CheckpointCommit != cpCommit {
		t.Errorf("expected CheckpointCommit %s, got %s", cpCommit, result.CheckpointCommit)
	}
	if result.CurrentCommit != forwardCommit {
		t.Errorf("expected CurrentCommit %s, got %s", forwardCommit, result.CurrentCommit)
	}
	if len(result.ConstraintViolations) != 0 {
		t.Errorf("expected 0 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
	if len(result.InvalidatedClaims) != 0 {
		t.Errorf("expected 0 invalidated claims, got %d: %v", len(result.InvalidatedClaims), result.InvalidatedClaims)
	}
}

// =========================================================================================
// TEST 3: TestReconciliationSuite_STALE_TaskFilesModified
// Acceptance Criteria: Verify STALE status when task-related files are modified in working
// tree without violating constraints.
// =========================================================================================
func TestReconciliationSuite_STALE_TaskFilesModified(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("tasks/handler.go", "package tasks\n\nfunc Handle() {}\n")
	repo.writeFile("protected/system.go", "package protected\n\nfunc Lock() {}\n")
	commitHash := repo.commitAll("Base task commit")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.StateData.Constraints = []string{"protected/*"}

	// Modify task-related file in working tree (uncommitted)
	repo.writeFile("tasks/handler.go", "package tasks\n\nfunc Handle() { /* modified */ }\n")

	taskFiles := []string{"tasks/handler.go"}
	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, taskFiles)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusStale {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusStale, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceLow {
		t.Errorf("expected ConfidenceLow, got %s", result.ConfidenceLevel)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0] != "tasks/handler.go" {
		t.Errorf("expected changed files ['tasks/handler.go'], got %v", result.ChangedFiles)
	}
	if len(result.TaskRelatedChanges) != 1 || result.TaskRelatedChanges[0] != "tasks/handler.go" {
		t.Errorf("expected task-related changes ['tasks/handler.go'], got %v", result.TaskRelatedChanges)
	}
	if len(result.ConstraintViolations) != 0 {
		t.Errorf("expected 0 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
}

// =========================================================================================
// TEST 4: TestReconciliationSuite_CONFLICT_ConstraintViolation
// Acceptance Criteria: Verify CONFLICT status when simulated task-related files protected
// by constraints have been manually modified since the checkpoint.
// =========================================================================================
func TestReconciliationSuite_CONFLICT_ConstraintViolation(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("auth/jwt.go", "package auth\n\nfunc Verify() bool { return true }\n")
	repo.writeFile("db/schema.sql", "CREATE TABLE users (id INT);\n")
	repo.writeFile("worker/job.go", "package worker\n\nfunc Work() {}\n")
	commitHash := repo.commitAll("Initial setup with protected auth and db")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.StateData.Constraints = []string{"Do not touch auth", "db/*"}

	// Modify protected files in working tree
	repo.writeFile("auth/jwt.go", "package auth\n\nfunc Verify() bool { return false /* compromised */ }\n")
	repo.writeFile("db/schema.sql", "CREATE TABLE users (id INT, role TEXT);\n")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, []string{"auth/jwt.go"})
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusConflict, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.ConstraintViolations) < 2 {
		t.Fatalf("expected at least 2 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
}

// =========================================================================================
// TEST 5: TestReconciliationSuite_CONFLICT_DecisionViolation
// Acceptance Criteria: Verify CONFLICT status when files governed by active decisions are modified.
// =========================================================================================
func TestReconciliationSuite_CONFLICT_DecisionViolation(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("config/prod.json", `{"env": "production", "port": 8080}`)
	repo.writeFile("legacy/old_module.go", "package legacy\n\nfunc Old() {}\n")
	commitHash := repo.commitAll("Base configuration")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.StateData.Decisions = []state.Decision{
		{
			ID:          "DEC-001",
			Description: "Protect config/prod.json from manual edits",
			Status:      "ACTIVE",
		},
		{
			ID:          "DEC-002",
			Description: "Deprecate legacy/old_module.go",
			Status:      "REJECTED", // Rejected decisions should not trigger violations
		},
	}

	// Modify file protected by ACTIVE decision
	repo.writeFile("config/prod.json", `{"env": "production", "port": 9090}`)
	// Modify file covered by REJECTED decision
	repo.writeFile("legacy/old_module.go", "package legacy\n\nfunc Old() { /* altered */ }\n")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusConflict, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.ConstraintViolations) != 1 {
		t.Fatalf("expected exactly 1 active decision violation, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
	if !strings.Contains(result.ConstraintViolations[0], "DEC-001") && !strings.Contains(result.ConstraintViolations[0], "config/prod.json") {
		t.Errorf("expected violation mentioning DEC-001 or config/prod.json, got: %s", result.ConstraintViolations[0])
	}
}

// =========================================================================================
// TEST 6: TestReconciliationSuite_CONFLICT_DeletedMilestoneArtifact
// Acceptance Criteria: Verify CONFLICT status when completed/do-not-repeat milestone files are deleted.
// =========================================================================================
func TestReconciliationSuite_CONFLICT_DeletedMilestoneArtifact(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("milestones/schema.sql", "CREATE TABLE milestones (id INT);\n")
	repo.writeFile("milestones/seed.sql", "INSERT INTO milestones VALUES (1);\n")
	repo.writeFile("app.go", "package main\n\nfunc main() {}\n")
	commitHash := repo.commitAll("Milestone 1 completed")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.StateData.Completed = []string{"milestones/schema.sql"}
	cp.StateData.DoNotRepeat = []string{"milestones/seed.sql"}

	// Physically remove completed milestone files from disk
	repo.deleteFile("milestones/schema.sql")
	repo.deleteFile("milestones/seed.sql")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusConflict, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.InvalidatedClaims) < 2 {
		t.Fatalf("expected at least 2 invalidated claims for deleted milestone files, got %d: %v", len(result.InvalidatedClaims), result.InvalidatedClaims)
	}
}

// =========================================================================================
// TEST 7: TestReconciliationSuite_CONFLICT_MergeConflicts
// Acceptance Criteria: Verify CONFLICT status when unresolved merge conflicts exist.
// =========================================================================================
func TestReconciliationSuite_CONFLICT_MergeConflicts(t *testing.T) {
	repo := initGitTestRepo(t)

	// 1. Initial base commit
	repo.writeFile("service/config.go", "package service\n\nvar Mode = \"base\"\n")
	baseCommit := repo.commitAll("Initial base config")
	baseBranch := repo.currentBranch()

	// 2. Create feature branch and modify file
	repo.createBranch("feature-branch")
	repo.writeFile("service/config.go", "package service\n\nvar Mode = \"feature\"\n")
	repo.commitAll("Feature branch edit")

	// 3. Switch back to base branch and make conflicting edit
	repo.checkoutBranch(baseBranch)

	repo.writeFile("service/config.go", "package service\n\nvar Mode = \"main-override\"\n")
	cpCommit := repo.commitAll("Main branch edit")

	cp := makeTestCheckpoint(repo, cpCommit)

	// 4. Trigger merge conflict by merging feature-branch
	_, _ = repo.runGitAllowError("merge", "feature-branch")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusConflict, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if !strings.Contains(strings.ToLower(result.Reason), "merge conflict") {
		t.Errorf("expected reason mentioning merge conflicts, got '%s'", result.Reason)
	}
	_ = baseCommit
}

// =========================================================================================
// Supplementary Test: TestReconciliationSuite_BranchMismatch
// =========================================================================================
func TestReconciliationSuite_BranchMismatch(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("app.go", "package main\n\nfunc main() {}\n")
	commitHash := repo.commitAll("Initial commit")

	cp := makeTestCheckpoint(repo, commitHash)
	cp.Branch = "release-1.0" // Checkpoint was created on release branch

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusStale {
		t.Fatalf("expected status %s for branch mismatch, got %s", StatusStale, result.Status)
	}
	if result.BranchMatch {
		t.Errorf("expected BranchMatch to be false")
	}
}

// =========================================================================================
// Supplementary Test: TestReconciliationSuite_DivergedHistory
// =========================================================================================
func TestReconciliationSuite_DivergedHistory(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("shared.go", "package main\n")
	repo.commitAll("Base commit")
	baseBranch := repo.currentBranch()

	// Branch A
	repo.createBranch("branch-a")
	repo.writeFile("a.go", "package main\n")
	commitA := repo.commitAll("Branch A commit")

	// Checkpoint is recorded on commitA
	cp := makeTestCheckpoint(repo, commitA)

	// Branch B (diverges from base)
	repo.checkoutBranch(baseBranch)
	repo.createBranch("branch-b")
	repo.writeFile("b.go", "package main\n")
	repo.commitAll("Branch B commit")

	// Reconcile branch-b repository state against cp from branch-a
	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status for diverged history, got %s (reason: %s)", result.Status, result.Reason)
	}
	if !strings.Contains(result.Reason, "diverged") {
		t.Errorf("expected reason to mention diverged history, got '%s'", result.Reason)
	}
}

// =========================================================================================
// Supplementary Test: TestReconciliationSuite_UntrackedFiles
// =========================================================================================
func TestReconciliationSuite_UntrackedFiles(t *testing.T) {
	repo := initGitTestRepo(t)

	repo.writeFile("app.go", "package main\n\nfunc main() {}\n")
	commitHash := repo.commitAll("Clean commit")

	cp := makeTestCheckpoint(repo, commitHash)

	// Add untracked non-conflicting file
	repo.writeFile("notes/scratch.txt", "some temporary scratchpad notes")

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, repo.client, repo.dir, nil)
	if err != nil {
		t.Fatalf("ReconcileRepo failed: %v", err)
	}

	if result.Status != StatusStale {
		t.Fatalf("expected STALE status when untracked files are present, got %s", result.Status)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0] != "notes/scratch.txt" {
		t.Errorf("expected changed files ['notes/scratch.txt'], got %v", result.ChangedFiles)
	}
}

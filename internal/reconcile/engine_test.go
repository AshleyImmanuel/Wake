package reconcile

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/state"
)

// Helper function to create a base checkpoint for testing
func newTestCheckpoint(commit, branch string) state.Checkpoint {
	return state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        uuid.New(),
		Timestamp:     "2026-08-28T12:00:00Z",
		Repository:    "github.com/wake/wake",
		Branch:        branch,
		Commit:        commit,
		StateVersion:  1,
		EventPosition: 5,
		StateData: state.State{
			TaskID:       uuid.New(),
			Objective:    "Implement Sentinel Reconciliation",
			Constraints:  make([]string, 0),
			Decisions:    make([]state.Decision, 0),
			Completed:    make([]string, 0),
			DoNotRepeat:  make([]string, 0),
			LastVerified: commit,
			Confidence:   state.ConfidenceHigh,
		},
	}
}

// 1. Test SAFE evaluation when commit matches and repo is clean.
func TestReconcile_SAFE_MatchingCommitAndClean(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           true,
		HasMergeConflicts: false,
		StagedFiles:       []git.FileStatus{},
		UnstagedFiles:     []git.FileStatus{},
		UntrackedFiles:    []string{},
		UnmergedFiles:     []string{},
		ModifiedFiles:     []string{},
	}

	result := Reconcile(cp, repo, []string{"internal/reconcile/engine.go"})

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
		t.Errorf("expected 0 changed files, got %d", len(result.ChangedFiles))
	}
	if len(result.ConstraintViolations) != 0 {
		t.Errorf("expected 0 constraint violations, got %d", len(result.ConstraintViolations))
	}
}

// 2. Test STALE evaluation when forward commits exist without constraint violations.
func TestReconcile_STALE_ForwardCommits(t *testing.T) {
	commit1 := "1111111111111111111111111111111111111111"
	commit2 := "2222222222222222222222222222222222222222"
	cp := newTestCheckpoint(commit1, "main")

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit2,
		HasCommits:        true,
		IsClean:           true,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusStale {
		t.Fatalf("expected status %s, got %s", StatusStale, result.Status)
	}
	if result.ConfidenceLevel != state.ConfidenceLow {
		t.Errorf("expected ConfidenceLow, got %s", result.ConfidenceLevel)
	}
	if result.CurrentCommit != commit2 {
		t.Errorf("expected current commit %s, got %s", commit2, result.CurrentCommit)
	}
	if result.CheckpointCommit != commit1 {
		t.Errorf("expected checkpoint commit %s, got %s", commit1, result.CheckpointCommit)
	}
}

// 3. Test STALE evaluation when non-conflicting modified files exist.
func TestReconcile_STALE_NonConflictingModifications(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Constraints = []string{"auth/*"}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{"billing/invoice.go", "docs/readme.md"},
	}

	taskFiles := []string{"billing/invoice.go"}
	result := Reconcile(cp, repo, taskFiles)

	if result.Status != StatusStale {
		t.Fatalf("expected status %s, got %s (reason: %s)", StatusStale, result.Status, result.Reason)
	}
	if result.ConfidenceLevel != state.ConfidenceLow {
		t.Errorf("expected ConfidenceLow, got %s", result.ConfidenceLevel)
	}
	if len(result.ChangedFiles) != 2 {
		t.Fatalf("expected 2 changed files, got %d", len(result.ChangedFiles))
	}
	if len(result.TaskRelatedChanges) != 1 || result.TaskRelatedChanges[0] != "billing/invoice.go" {
		t.Errorf("expected TaskRelatedChanges to contain 'billing/invoice.go', got %v", result.TaskRelatedChanges)
	}
	if len(result.UnrelatedChanges) != 1 || result.UnrelatedChanges[0] != "docs/readme.md" {
		t.Errorf("expected UnrelatedChanges to contain 'docs/readme.md', got %v", result.UnrelatedChanges)
	}
	if len(result.ConstraintViolations) != 0 {
		t.Errorf("expected 0 constraint violations, got %d", len(result.ConstraintViolations))
	}
}

// Test STALE evaluation when untracked files exist.
func TestReconcile_STALE_UntrackedFiles(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")

	repo := git.RepositoryState{
		RootPath:       "/workspace",
		Branch:         "main",
		CommitHash:     commit,
		HasCommits:     true,
		IsClean:        false,
		UntrackedFiles: []string{"temp/notes.txt"},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusStale {
		t.Fatalf("expected status %s for untracked files, got %s", StatusStale, result.Status)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0] != "temp/notes.txt" {
		t.Errorf("expected changed files to contain 'temp/notes.txt', got %v", result.ChangedFiles)
	}
}

// 4. Test CONFLICT evaluation when constraint file is modified.
func TestReconcile_CONFLICT_ConstraintViolation(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Constraints = []string{"Do not touch auth", "internal/db/*"}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{"auth/session.go", "internal/db/db.go"},
	}

	result := Reconcile(cp, repo, []string{"auth/session.go"})

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.ConstraintViolations) < 2 {
		t.Fatalf("expected at least 2 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
}

// 5. Test CONFLICT evaluation when decision file is modified.
func TestReconcile_CONFLICT_DecisionViolation(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Decisions = []state.Decision{
		{
			ID:          "DEC-001",
			Description: "Protect config/settings.json from changes",
			Status:      "ACTIVE",
		},
		{
			ID:          "DEC-002",
			Description: "Do not modify legacy pkg/old",
			Status:      "REJECTED", // Rejected decision should not trigger
		},
		{
			ID:          "DEC-003",
			Description: "", // Empty description
			Status:      "ACTIVE",
		},
	}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{"config/settings.json", "pkg/old/file.go"},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.ConstraintViolations) != 1 {
		t.Fatalf("expected 1 active decision violation, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
}

// 6. Test CONFLICT evaluation when completed/do-not-repeat file is modified or deleted.
func TestReconcile_CONFLICT_CompletedOrDoNotRepeatModified(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Completed = []string{"schema/migration.sql", "Created service/auth.go"}
	cp.StateData.DoNotRepeat = []string{"internal/git/parser.go", "Do not alter db/seed.sql"}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{"schema/migration.sql", "internal/git/parser.go", "service/auth.go", "db/seed.sql"},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if len(result.InvalidatedClaims) < 4 {
		t.Fatalf("expected at least 4 invalidated claims, got %d: %v", len(result.InvalidatedClaims), result.InvalidatedClaims)
	}
}

// Test CONFLICT evaluation when completed file is marked deleted in git status.
func TestReconcile_CONFLICT_CompletedDeleted(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Completed = []string{"schema/migration.sql"}
	cp.StateData.DoNotRepeat = []string{"pkg/legacy.go"}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		UnstagedFiles: []git.FileStatus{
			{
				Path:           "schema/migration.sql",
				WorkTreeStatus: git.StatusDeleted,
			},
		},
		StagedFiles: []git.FileStatus{
			{
				Path:          "pkg/legacy.go",
				StagingStatus: git.StatusDeleted,
			},
		},
		ModifiedFiles: []string{"schema/migration.sql", "pkg/legacy.go"},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if len(result.InvalidatedClaims) < 2 {
		t.Errorf("expected at least 2 invalidated claims for deleted files, got %d: %v", len(result.InvalidatedClaims), result.InvalidatedClaims)
	}
}

// 7. Test CONFLICT evaluation on merge conflicts.
func TestReconcile_CONFLICT_MergeConflicts(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: true,
		UnmergedFiles:     []string{"cmd/root.go"},
		ModifiedFiles:     []string{"cmd/root.go"},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if result.ConfidenceLevel != state.ConfidenceNone {
		t.Errorf("expected ConfidenceNone, got %s", result.ConfidenceLevel)
	}
	if result.Reason != "Working tree has unresolved merge conflicts" {
		t.Errorf("unexpected reason: %s", result.Reason)
	}
}

// 8. Test edge cases: empty repo, empty checkpoint commit, branch mismatch, path prefix variations.
func TestReconcile_EdgeCases_EmptyRepo(t *testing.T) {
	cp := newTestCheckpoint("commit123", "main")

	repo := git.RepositoryState{
		RootPath:   "/workspace",
		Branch:     "main",
		CommitHash: "",
		HasCommits: false,
		IsClean:    true,
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s when repo has no commits but checkpoint has commit, got %s", StatusConflict, result.Status)
	}
}

func TestReconcile_EdgeCases_EmptyCheckpointCommit(t *testing.T) {
	cp := newTestCheckpoint("", "main")

	repo := git.RepositoryState{
		RootPath:   "/workspace",
		Branch:     "main",
		CommitHash: "commit123",
		HasCommits: true,
		IsClean:    true,
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusStale {
		t.Fatalf("expected status %s when checkpoint commit is empty, got %s", StatusStale, result.Status)
	}
}

func TestReconcile_EdgeCases_BranchMismatch(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "feature/auth")

	repo := git.RepositoryState{
		RootPath:   "/workspace",
		Branch:     "main",
		CommitHash: commit,
		HasCommits: true,
		IsClean:    true,
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusStale {
		t.Fatalf("expected status %s on branch mismatch, got %s", StatusStale, result.Status)
	}
	if result.BranchMatch {
		t.Errorf("expected BranchMatch to be false")
	}
}

func TestReconcile_EdgeCases_BranchHeadCompatibility(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "HEAD")

	repo := git.RepositoryState{
		RootPath:   "/workspace",
		Branch:     "main",
		CommitHash: commit,
		HasCommits: true,
		IsClean:    true,
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusSafe {
		t.Fatalf("expected status %s when checkpoint branch is HEAD, got %s", StatusSafe, result.Status)
	}
	if !result.BranchMatch {
		t.Errorf("expected BranchMatch to be true")
	}
}

func TestReconcile_EdgeCases_PathPrefixAndNormalization(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Constraints = []string{`.\auth\session.go`, "internal/git/*", "  ", ""}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		ModifiedFiles:     []string{"auth/session.go", `internal\git\client.go`},
	}

	result := Reconcile(cp, repo, []string{`.\auth\session.go`})

	if result.Status != StatusConflict {
		t.Fatalf("expected status %s, got %s", StatusConflict, result.Status)
	}
	if len(result.ConstraintViolations) < 2 {
		t.Errorf("expected 2 constraint violations, got %d: %v", len(result.ConstraintViolations), result.ConstraintViolations)
	}
	if len(result.TaskRelatedChanges) != 1 || result.TaskRelatedChanges[0] != "auth/session.go" {
		t.Errorf("expected TaskRelatedChanges to be normalized 'auth/session.go', got %v", result.TaskRelatedChanges)
	}
}

func TestReconcile_StagedRenamedFiles(t *testing.T) {
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Constraints = []string{"legacy/old.go"}

	repo := git.RepositoryState{
		RootPath:          "/workspace",
		Branch:            "main",
		CommitHash:        commit,
		HasCommits:        true,
		IsClean:           false,
		HasMergeConflicts: false,
		StagedFiles: []git.FileStatus{
			{
				OrigPath:      "legacy/old.go",
				Path:          "modern/new.go",
				StagingStatus: git.StatusRenamed,
			},
		},
	}

	result := Reconcile(cp, repo, nil)

	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status when renamed orig_path violates constraint, got %s", result.Status)
	}
}

// Test Engine interface constructor and method
func TestNewEngine(t *testing.T) {
	engine := NewEngine()
	if engine == nil {
		t.Fatalf("expected non-nil Engine from NewEngine()")
	}

	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	repo := git.RepositoryState{
		Branch:     "main",
		CommitHash: commit,
		HasCommits: true,
		IsClean:    true,
	}

	result := engine.Reconcile(cp, repo, nil)
	if result.Status != StatusSafe {
		t.Errorf("expected SAFE status, got %s", result.Status)
	}
}

// Test ReconcileRepo with live temp directory and missing claimed file check
func TestReconcileRepo_MissingClaimedFile(t *testing.T) {
	tmpDir := t.TempDir()
	commit := "a1b2c3d4e5f67890123456789abcdef012345678"
	cp := newTestCheckpoint(commit, "main")
	cp.StateData.Completed = []string{"missing_file.txt"}

	// Mock git client returns clean state with matching commit
	mockClient := &mockGitClient{
		state: &git.RepositoryState{
			RootPath:   tmpDir,
			Branch:     "main",
			CommitHash: commit,
			HasCommits: true,
			IsClean:    true,
		},
		commitExists: true,
		isAncestor:   true,
	}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error from ReconcileRepo: %v", err)
	}
	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status for missing claimed file, got %s", result.Status)
	}
	if len(result.InvalidatedClaims) == 0 {
		t.Errorf("expected invalidated claims for missing file")
	}

	// Now create the file on disk
	filePath := filepath.Join(tmpDir, "missing_file.txt")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	result2, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error from ReconcileRepo: %v", err)
	}
	if result2.Status != StatusSafe {
		t.Fatalf("expected SAFE status when file exists and repo is clean, got %s (reason: %s)", result2.Status, result2.Reason)
	}
}

// Test ReconcileRepo when commit has diverged
func TestReconcileRepo_DivergedCommit(t *testing.T) {
	tmpDir := t.TempDir()
	cpCommit := "1111111111111111111111111111111111111111"
	repoCommit := "2222222222222222222222222222222222222222"
	cp := newTestCheckpoint(cpCommit, "main")

	mockClient := &mockGitClient{
		state: &git.RepositoryState{
			RootPath:   tmpDir,
			Branch:     "main",
			CommitHash: repoCommit,
			HasCommits: true,
			IsClean:    true,
		},
		commitExists: true,
		isAncestor:   false, // History diverged
	}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status when commit history diverged, got %s", result.Status)
	}
}

// Test ReconcileRepo when commit does not exist locally
func TestReconcileRepo_CommitDoesNotExist(t *testing.T) {
	tmpDir := t.TempDir()
	cpCommit := "nonexistentcommit111111111111111111111111"
	repoCommit := "2222222222222222222222222222222222222222"
	cp := newTestCheckpoint(cpCommit, "main")

	mockClient := &mockGitClient{
		state: &git.RepositoryState{
			RootPath:   tmpDir,
			Branch:     "main",
			CommitHash: repoCommit,
			HasCommits: true,
			IsClean:    true,
		},
		commitExists: false, // Commit does not exist
	}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status when checkpoint commit does not exist, got %s", result.Status)
	}
}

// Test ReconcileRepo with commit-level changed files
func TestReconcileRepo_CommittedChangedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	cpCommit := "1111111111111111111111111111111111111111"
	repoCommit := "2222222222222222222222222222222222222222"
	cp := newTestCheckpoint(cpCommit, "main")
	cp.StateData.Constraints = []string{"protected/*"}

	mockClient := &mockGitClient{
		state: &git.RepositoryState{
			RootPath:   tmpDir,
			Branch:     "main",
			CommitHash: repoCommit,
			HasCommits: true,
			IsClean:    true,
		},
		commitExists: true,
		isAncestor:   true,
		changedFiles: []string{"protected/secret.key"},
	}

	ctx := context.Background()
	result, err := ReconcileRepo(ctx, cp, mockClient, tmpDir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusConflict {
		t.Fatalf("expected CONFLICT status when committed changes between commits violate constraint, got %s", result.Status)
	}
	if len(result.ConstraintViolations) == 0 {
		t.Errorf("expected constraint violations from committed changes")
	}
}

// Test ReconcileRepo client error propagation
func TestReconcileRepo_ClientError(t *testing.T) {
	mockClient := &mockGitClient{
		err: errors.New("git client failed"),
	}

	ctx := context.Background()
	cp := newTestCheckpoint("c1", "main")
	_, err := ReconcileRepo(ctx, cp, mockClient, "/tmp", nil)
	if err == nil {
		t.Fatalf("expected error from ReconcileRepo when client fails, got nil")
	}
}

// Mock git.Client implementation for unit tests
type mockGitClient struct {
	state        *git.RepositoryState
	commitExists bool
	isAncestor   bool
	changedFiles []string
	err          error
}

func (m *mockGitClient) GetState(ctx context.Context, repoPath string) (*git.RepositoryState, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.state, nil
}

func (m *mockGitClient) GetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	return m.state.CommitHash, nil
}

func (m *mockGitClient) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	return m.state.Branch, nil
}

func (m *mockGitClient) GetStatus(ctx context.Context, repoPath string) (*git.StatusResult, error) {
	return &git.StatusResult{IsClean: m.state.IsClean}, nil
}

func (m *mockGitClient) GetDiff(ctx context.Context, repoPath string, staged bool) (string, error) {
	return "", nil
}

func (m *mockGitClient) GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error) {
	return "", nil
}

func (m *mockGitClient) GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error) {
	return m.changedFiles, nil
}

func (m *mockGitClient) IsClean(ctx context.Context, repoPath string) (bool, error) {
	return m.state.IsClean, nil
}

func (m *mockGitClient) CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error) {
	return m.commitExists, nil
}

func (m *mockGitClient) IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error) {
	return m.isAncestor, nil
}

func (m *mockGitClient) GetRepoRoot(ctx context.Context, dir string) (string, error) {
	return dir, nil
}

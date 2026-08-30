package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/reconcile"
)

type mockGitClient struct {
	git.Client
	state    *git.RepositoryState
	err      error
	repoRoot string
}

func (m *mockGitClient) GetRepoRoot(ctx context.Context, dir string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.repoRoot != "" {
		return m.repoRoot, nil
	}
	return dir, nil
}

func (m *mockGitClient) GetState(ctx context.Context, repoPath string) (*git.RepositoryState, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.state != nil {
		return m.state, nil
	}
	return &git.RepositoryState{
		RootPath:   repoPath,
		CommitHash: "abcd123",
		Branch:     "main",
		IsClean:    true,
		HasCommits: true,
	}, nil
}

func (m *mockGitClient) GetCurrentBranch(ctx context.Context, repoPath string) (string, error) {
	if m.state != nil {
		return m.state.Branch, nil
	}
	return "main", nil
}

func (m *mockGitClient) GetDiff(ctx context.Context, repoPath string, staged bool) (string, error) {
	return "", nil
}

func (m *mockGitClient) GetDiffBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) (string, error) {
	return "", nil
}

func (m *mockGitClient) GetChangedFilesBetween(ctx context.Context, repoPath string, fromCommit, toCommit string) ([]string, error) {
	return []string{}, nil
}

func (m *mockGitClient) IsAncestor(ctx context.Context, repoPath string, ancestorCommit, descendantCommit string) (bool, error) {
	return true, nil
}

func (m *mockGitClient) CommitExists(ctx context.Context, repoPath string, commitHash string) (bool, error) {
	return true, nil
}

func (m *mockGitClient) IsClean(ctx context.Context, repoPath string) (bool, error) {
	if m.state != nil {
		return m.state.IsClean, nil
	}
	return true, nil
}

func (m *mockGitClient) GetCurrentCommit(ctx context.Context, repoPath string) (string, error) {
	if m.state != nil {
		return m.state.CommitHash, nil
	}
	return "abcd123", nil
}

func setupTestService(t *testing.T, client git.Client) (TaskService, string) {
	t.Helper()
	dir := t.TempDir()
	
	// Create .wake dir manually or InitDB fails if it doesn't try to create it
	// Actually InitDB creates it
	
	database, err := db.InitDB(dir)
	if err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	t.Cleanup(func() {
		database.Close()
	})
	
	if client == nil {
		client = &mockGitClient{repoRoot: dir}
	}

	return NewTaskService(database, client), dir
}

func TestInitWorkspace(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	
	ctx := context.Background()
	err := svc.InitWorkspace(ctx, dir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	wakeDir := filepath.Join(dir, ".wake")
	if _, err := os.Stat(wakeDir); os.IsNotExist(err) {
		t.Errorf("expected .wake directory to be created")
	}
	
	gitIgnore := filepath.Join(wakeDir, ".gitignore")
	if _, err := os.Stat(gitIgnore); os.IsNotExist(err) {
		t.Errorf("expected .gitignore to be created in .wake")
	}
}

func TestCreateCheckpoint_HappyPath(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	ctx := context.Background()
	
	req := CheckpointRequest{
		Dir: dir,
		Objective: "Test Objective",
	}
	
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cp == nil {
		t.Fatalf("expected checkpoint to be returned")
	}
	
	if cp.StateData.Objective != "Test Objective" {
		t.Errorf("expected objective 'Test Objective', got '%s'", cp.StateData.Objective)
	}
	if cp.Commit != "abcd123" {
		t.Errorf("expected commit 'abcd123', got '%s'", cp.Commit)
	}
}

func TestCreateCheckpoint_Force(t *testing.T) {
	client := &mockGitClient{
		state: &git.RepositoryState{
			IsClean: false,
			ModifiedFiles: []string{"file1.go"},
			CommitHash: "abcd123",
			Branch: "main",
		},
	}
	svc, dir := setupTestService(t, client)
	ctx := context.Background()
	
	req := CheckpointRequest{
		Dir: dir,
	}
	
	_, err := svc.CreateCheckpoint(ctx, req)
	if err == nil {
		t.Fatalf("expected error due to dirty state without force")
	}
	
	req.Force = true
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error with force flag, got %v", err)
	}
	if cp == nil {
		t.Fatalf("expected checkpoint to be returned")
	}
}

func TestCreateCheckpoint_ExplicitTaskID(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	ctx := context.Background()
	
	taskID := uuid.New().String()
	req := CheckpointRequest{
		Dir: dir,
		TaskID: taskID,
	}
	
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cp.TaskID.String() != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, cp.TaskID.String())
	}
}

func TestGetStatus_HappyPath(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	ctx := context.Background()
	
	req := CheckpointRequest{
		Dir: dir,
	}
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error creating checkpoint, got %v", err)
	}
	
	statusReq := StatusRequest{
		TaskID: cp.TaskID.String(),
		Dir: dir,
	}
	result, err := svc.GetStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	if result.Status != reconcile.StatusSafe {
		t.Errorf("expected status safe, got %v, reason: %s", result.Status, result.Reason)
	}
}

func TestRecordEvent_And_GetHistory(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	ctx := context.Background()
	
	// Create checkpoint to establish a TaskID implicitly
	req := CheckpointRequest{
		Dir: dir,
	}
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	taskID := cp.TaskID.String()
	
	ev, err := svc.RecordEvent(ctx, taskID, events.FileChanged, map[string]interface{}{"file": "test.go"})
	if err != nil {
		t.Fatalf("expected no error recording event, got %v", err)
	}
	if ev.TaskID.String() != taskID {
		t.Errorf("expected event task id %s, got %s", taskID, ev.TaskID.String())
	}
	
	history, err := svc.GetHistory(ctx, taskID, 10)
	if err != nil {
		t.Fatalf("expected no error getting history, got %v", err)
	}
	
	if len(history) < 2 { // Commit event from checkpoint + FileChanged
		t.Fatalf("expected at least 2 events in history, got %d", len(history))
	}
	
	lastEvent := history[len(history)-1]
	if lastEvent.Type != events.FileChanged {
		t.Errorf("expected last event to be FileChanged, got %s", lastEvent.Type)
	}
}

func TestResumeTask(t *testing.T) {
	client := &mockGitClient{
		state: &git.RepositoryState{
			IsClean: true,
			CommitHash: "abcd123",
			Branch: "main",
			HasCommits: true,
		},
	}
	svc, dir := setupTestService(t, client)
	ctx := context.Background()
	
	req := CheckpointRequest{
		Dir: dir,
	}
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	taskID := cp.TaskID.String()
	packet, err := svc.ResumeTask(ctx, taskID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	if packet.Checkpoint.ID != cp.ID {
		t.Errorf("expected checkpoint ID %s, got %s", cp.ID, packet.Checkpoint.ID)
	}
	if !strings.Contains(packet.Guidance, "Safe to resume") {
		t.Errorf("expected guidance to contain 'Safe to resume', got '%s', status: %s, reason: %s", packet.Guidance, packet.ReconciliationResult.Status, packet.ReconciliationResult.Reason)
	}
}

func TestUpdateObjective(t *testing.T) {
	svc, dir := setupTestService(t, nil)
	ctx := context.Background()
	
	req := CheckpointRequest{
		Dir: dir,
	}
	cp, err := svc.CreateCheckpoint(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	taskID := cp.TaskID.String()
	newObj := "New Test Objective"
	err = svc.UpdateObjective(ctx, taskID, newObj)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	
	history, err := svc.GetHistory(ctx, taskID, 10)
	if err != nil {
		t.Fatalf("expected no error getting history, got %v", err)
	}
	
	found := false
	for _, ev := range history {
		if ev.Type == events.TaskStarted {
			if obj, ok := ev.Payload["objective"].(string); ok && obj == newObj {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("expected to find event updating objective")
	}
}

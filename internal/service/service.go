package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/guard"
	"github.com/wake/wake/internal/reconcile"
	"github.com/wake/wake/internal/state"
)

type TaskService interface {
	CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
	GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
	GetHistory(ctx context.Context, taskID string, limit int) ([]events.Event, error)
	ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
	UpdateObjective(ctx context.Context, taskID string, objective string) error
	RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error)
	InitWorkspace(ctx context.Context, dir string) error
}

type taskService struct {
	db        *sql.DB
	gitClient git.Client
}

func NewTaskService(database *sql.DB, gitClient git.Client) TaskService {
	return &taskService{
		db:        database,
		gitClient: gitClient,
	}
}

func (s *taskService) getRepoRoot(ctx context.Context, dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get current working directory: %w", err)
		}
	}
	repoRoot, err := s.gitClient.GetRepoRoot(ctx, dir)
	if err != nil {
		return "", fmt.Errorf("git repository root not found at '%s': %w", dir, err)
	}
	return repoRoot, nil
}

func (s *taskService) CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error) {
	repoRoot, err := s.getRepoRoot(ctx, req.Dir)
	if err != nil {
		return nil, err
	}

	repoState, err := s.gitClient.GetState(ctx, repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect git repository state: %w", err)
	}

	guardOpts := guard.CheckpointGuardOptions{
		Force:        req.Force,
		TrackedFiles: req.TrackedFiles,
		RepoRoot:     repoRoot,
	}
	if err := guard.ValidatePreCheckpoint(ctx, repoState, guardOpts); err != nil {
		return nil, fmt.Errorf("pre-checkpoint guard blocked checkpoint: %w", err)
	}

	var taskID uuid.UUID
	var stateVersion int = 1
	var currentState state.State

	if req.TaskID != "" {
		parsed, err := uuid.Parse(req.TaskID)
		if err != nil {
			return nil, fmt.Errorf("invalid task-id '%s': %w", req.TaskID, err)
		}
		taskID = parsed
	}

	queryID := ""
	if taskID != uuid.Nil {
		queryID = taskID.String()
	}
	latestCP, err := db.GetLatestCheckpoint(ctx, s.db, queryID)
	if err == nil && latestCP != nil {
		if taskID == uuid.Nil {
			taskID = latestCP.TaskID
		}
		stateVersion = latestCP.StateVersion + 1
		currentState = latestCP.StateData
	} else if taskID == uuid.Nil {
		taskID = uuid.New()
	}

	history, err := db.GetEvents(ctx, s.db, taskID.String())
	if err == nil && len(history) > 0 {
		reduced := state.Reduce(taskID.String(), history)
		currentState = reduced
	}

	if req.Objective != "" {
		currentState.Objective = req.Objective
	}
	currentState.TaskID = taskID
	currentState.LastVerified = repoState.CommitHash

	commitEv := events.NewEvent(taskID, events.GitCommit, map[string]interface{}{
		"hash":   repoState.CommitHash,
		"branch": repoState.Branch,
		"clean":  repoState.IsClean,
	})
	_ = db.SaveEvent(ctx, s.db, commitEv)

	cp := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        taskID,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    repoState.RootPath,
		Branch:        repoState.Branch,
		Commit:        repoState.CommitHash,
		StateVersion:  stateVersion,
		EventPosition: len(history) + 1,
		StateData:     currentState,
	}

	if err := db.SaveCheckpoint(ctx, s.db, cp); err != nil {
		return nil, fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return &cp, nil
}

func (s *taskService) GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error) {
	repoRoot, err := s.getRepoRoot(ctx, req.Dir)
	if err != nil {
		return nil, err
	}

	cp, err := db.GetLatestCheckpoint(ctx, s.db, req.TaskID)
	if err != nil {
		return nil, err
	}

	result, err := reconcile.ReconcileRepo(ctx, *cp, s.gitClient, repoRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile repository: %w", err)
	}

	return &result, nil
}

func (s *taskService) GetHistory(ctx context.Context, taskID string, limit int) ([]events.Event, error) {
	cp, err := db.GetLatestCheckpoint(ctx, s.db, taskID)
	if err != nil {
		return nil, fmt.Errorf("no active task found")
	}

	evs, err := db.GetEvents(ctx, s.db, cp.TaskID.String())
	if err != nil {
		return nil, err
	}
	
	if limit > 0 && len(evs) > limit {
		evs = evs[len(evs)-limit:]
	}
	return evs, nil
}

func (s *taskService) ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error) {
	cp, err := db.GetLatestCheckpoint(ctx, s.db, taskID)
	if err != nil {
		return nil, fmt.Errorf("could not find latest checkpoint: %w", err)
	}

	repoRoot := cp.Repository
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	result, err := reconcile.ReconcileRepo(ctx, *cp, s.gitClient, repoRoot, nil)
	if err != nil {
		return nil, fmt.Errorf("reconciliation failed: %w", err)
	}

	guidance := ""
	if result.Status == reconcile.StatusSafe {
		guidance = "No modifications since last checkpoint. Safe to resume from Next Action."
	} else {
		if strings.Contains(result.Reason, "merge conflicts") {
			guidance = "CRITICAL RECOVERY INSTRUCTION: The repository is in a broken Git merge state. You must resolve the merge conflicts using `git merge --continue` or `git rebase --continue` before you do any other work."
		} else if !result.BranchMatch {
			branch, _ := s.gitClient.GetCurrentBranch(ctx, repoRoot)
			guidance = fmt.Sprintf("CRITICAL RECOVERY INSTRUCTION: You are on branch '%s', but the checkpoint was saved on branch '%s'. You must run `git checkout %s` before continuing to avoid corrupting the state.", branch, cp.Branch, cp.Branch)
		} else if len(result.ChangedFiles) > 0 {
			guidance = "RECOVERY INSTRUCTION: Read the changed files above before continuing to ensure your context is completely up-to-date."
		}
	}

	return &ResumePacket{
		Checkpoint:           *cp,
		ReconciliationResult: result,
		Guidance:             guidance,
	}, nil
}

func (s *taskService) UpdateObjective(ctx context.Context, taskID string, objective string) error {
	cp, err := db.GetLatestCheckpoint(ctx, s.db, taskID)
	if err != nil {
		return fmt.Errorf("no active task found to update")
	}

	ev := events.NewEvent(cp.TaskID, events.TaskStarted, map[string]interface{}{
		"objective": objective,
		"note":      "Human manually pivoted the objective",
	})

	if err := db.SaveEvent(ctx, s.db, ev); err != nil {
		return err
	}

	return nil
}

func (s *taskService) RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error) {
	var tid uuid.UUID
	if taskID != "" {
		parsed, err := uuid.Parse(taskID)
		if err != nil {
			return nil, fmt.Errorf("invalid task ID: %w", err)
		}
		tid = parsed
	} else {
		cp, err := db.GetLatestCheckpoint(ctx, s.db, "")
		if err == nil {
			tid = cp.TaskID
		} else {
			tid = uuid.New()
		}
	}

	ev := events.NewEvent(tid, eventType, payload)
	if err := db.SaveEvent(ctx, s.db, ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

func (s *taskService) InitWorkspace(ctx context.Context, dir string) error {
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
	}
	
	wakeDir := filepath.Join(dir, ".wake")
	if err := os.MkdirAll(wakeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .wake directory: %w", err)
	}
	
	gitignorePath := filepath.Join(wakeDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte("*\n"), 0644)
	}

	return nil
}

package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wake/internal/db"
	"wake/internal/events"
	"wake/internal/git"
	"wake/internal/guard"
	"wake/internal/reconcile"
	"wake/internal/state"
	"github.com/google/uuid"
)

type TaskService interface {
	CreateCheckpoint(ctx context.Context, req CheckpointRequest) (*state.Checkpoint, error)
	GetStatus(ctx context.Context, req StatusRequest) (*reconcile.ReconciliationResult, error)
	GetHistory(ctx context.Context, taskID string, limit int) ([]events.Event, error)
	ResumeTask(ctx context.Context, taskID string) (*ResumePacket, error)
	DiffCheckpoints(ctx context.Context, taskID string, v1, v2 int) (*state.StateDiff, error)
	UpdateObjective(ctx context.Context, taskID string, objective string) error
	RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error)
	InitWorkspace(ctx context.Context, dir string) error
	SetAuthor(author string)
}

type taskService struct {
	db        *sql.DB
	gitClient git.Client
	author    string
}

func NewTaskService(database *sql.DB, gitClient git.Client) TaskService {
	return &taskService{
		db:        database,
		gitClient: gitClient,
		author:    "Human CLI",
	}
}

func (s *taskService) SetAuthor(author string) {
	s.author = author
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
		return dir, nil
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

	var parsedTaskID uuid.UUID
	var stateVersion int = 1
	var currentState state.State

	if req.TaskID != "" {
		parsed, err := uuid.Parse(req.TaskID)
		if err != nil {
			return nil, fmt.Errorf("invalid task-id '%s': %w", req.TaskID, err)
		}
		parsedTaskID = parsed
	}

	queryID := ""
	if parsedTaskID != uuid.Nil {
		queryID = parsedTaskID.String()
	}
	latestCP, err := db.GetLatestCheckpoint(ctx, s.db, queryID)
	if err == nil && latestCP != nil {
		if parsedTaskID == uuid.Nil {
			parsedTaskID = latestCP.TaskID
		}
		stateVersion = latestCP.StateVersion + 1
		currentState = latestCP.StateData
	} else if parsedTaskID == uuid.Nil {
		parsedTaskID = uuid.New()
	}

	history, err := db.GetEvents(ctx, s.db, parsedTaskID.String())
	if err == nil && len(history) > 0 {
		reduced := state.Reduce(parsedTaskID.String(), history)
		currentState = reduced
	}

	if req.Objective != "" {
		currentState.Objective = req.Objective
	}
	currentState.TaskID = parsedTaskID
	currentState.LastVerified = repoState.CommitHash

	// Track files locally if no git commit
	if repoState.CommitHash == "" {
		if files, err := reconcile.ScanDirectory(repoState.RootPath); err == nil {
			currentState.Files = files
		}
	}

	commitEv := events.NewEvent(parsedTaskID, uuid.Nil, events.GitCommit, s.author, map[string]interface{}{
		"hash":   repoState.CommitHash,
		"branch": repoState.Branch,
		"clean":  repoState.IsClean,
	})
	_ = db.SaveEvent(ctx, s.db, commitEv)

	cp := state.Checkpoint{
		ID:            uuid.New(),
		TaskID:        parsedTaskID,
		SessionID:     uuid.Nil,
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Repository:    repoState.RootPath,
		Branch:        repoState.Branch,
		Commit:        repoState.CommitHash,
		Author:        s.author,
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
		guidance = "Clean. Resume from Next Action."
	} else {
		if strings.Contains(result.Reason, "merge conflicts") {
			guidance = "CRITICAL: Resolve merge conflicts first (git merge/rebase --continue)."
		} else if !result.BranchMatch {
			branch, _ := s.gitClient.GetCurrentBranch(ctx, repoRoot)
			guidance = fmt.Sprintf("CRITICAL: On branch '%s', expected '%s'. Run: git checkout %s", branch, cp.Branch, cp.Branch)
		} else if len(result.ChangedFiles) > 0 {
			guidance = "Files changed since checkpoint. Review before continuing."
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

	ev := events.NewEvent(cp.TaskID, uuid.Nil, events.TaskStarted, s.author, map[string]interface{}{
		"objective": objective,
		"note":      "Human manually pivoted the objective",
	})

	if err := db.SaveEvent(ctx, s.db, ev); err != nil {
		return err
	}

	return nil
}

func (s *taskService) RecordEvent(ctx context.Context, taskID string, eventType events.EventType, payload map[string]interface{}) (*events.Event, error) {
	var parsedTaskID uuid.UUID
	if taskID != "" {
		parsed, err := uuid.Parse(taskID)
		if err != nil {
			return nil, fmt.Errorf("invalid task ID: %w", err)
		}
		parsedTaskID = parsed
	} else {
		cp, err := db.GetLatestCheckpoint(ctx, s.db, "")
		if err == nil {
			parsedTaskID = cp.TaskID
		}
	}
	if parsedTaskID == uuid.Nil {
		parsedTaskID = uuid.New()
	}

	ev := events.NewEvent(parsedTaskID, uuid.Nil, eventType, s.author, payload)

	if s.db != nil {
		if err := db.SaveEvent(ctx, s.db, ev); err != nil {
			return nil, err
		}
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
	if err := os.MkdirAll(wakeDir, 0700); err != nil {
		return fmt.Errorf("failed to create .wake directory: %w", err)
	}

	gitignorePath := filepath.Join(wakeDir, ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte("*\n"), 0600)
	}

	return nil
}

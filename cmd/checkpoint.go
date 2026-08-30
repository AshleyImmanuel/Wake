package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/guard"
	"github.com/wake/wake/internal/state"
)

var (
	checkpointTaskID       string
	checkpointObjective    string
	checkpointDir          string
	checkpointForce        bool
	checkpointTrackedFiles []string
)

var checkpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Create a versioned snapshot of the current task state",
	Long:  "Captures the current Git repository state, reduces task events, and saves a versioned state checkpoint to SQLite.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runCheckpointWithOpts(cmd.Context(), checkpointDir, checkpointTaskID, checkpointObjective, checkpointForce, checkpointTrackedFiles)
	},
}

// runCheckpoint creates a checkpoint using default force=false and nil trackedFiles for backward compatibility.
func runCheckpoint(ctx context.Context, targetDir, taskIDStr, objective string) error {
	return runCheckpointWithOpts(ctx, targetDir, taskIDStr, objective, false, nil)
}

func runCheckpointWithOpts(ctx context.Context, targetDir, taskIDStr, objective string, force bool, trackedFiles []string) error {
	if targetDir == "" {
		var err error
		targetDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current working directory: %w", err)
		}
	}

	gitClient := git.NewClient(nil)
	repoRoot, err := gitClient.GetRepoRoot(ctx, targetDir)
	if err != nil {
		return fmt.Errorf("git repository root not found at '%s': %w", targetDir, err)
	}

	repoState, err := gitClient.GetState(ctx, repoRoot)
	if err != nil {
		return fmt.Errorf("failed to inspect git repository state: %w", err)
	}

	// Pre-Checkpoint Guard: Enforce that no un-tracked or human-modified files are blindly scooped
	guardOpts := guard.CheckpointGuardOptions{
		Force:        force,
		TrackedFiles: trackedFiles,
		RepoRoot:     repoRoot,
	}
	if err := guard.ValidatePreCheckpoint(ctx, repoState, guardOpts); err != nil {
		return fmt.Errorf("pre-checkpoint guard blocked checkpoint: %w", err)
	}

	database, err := db.InitDB(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize sentinel database: %w", err)
	}
	defer database.Close()

	var taskID uuid.UUID
	var stateVersion int = 1
	var currentState state.State

	if taskIDStr != "" {
		parsed, err := uuid.Parse(taskIDStr)
		if err != nil {
			return fmt.Errorf("invalid task-id '%s': %w", taskIDStr, err)
		}
		taskID = parsed
	}

	// Check if there is an existing checkpoint
	queryID := ""
	if taskID != uuid.Nil {
		queryID = taskID.String()
	}
	latestCP, err := db.GetLatestCheckpoint(ctx, database, queryID)
	if err == nil && latestCP != nil {
		if taskID == uuid.Nil {
			taskID = latestCP.TaskID
		}
		stateVersion = latestCP.StateVersion + 1
		currentState = latestCP.StateData
	} else if taskID == uuid.Nil {
		taskID = uuid.New()
	}

	// Fetch any recorded events and reduce to latest state
	history, err := db.GetEvents(ctx, database, taskID.String())
	if err == nil && len(history) > 0 {
		reduced := state.Reduce(taskID.String(), history)
		currentState = reduced
	}

	// If explicit objective is provided, update it
	if objective != "" {
		currentState.Objective = objective
	}
	currentState.TaskID = taskID
	currentState.LastVerified = repoState.CommitHash

	// Record a GitCommit event
	commitEv := events.NewEvent(taskID, events.GitCommit, map[string]interface{}{
		"hash":   repoState.CommitHash,
		"branch": repoState.Branch,
		"clean":  repoState.IsClean,
	})
	_ = db.SaveEvent(ctx, database, commitEv)

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

	if err := db.SaveCheckpoint(ctx, database, cp); err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	fmt.Println("[WAKE] Checkpoint created successfully.")
	fmt.Printf("Task ID:       %s\n", cp.TaskID.String())
	fmt.Printf("Commit:        %s\n", cp.Commit)
	fmt.Printf("Branch:        %s\n", cp.Branch)
	fmt.Printf("State Version: %d\n", cp.StateVersion)
	if repoState.IsClean {
		fmt.Println("Working Tree:  Clean")
	} else {
		fmt.Printf("Working Tree:  %d modified file(s)\n", len(repoState.ModifiedFiles)+len(repoState.UntrackedFiles))
	}

	return nil
}

func init() {
	checkpointCmd.Flags().StringVar(&checkpointTaskID, "task-id", "", "Task UUID for the checkpoint")
	checkpointCmd.Flags().StringVar(&checkpointObjective, "objective", "", "Task objective description")
	checkpointCmd.Flags().StringVar(&checkpointDir, "dir", "", "Repository directory (defaults to current directory)")
	checkpointCmd.Flags().BoolVarP(&checkpointForce, "force", "f", false, "Force checkpoint even if unreviewed changes exist")
	checkpointCmd.Flags().StringSliceVar(&checkpointTrackedFiles, "tracked-files", nil, "Comma-separated list of files tracked by the current task")
	rootCmd.AddCommand(checkpointCmd)
}

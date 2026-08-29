package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/hashfs"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/spf13/cobra"
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
		repoRoot = targetDir
		gitClient = hashfs.NewClient()
	}

	database, err := db.InitDB(repoRoot)
	if err != nil {
		return fmt.Errorf("failed to initialize wake database: %w", err)
	}
	defer database.Close()

	svc := service.NewTaskService(database, gitClient)
	cp, err := svc.CreateCheckpoint(ctx, service.CheckpointRequest{
		TaskID:       taskIDStr,
		Objective:    objective,
		Dir:          targetDir,
		Force:        force,
		TrackedFiles: trackedFiles,
	})
	if err != nil {
		return err
	}

	fmt.Println("[WAKE] Checkpoint created successfully.")
	fmt.Printf("Task ID:       %s\n", cp.TaskID.String())
	fmt.Printf("Commit:        %s\n", cp.Commit)
	fmt.Printf("Branch:        %s\n", cp.Branch)
	fmt.Printf("State Version: %d\n", cp.StateVersion)

	isClean, _ := gitClient.IsClean(ctx, repoRoot)
	if isClean {
		fmt.Println("Working Tree:  Clean")
	} else {
		fmt.Println("Working Tree:  Modified")
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

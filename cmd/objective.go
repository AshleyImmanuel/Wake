package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/events"
	"github.com/wake/wake/internal/git"
	"github.com/spf13/cobra"
)

var objectiveTaskID string

var objectiveCmd = &cobra.Command{
	Use:   "objective [new objective string]",
	Short: "Update the core objective of the current task to handle feature pivots",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, _ := os.Getwd()
		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), targetDir)
		if err != nil {
			return err
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			return err
		}
		defer database.Close()

		cp, err := db.GetLatestCheckpoint(context.Background(), database, objectiveTaskID)
		if err != nil {
			return fmt.Errorf("no active task found to update")
		}

		newObjective := args[0]
		
		// 1. Log the new objective as an event
		ev := events.NewEvent(cp.TaskID, events.TaskStarted, map[string]interface{}{
			"objective": newObjective,
			"note":      "Human manually pivoted the objective",
		})
		
		if err := db.SaveEvent(context.Background(), database, ev); err != nil {
			return err
		}
		
		fmt.Printf("Successfully updated the task objective to: '%s'\n", newObjective)
		fmt.Println("Run 'wake checkpoint' to solidify this new objective into the state.")
		return nil
	},
}

func init() {
	objectiveCmd.Flags().StringVar(&objectiveTaskID, "task-id", "", "Task UUID to update")
	rootCmd.AddCommand(objectiveCmd)
}

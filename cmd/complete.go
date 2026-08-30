package cmd

import (
	"fmt"
	"os"
	"time"

	"wake/internal/db"
	"wake/internal/events"
	"wake/internal/git"
	"github.com/spf13/cobra"
)

var completeCmd = &cobra.Command{
	Use:   "complete [milestone/feature]",
	Short: "Mark a milestone or feature as completed",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		completedItem := args[0]

		dir, err := os.Getwd()
		if err != nil {
			return err
		}

		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), dir)
		if err != nil {
			repoRoot = dir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}
		defer database.Close()

		// Get the latest task checkpoint
		cp, err := db.GetLatestCheckpoint(cmd.Context(), database, "all")
		if err != nil {
			return fmt.Errorf("no active task found. Please create a checkpoint first: %w", err)
		}

		ev := events.Event{
			TaskID:    cp.TaskID,
			Type:      events.MilestoneCompleted,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"milestone": completedItem,
			},
		}

		if err := db.SaveEvent(cmd.Context(), database, ev); err != nil {
			return fmt.Errorf("failed to save completed event: %w", err)
		}

		fmt.Printf("Successfully marked as completed for task %s:\n", cp.TaskID.String())
		fmt.Printf(" -> %s\n", completedItem)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(completeCmd)
}

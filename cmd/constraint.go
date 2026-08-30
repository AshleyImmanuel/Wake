package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"wake/internal/db"
	"wake/internal/events"
	"wake/internal/git"
)

var constraintCmd = &cobra.Command{
	Use:   "constraint",
	Short: "Manage constraints for the current active task",
}

var constraintAddCmd = &cobra.Command{
	Use:   "add [constraint description]",
	Short: "Add a new constraint to the current active task",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		constraintText := args[0]

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
			Type:      events.ConstraintAdded,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"constraint": constraintText,
			},
		}

		if err := db.SaveEvent(cmd.Context(), database, ev); err != nil {
			return fmt.Errorf("failed to save constraint event: %w", err)
		}

		fmt.Printf("Successfully added constraint to task %s:\n", cp.TaskID.String())
		fmt.Printf(" -> %s\n", constraintText)
		return nil
	},
}

func init() {
	constraintCmd.AddCommand(constraintAddCmd)
	rootCmd.AddCommand(constraintCmd)
}

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"wake/internal/db"
	"wake/internal/git"
	"wake/internal/service"
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
			repoRoot = targetDir
		}
		database, err := db.InitDB(repoRoot)
		if err != nil {
			return err
		}
		defer database.Close()

		newObjective := args[0]
		svc := service.NewTaskService(database, gitClient)
		if err := svc.UpdateObjective(cmd.Context(), objectiveTaskID, newObjective); err != nil {
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

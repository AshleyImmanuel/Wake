package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/service"
)

var (
	historyTaskID string
	historyDir    string
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "View the event history of the active task",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := historyDir
		if targetDir == "" {
			targetDir, _ = os.Getwd()
		}

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

		svc := service.NewTaskService(database, gitClient)
		events, err := svc.GetHistory(cmd.Context(), historyTaskID, 0)
		if err != nil {
			return err
		}

		fmt.Printf("Event History for Task\n") // We omit taskID since svc handles empty
		fmt.Println("--------------------------------------------------")
		for _, e := range events {
			fmt.Printf("[%s] %s\n", e.Timestamp.Format("15:04:05"), e.Type)
		}
		fmt.Printf("\nTotal Events: %d\n", len(events))
		return nil
	},
}

func init() {
	historyCmd.Flags().StringVar(&historyTaskID, "task-id", "", "Task UUID")
	historyCmd.Flags().StringVar(&historyDir, "dir", "", "Repository directory")
	rootCmd.AddCommand(historyCmd)
}

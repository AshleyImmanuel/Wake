package cmd

import (
	"fmt"
	"os"

	"wake/internal/db"
	"wake/internal/git"
	"wake/internal/service"
	"github.com/spf13/cobra"
)

var (
	diffTaskID string
	diffV1     int
	diffV2     int
)

var diffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the state delta between two checkpoints",
	Run: func(cmd *cobra.Command, args []string) {
		currentDir, _ := os.Getwd()
		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), currentDir)
		if err != nil {
			repoRoot = currentDir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			fmt.Printf("Failed to init db: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		svc := service.NewTaskService(database, gitClient)

		diff, err := svc.DiffCheckpoints(cmd.Context(), diffTaskID, diffV1, diffV2)
		if err != nil {
			fmt.Printf("Error calculating diff: %v\n", err)
			os.Exit(1)
		}

		fmt.Println(service.FormatDiff(*diff))
	},
}

func init() {
	rootCmd.AddCommand(diffCmd)
	diffCmd.Flags().StringVar(&diffTaskID, "task-id", "", "Specific task ID to diff")
	diffCmd.Flags().IntVar(&diffV1, "since", 0, "Older state version to compare from")
	diffCmd.Flags().IntVar(&diffV2, "to", 0, "Newer state version to compare to (defaults to latest)")
}

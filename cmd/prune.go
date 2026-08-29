package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/spf13/cobra"
)

var (
	pruneDays   int
	pruneDryRun bool
	pruneDir    string
)

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune old events and checkpoints to reduce database bloat",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := pruneDir
		if targetDir == "" {
			targetDir, _ = os.Getwd()
		}

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

		olderThan := time.Now().AddDate(0, 0, -pruneDays)

		fmt.Printf("Preparing to prune history older than %d days (%s)\n", pruneDays, olderThan.Format("2006-01-02"))
		fmt.Printf("Note: The latest checkpoint for each task will ALWAYS be preserved.\n")

		if pruneDryRun {
			fmt.Println("\n[DRY RUN] Would prune database, but no changes were made.")
			return nil
		}

		fmt.Print("\nWARNING: This is a destructive operation and cannot be undone. Are you sure you want to proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}

		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			fmt.Println("Prune aborted.")
			return nil
		}

		stats, err := db.PruneHistory(cmd.Context(), database, olderThan)
		if err != nil {
			return fmt.Errorf("failed to prune history: %w", err)
		}

		fmt.Printf("\nSuccessfully pruned database.\n")
		fmt.Printf("- Deleted Checkpoints: %d\n", stats.DeletedCheckpoints)
		fmt.Printf("- Deleted Events: %d\n", stats.DeletedEvents)

		return nil
	},
}

func init() {
	pruneCmd.Flags().IntVar(&pruneDays, "days", 30, "Delete history older than N days")
	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Simulate pruning without actually deleting anything")
	pruneCmd.Flags().StringVar(&pruneDir, "dir", "", "Repository directory")
	rootCmd.AddCommand(pruneCmd)
}

package cmd

import (
	"fmt"
	"os"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/spf13/cobra"
)

var initDir string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new Wake workspace",
	Long:  "Creates the .wake/ directory and initializes the SQLite database.",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := initDir
		if targetDir == "" {
			targetDir, _ = os.Getwd()
		}

		gitClient := git.NewClient(nil)
		// Best effort to find git repo, though init can just be in current dir
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), targetDir)
		if err != nil {
			repoRoot = targetDir
		}

		svc := service.NewTaskService(nil, gitClient)
		if err := svc.InitWorkspace(cmd.Context(), repoRoot); err != nil {
			return err
		}

		// Also init the db to ensure tables are created
		database, err := db.InitDB(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to initialize database: %w", err)
		}
		defer database.Close()

		fmt.Println("[WAKE] Successfully initialized Wake workspace in", repoRoot)
		return nil
	},
}

func init() {
	initCmd.Flags().StringVar(&initDir, "dir", "", "Directory to initialize (defaults to current directory)")
	rootCmd.AddCommand(initCmd)
}

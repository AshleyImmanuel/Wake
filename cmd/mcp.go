package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/mcp"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Wake MCP stdio server",
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, _ := os.Getwd()

		ctx, cancel := context.WithCancel(cmd.Context())
		defer cancel()

		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(ctx, workDir)
		if err != nil {
			repoRoot = workDir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to init db: %w", err)
		}
		defer database.Close()

		svc := service.NewTaskService(database, gitClient)

		server := mcp.NewServer(svc, workDir)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
		}()

		return server.Serve(ctx, os.Stdin, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

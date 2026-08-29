package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/wake/wake/internal/db"
	"github.com/wake/wake/internal/git"
	"github.com/wake/wake/internal/mcp"
	"github.com/wake/wake/internal/service"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Wake MCP stdio server",
	RunE: func(cmd *cobra.Command, args []string) error {
		workDir, _ := os.Getwd()

		database, err := db.InitDB(workDir)
		if err != nil {
			return fmt.Errorf("failed to init db: %w", err)
		}
		defer database.Close()

		gitClient := git.NewClient(nil)
		svc := service.NewTaskService(database, gitClient)

		server := mcp.NewServer(svc, workDir)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

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

package cmd

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
	"wake/internal/db"
	"wake/internal/git"
	"wake/internal/hashfs"
	"wake/internal/mcp"
	"wake/internal/service"
	"wake/internal/stash"
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
			gitClient = hashfs.NewClient()
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

		stashEngine := stash.NewEngine(repoRoot, gitClient)

		// Embedded File Watcher / Auto-Checkpointer (Highly Optimized)
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()

			lastCheckpointModTime := getModifiedFilesMaxTime(ctx, gitClient, repoRoot)

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// 1. Proactively backup human code (Continuous Recovery Stashing)
					_ = stashEngine.StashModifiedFiles(ctx)

					// 2. Checkpoint debounce logic
					currentModTime := getModifiedFilesMaxTime(ctx, gitClient, repoRoot)
					if currentModTime.After(lastCheckpointModTime) {
						// Check if stable (debounce)
						time.Sleep(3 * time.Second)
						if getModifiedFilesMaxTime(ctx, gitClient, repoRoot).Equal(currentModTime) {
							svc.CreateCheckpoint(ctx, service.CheckpointRequest{
								Objective: "Auto-checkpoint by Wake MCP Daemon",
								Dir:       repoRoot,
							})
							lastCheckpointModTime = currentModTime
						}
					}
				}
			}
		}()

		return server.Serve(ctx, os.Stdin, os.Stdout)
	},
}

func getModifiedFilesMaxTime(ctx context.Context, gitClient git.Client, root string) time.Time {
	var maxTime time.Time
	status, err := gitClient.GetStatus(ctx, root)
	if err != nil {
		return maxTime
	}

	filesToCheck := git.ExtractModifiedFiles(status)

	for _, f := range filesToCheck {
		absPath := filepath.Join(root, f)
		info, err := os.Stat(absPath)
		if err == nil && info.ModTime().After(maxTime) {
			maxTime = info.ModTime()
		}
	}
	return maxTime
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}

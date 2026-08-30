package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/events"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var watchDir string

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Run a background daemon to auto-track file modifications",
	Run: func(cmd *cobra.Command, args []string) {
		targetDir := watchDir
		if targetDir == "" {
			var err error
			targetDir, err = os.Getwd()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
		}

		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), targetDir)
		if err != nil {
			repoRoot = targetDir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			fmt.Printf("Failed to init db: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		svc := service.NewTaskService(database, gitClient)
		svc.SetAuthor("Wake Watcher Daemon")

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			fmt.Printf("Failed to create watcher: %v\n", err)
			os.Exit(1)
		}
		defer watcher.Close()

		// Walk through dir and add all subdirectories (except ignored)
		ignoredDirs := []string{".git", ".wake", "node_modules", ".vscode", ".claude", ".cursor", "vendor"}

		err = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				for _, ignored := range ignoredDirs {
					if strings.Contains(path, string(filepath.Separator)+ignored) || strings.HasSuffix(path, ignored) {
						return filepath.SkipDir
					}
				}
				err = watcher.Add(path)
				if err != nil {
					fmt.Printf("Could not watch %s: %v\n", path, err)
				}
			}
			return nil
		})
		if err != nil {
			fmt.Printf("Error setting up directory watches: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Wake daemon watching %s for changes...\n", repoRoot)

		debounceTimer := time.NewTimer(0)
		<-debounceTimer.C // stop initial fire

		modifiedFiles := make(map[string]bool)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Only track write events for simplicity (creates and deletes can also be tracked if needed)
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					// Ignore tmp files and editors saving
					if strings.HasSuffix(event.Name, "~") || strings.HasSuffix(event.Name, ".tmp") {
						continue
					}

					relPath, _ := filepath.Rel(repoRoot, event.Name)
					if relPath == "" {
						relPath = event.Name
					}

					modifiedFiles[relPath] = true
					debounceTimer.Reset(2 * time.Second)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				fmt.Println("Error:", err)
			case <-debounceTimer.C:
				if len(modifiedFiles) > 0 {
					// Record the events
					for file := range modifiedFiles {
						_, err := svc.RecordEvent(cmd.Context(), "", events.FileChanged, map[string]interface{}{
							"file":   file,
							"action": "modified",
						})
						if err != nil {
							fmt.Printf("Failed to record FileChanged for %s: %v\n", file, err)
						} else {
							fmt.Printf("[DAEMON] Auto-recorded modification: %s\n", file)
						}
					}
					// Clear the map
					modifiedFiles = make(map[string]bool)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
	watchCmd.Flags().StringVar(&watchDir, "dir", "", "Directory to watch (defaults to current)")
}

package cmd

import (
	"fmt"
	"os"

	"wake/internal/db"
	"wake/internal/git"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var sessionsTaskID string

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List all sessions for the active task",
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

		if sessionsTaskID == "" {
			latestCP, err := db.GetLatestCheckpoint(cmd.Context(), database, "")
			if err != nil {
				fmt.Printf("Error: could not find active task: %v\n", err)
				os.Exit(1)
			}
			sessionsTaskID = latestCP.TaskID.String()
		}

		checkpoints, err := db.GetRecentCheckpoints(cmd.Context(), database, sessionsTaskID, 1000)
		if err != nil {
			fmt.Printf("Error fetching sessions: %v\n", err)
			os.Exit(1)
		}

		sessionMap := make(map[uuid.UUID]int)
		var ordered []uuid.UUID

		for i := len(checkpoints) - 1; i >= 0; i-- {
			cp := checkpoints[i]
			if cp.SessionID != uuid.Nil {
				if _, exists := sessionMap[cp.SessionID]; !exists {
					ordered = append(ordered, cp.SessionID)
				}
				sessionMap[cp.SessionID]++
			}
		}

		fmt.Printf("SESSIONS FOR TASK %s\n\n", sessionsTaskID)
		if len(ordered) == 0 {
			fmt.Println("No named sessions found. All checkpoints are un-sessioned.")
			return
		}

		for i, sID := range ordered {
			fmt.Printf("Session #%d: %s (%d checkpoints)\n", i+1, sID.String(), sessionMap[sID])
		}
	},
}

func init() {
	rootCmd.AddCommand(sessionsCmd)
	sessionsCmd.Flags().StringVar(&sessionsTaskID, "task-id", "", "Specific task ID to list sessions for")
}

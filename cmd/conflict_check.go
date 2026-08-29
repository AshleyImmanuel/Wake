package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var conflictFile string

var conflictCmd = &cobra.Command{
	Use:   "check-conflict",
	Short: "Optimistic concurrency check to prevent AI from overwriting human work",
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetFile = conflictFile
		if targetFile == "" {
			// Read from stdin
			var payload struct {
				ToolCall struct {
					Args map[string]interface{} `json:"args"`
				} `json:"toolCall"`
			}
			if err := json.NewDecoder(os.Stdin).Decode(&payload); err == nil {
				if tf, ok := payload.ToolCall.Args["TargetFile"].(string); ok {
					targetFile = tf
				}
			}
		}

		if targetFile == "" {
			fmt.Println(`{"decision": "allow"}`)
			return nil
		}

		stat, err := os.Stat(targetFile)
		if err != nil {
			// File doesn't exist, no conflict
			fmt.Println(`{"decision": "allow"}`)
			return nil
		}

		// If file was modified in the last 10 seconds, it's highly likely a human just typed in it.
		if time.Since(stat.ModTime()) < 10*time.Second {
			fmt.Println(`{"decision": "deny", "reason": "Write-Write conflict detected! A human modified this file less than 10 seconds ago. Aborting write to prevent data loss."}`)
			return nil
		}

		fmt.Println(`{"decision": "allow"}`)
		return nil
	},
}

func init() {
	conflictCmd.Flags().StringVar(&conflictFile, "file", "", "Path to the file to check")
	rootCmd.AddCommand(conflictCmd)
}

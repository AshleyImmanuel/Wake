package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	markFile   string
	markAuthor string
)

var markCmd = &cobra.Command{
	Use:   "mark",
	Short: "Mark a file with an author attribution (AI or HUMAN)",
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetFile = markFile
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

		if targetFile == "" || markAuthor == "" {
			fmt.Println("{}")
			return nil
		}

		// SANITIZE INPUTS to prevent Log Injection and Evasion vulnerabilities
		targetFile = strings.ReplaceAll(targetFile, "\n", "")
		targetFile = strings.ReplaceAll(targetFile, "\r", "")
		targetFile = strings.ReplaceAll(targetFile, "|", "")
		
		markAuthor = strings.ReplaceAll(markAuthor, "\n", "")
		markAuthor = strings.ReplaceAll(markAuthor, "\r", "")
		markAuthor = strings.ReplaceAll(markAuthor, "|", "")

		targetDir, err := os.Getwd()
		if err != nil {
			return err
		}

		logPath := filepath.Join(targetDir, ".wake", "attribution.log")
		os.MkdirAll(filepath.Dir(logPath), 0700)

		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		defer f.Close()

		entry := fmt.Sprintf("%d|%s|%s\n", time.Now().Unix(), targetFile, markAuthor)
		if _, err := f.WriteString(entry); err != nil {
			return err
		}

		fmt.Println("{}")
		return nil
	},
}

func init() {
	markCmd.Flags().StringVar(&markFile, "file", "", "Path to the file being modified")
	markCmd.Flags().StringVar(&markAuthor, "author", "", "Author (AI or HUMAN)")
	rootCmd.AddCommand(markCmd)
}

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"wake/internal/updater"
)

// Version represents the current release version of Wake.
const Version = "v1.0.0-beta"

var rootCmd = &cobra.Command{
	Use:   "wake",
	Short: "Wake - AI coding-agent recovery infrastructure",
	Long: `Wake is a plug-and-play layer for existing AI coding agents 
that reconstructs and restores the current executable state of a 
coding task after a context or session transition.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// Do not block or prompt during MCP headless operations
		if cmd.Name() == "mcp" {
			return
		}
		updater.CheckForUpdates(Version)
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Initialize configuration or flags here
}

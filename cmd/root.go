package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wake",
	Short: "Wake - AI coding-agent recovery infrastructure",
	Long: `Wake is a plug-and-play layer for existing AI coding agents 
that reconstructs and restores the current executable state of a 
coding task after a context or session transition.`,
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

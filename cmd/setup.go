package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	setupCursor bool
	setupVSCode bool
	setupClaude bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Generate IDE MCP configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !setupCursor && !setupVSCode && !setupClaude {
			setupCursor = true
			setupVSCode = true
			setupClaude = true
		}

		exe, err := os.Executable()
		if err != nil {
			exe = "wake"
		}
		if exePath, err := exec.LookPath(exe); err == nil {
			if abs, err := filepath.Abs(exePath); err == nil {
				exe = abs
			}
		}

		mcpConfig := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"wake": map[string]interface{}{
					"command": exe,
					"args":    []string{"mcp"},
				},
			},
		}

		b, _ := json.MarshalIndent(mcpConfig, "", "  ")

		if setupCursor {
			if err := writeConfig(".cursor", "mcp.json", b); err != nil {
				return err
			}
			fmt.Println("Generated .cursor/mcp.json")
		}

		if setupVSCode {
			if err := writeConfig(".vscode", "mcp.json", b); err != nil {
				return err
			}
			fmt.Println("Generated .vscode/mcp.json")
		}

		if setupClaude {
			if err := os.WriteFile("claude_desktop_config.json", b, 0644); err != nil {
				return err
			}
			fmt.Println("Generated claude_desktop_config.json snippet")
		}

		return nil
	},
}

func writeConfig(dir, file string, data []byte) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, file), data, 0644)
}

func init() {
	setupCmd.Flags().BoolVar(&setupCursor, "cursor", false, "Generate .cursor/mcp.json")
	setupCmd.Flags().BoolVar(&setupVSCode, "vscode", false, "Generate .vscode/mcp.json")
	setupCmd.Flags().BoolVar(&setupClaude, "claude", false, "Generate claude_desktop_config.json snippet")
	rootCmd.AddCommand(setupCmd)
}

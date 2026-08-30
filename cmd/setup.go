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
	setupCursor      bool
	setupVSCode      bool
	setupClaude      bool
	setupAntigravity bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Generate IDE MCP configurations",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !setupCursor && !setupVSCode && !setupClaude && !setupAntigravity {
			setupCursor = true
			setupVSCode = true
			setupClaude = true
			setupAntigravity = true
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
			if err := os.WriteFile("claude_desktop_config.json", b, 0600); err != nil {
				return err
			}
			fmt.Println("Generated claude_desktop_config.json snippet")
		}

		if setupAntigravity {
			if err := writeConfig(".agents", "mcp_config.json", b); err != nil {
				return err
			}
			fmt.Println("Generated .agents/mcp_config.json")

			hooksConfig := map[string]interface{}{
				"wake-autosave": map[string]interface{}{
					"PostToolUse": []interface{}{
						map[string]interface{}{
							"matcher": "write_to_file|replace_file_content|multi_replace_file_content",
							"hooks": []interface{}{
								map[string]interface{}{
									"type":    "command",
									"command": fmt.Sprintf("\"%s\" checkpoint --objective 'Antigravity Auto-Save'", exe),
								},
							},
						},
					},
				},
				"wake-resume": map[string]interface{}{
					"PreInvocation": []interface{}{
						map[string]interface{}{
							"type":    "command",
							"command": fmt.Sprintf("\"%s\" resume > .agents/last_resume.txt", exe),
						},
					},
				},
			}
			hb, _ := json.MarshalIndent(hooksConfig, "", "  ")
			if err := writeConfig(".agents", "hooks.json", hb); err != nil {
				return err
			}
			fmt.Println("Generated .agents/hooks.json")
		}

		return nil
	},
}

func writeConfig(dir, file string, data []byte) error {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, file), data, 0600)
}

func init() {
	setupCmd.Flags().BoolVar(&setupCursor, "cursor", false, "Generate .cursor/mcp.json")
	setupCmd.Flags().BoolVar(&setupVSCode, "vscode", false, "Generate .vscode/mcp.json")
	setupCmd.Flags().BoolVar(&setupClaude, "claude", false, "Generate claude_desktop_config.json snippet")
	setupCmd.Flags().BoolVar(&setupAntigravity, "antigravity", false, "Generate .agents/mcp_config.json and hooks.json")
	rootCmd.AddCommand(setupCmd)
}

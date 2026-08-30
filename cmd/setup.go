package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

var (
	setupCursor      bool
	setupVSCode      bool
	setupClaude      bool
	setupAntigravity bool
	setupWindsurf    bool
	setupKiro        bool
	setupZed         bool
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Generate IDE MCP configurations for any AI coding tool",
	Long: `Generate MCP server configurations for Wake across all major AI coding tools.
If no flags are provided, configurations are generated for all supported IDEs.

Supported IDEs: Cursor, VS Code (Copilot), Windsurf, Kiro, Claude Desktop/Code, Zed, Antigravity`,
	RunE: func(cmd *cobra.Command, args []string) error {
		noFlags := !setupCursor && !setupVSCode && !setupClaude &&
			!setupAntigravity && !setupWindsurf && !setupKiro && !setupZed
		if noFlags {
			setupCursor = true
			setupVSCode = true
			setupClaude = true
			setupAntigravity = true
			setupWindsurf = true
			setupKiro = true
			setupZed = true
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

		// Standard MCP config used by Cursor, Windsurf, Claude Desktop, and Antigravity
		mcpConfig := map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"wake": map[string]interface{}{
					"command": exe,
					"args":    []string{"mcp"},
				},
			},
		}
		b, _ := json.MarshalIndent(mcpConfig, "", "  ")

		// VS Code uses a slightly different format with "servers" and "type"
		vscodeMcpConfig := map[string]interface{}{
			"servers": map[string]interface{}{
				"wake": map[string]interface{}{
					"type":    "stdio",
					"command": exe,
					"args":    []string{"mcp"},
				},
			},
		}
		vb, _ := json.MarshalIndent(vscodeMcpConfig, "", "  ")

		// --- Cursor ---
		if setupCursor {
			if err := writeConfig(".cursor", "mcp.json", b); err != nil {
				return err
			}
			fmt.Println("[OK] Generated .cursor/mcp.json")

			cursorHooks := map[string]interface{}{
				"version": 1,
				"hooks": map[string]interface{}{
					"preToolUse": []interface{}{
						map[string]interface{}{"command": fmt.Sprintf("\"%s\" check-conflict", exe)},
					},
					"postToolUse": []interface{}{
						map[string]interface{}{"command": fmt.Sprintf("\"%s\" mark --author AI", exe)},
						map[string]interface{}{"command": fmt.Sprintf("\"%s\" checkpoint --objective 'Cursor Auto-Save'", exe)},
					},
				},
			}
			cb, _ := json.MarshalIndent(cursorHooks, "", "  ")
			if err := writeConfig(".cursor", "hooks.json", cb); err != nil {
				return err
			}
			fmt.Println("[OK] Generated .cursor/hooks.json")
		}

		// --- VS Code (GitHub Copilot) ---
		if setupVSCode {
			if err := writeConfig(".vscode", "mcp.json", vb); err != nil {
				return err
			}
			fmt.Println("[OK] Generated .vscode/mcp.json")
		}

		// --- Windsurf ---
		if setupWindsurf {
			// Windsurf uses ~/.codeium/windsurf/mcp_config.json (global)
			windsurfDir := filepath.Join(homeDir(), ".codeium", "windsurf")
			if err := writeConfig(windsurfDir, "mcp_config.json", b); err != nil {
				// Fall back to project-level
				if err := writeConfig(".windsurf", "mcp_config.json", b); err != nil {
					return err
				}
				fmt.Println("[OK] Generated .windsurf/mcp_config.json (project-level)")
			} else {
				fmt.Println("[OK] Generated ~/.codeium/windsurf/mcp_config.json")
			}
		}

		// --- Kiro ---
		if setupKiro {
			if err := writeConfig(".kiro/settings", "mcp.json", b); err != nil {
				return err
			}
			fmt.Println("[OK] Generated .kiro/settings/mcp.json")
		}

		// --- Claude Desktop / Claude Code ---
		if setupClaude {
			if err := writeConfig(".claude", "mcp.json", b); err != nil {
				return err
			}
			fmt.Println("[OK] Generated .claude/mcp.json (Claude Code project-level)")

			if err := os.WriteFile("claude_desktop_config.json", b, 0600); err != nil {
				return err
			}
			fmt.Println("[OK] Generated claude_desktop_config.json (Claude Desktop snippet)")
		}

		// --- Zed ---
		if setupZed {
			zedConfig := map[string]interface{}{
				"context_servers": map[string]interface{}{
					"wake": map[string]interface{}{
						"command": exe,
						"args":    []string{"mcp"},
					},
				},
			}
			zb, _ := json.MarshalIndent(zedConfig, "", "  ")
			if err := os.WriteFile("zed_mcp_config.json", zb, 0600); err != nil {
				return err
			}
			fmt.Println("[OK] Generated zed_mcp_config.json (paste into Zed settings.json)")
		}

		// --- Antigravity ---
		if setupAntigravity {
			globalPluginDir := filepath.Join(homeDir(), ".gemini", "config", "plugins", "wake-mcp-plugin")
			
			if err := writeConfig(globalPluginDir, "mcp_config.json", b); err != nil {
				return err
			}
			fmt.Println("[OK] Generated global mcp_config.json")

			hooksConfig := map[string]interface{}{
				"wake-autosave": map[string]interface{}{
					"PreToolUse": []interface{}{
						map[string]interface{}{
							"matcher": "write_to_file|replace_file_content|multi_replace_file_content",
							"hooks": []interface{}{
								map[string]interface{}{
									"type":    "command",
									"command": fmt.Sprintf("\"%s\" check-conflict", exe),
								},
							},
						},
					},
					"PostToolUse": []interface{}{
						map[string]interface{}{
							"matcher": "write_to_file|replace_file_content|multi_replace_file_content",
							"hooks": []interface{}{
								map[string]interface{}{
									"type":    "command",
									"command": fmt.Sprintf("\"%s\" mark --author AI", exe),
								},
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
			if err := writeConfig(globalPluginDir, "hooks.json", hb); err != nil {
				return err
			}
			fmt.Println("[OK] Generated global hooks.json")
			
			pluginData := []byte("{\n  \"name\": \"wake-mcp-plugin\",\n  \"description\": \"Wake State Recovery Engine integration\"\n}")
			if err := writeConfig(globalPluginDir, "plugin.json", pluginData); err != nil {
				return err
			}
			
			skillData := []byte("---\nname: wake\ndescription: \"Wake: State Recovery Engine integration. Use to manage checkpoints, restore state, and resolve conflicts.\"\n---\n# Wake\nWake is an AI state recovery engine. You have access to MCP tools under the `wake` server (`wake_checkpoint`, `wake_status`, etc).\nWhen you begin a task in a workspace with a `.wake` directory, use `wake_status` to understand the state.")
			if err := writeConfig(filepath.Join(globalPluginDir, "skills", "wake"), "SKILL.md", skillData); err != nil {
				return err
			}
			fmt.Println("[OK] Generated global Wake SKILL.md")
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

func homeDir() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("USERPROFILE")
	}
	return os.Getenv("HOME")
}

func init() {
	setupCmd.Flags().BoolVar(&setupCursor, "cursor", false, "Generate .cursor/mcp.json")
	setupCmd.Flags().BoolVar(&setupVSCode, "vscode", false, "Generate .vscode/mcp.json (GitHub Copilot)")
	setupCmd.Flags().BoolVar(&setupWindsurf, "windsurf", false, "Generate Windsurf MCP config")
	setupCmd.Flags().BoolVar(&setupKiro, "kiro", false, "Generate .kiro/settings/mcp.json")
	setupCmd.Flags().BoolVar(&setupClaude, "claude", false, "Generate Claude Desktop/Code MCP configs")
	setupCmd.Flags().BoolVar(&setupZed, "zed", false, "Generate Zed MCP config snippet")
	setupCmd.Flags().BoolVar(&setupAntigravity, "antigravity", false, "Generate global Antigravity MCP configs and skills")
	rootCmd.AddCommand(setupCmd)
}

package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	installClient string
	installMcpDir string
)

const wakeInstructions = `
# Wake MCP Server Instructions

You are currently connected to the **Wake MCP Server**, which provides incremental state recovery, session continuity, and constraint tracking for this project.

## CORE DIRECTIVES (MUST FOLLOW):
1. **At the start of EVERY new session or when you first boot up:**
   - ALWAYS immediately call the ` + "`wake_resume`" + ` tool (passing empty task_id to get the active task).
   - Read the returned state JSON. It contains your Objective, Next Action, Blockers, and Constraints.
   - Proceed exactly from where the state dictates. Do not ignore it.

2. **When the user mentions making a manual change:**
   - ALWAYS call ` + "`wake_resume`" + ` or ` + "`wake_status`" + ` to sync your state before proceeding.

3. **Before you stop working or end a session:**
   - ALWAYS call ` + "`wake_checkpoint`" + ` to save your final state so the next agent can resume seamlessly.

Wake manages your context so you don't have to read the entire chat history. Rely on it heavily.
`

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install Wake MCP instructions for AI auto-bootstrapping",
	Run: func(cmd *cobra.Command, args []string) {
		targetDir := installMcpDir

		if targetDir == "" {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				fmt.Println("Error: could not determine home directory.")
				os.Exit(1)
			}

			switch installClient {
			case "antigravity":
				targetDir = filepath.Join(homeDir, ".gemini", "antigravity-ide", "mcp", "wake")
			case "claude":
				targetDir = filepath.Join(homeDir, ".claude", "mcp", "wake")
			case "cursor":
				targetDir = filepath.Join(homeDir, ".cursor", "mcp", "wake")
			default:
				fmt.Println("Error: Please provide --mcp-dir or a supported --client (antigravity, claude, cursor).")
				os.Exit(1)
			}
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", targetDir, err)
			os.Exit(1)
		}

		instrPath := filepath.Join(targetDir, "instructions.md")
		if err := os.WriteFile(instrPath, []byte(wakeInstructions), 0644); err != nil {
			fmt.Printf("Error writing instructions.md: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully installed Wake MCP instructions to:\n%s\n", instrPath)
		fmt.Println("Agents connecting to this MCP server will now automatically bootstrap and resume state.")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
	installCmd.Flags().StringVar(&installClient, "client", "", "Target client for automatic path resolution (antigravity, claude, cursor)")
	installCmd.Flags().StringVar(&installMcpDir, "mcp-dir", "", "Specific directory to install instructions.md")
}

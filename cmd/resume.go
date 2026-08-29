package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/reconcile"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/spf13/cobra"
)

var (
	resumeTaskID string
	resumeDir    string
)

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Generate a compact recovery packet for a new agent session",
	Long:  "Creates a Recovery Packet containing the latest state, the repository delta, and instructions for how the AI should continue.",
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir := resumeDir
		if targetDir == "" {
			var err error
			targetDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
		}

		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), targetDir)
		if err != nil {
			repoRoot = targetDir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to init db: %w", err)
		}
		defer database.Close()

		svc := service.NewTaskService(database, gitClient)
		packet, err := svc.ResumeTask(cmd.Context(), resumeTaskID)
		if err != nil {
			return err
		}

		cp := packet.Checkpoint
		result := packet.ReconciliationResult

		fmt.Println("======================================================================")
		fmt.Printf("RESUMING TASK: %s\n", cp.TaskID.String())
		fmt.Println("======================================================================")

		if cp.StateData.Objective != "" {
			fmt.Printf("\nGOAL\n%s\n", cp.StateData.Objective)
		}

		if len(cp.StateData.Completed) > 0 {
			fmt.Println("\nCOMPLETED")
			for _, c := range cp.StateData.Completed {
				fmt.Printf("[OK] %s\n", c)
			}
		}

		if cp.StateData.Current != "" {
			fmt.Printf("\nCURRENT\n%s\n", cp.StateData.Current)
		}

		activeBlockers := 0
		for _, b := range cp.StateData.Blocked {
			if b.Status == "ACTIVE" {
				if activeBlockers == 0 {
					fmt.Println("\nBLOCKERS")
				}
				fmt.Printf("[!] %s: %s\n", b.ID, b.Description)
				activeBlockers++
			}
		}

		if len(cp.StateData.Constraints) > 0 {
			fmt.Println("\nCONSTRAINTS")
			for _, c := range cp.StateData.Constraints {
				fmt.Printf("- %s\n", c)
			}
		}

		if len(cp.StateData.DoNotRepeat) > 0 {
			fmt.Println("\nDO NOT REPEAT")
			for _, c := range cp.StateData.DoNotRepeat {
				fmt.Printf("- %s\n", c)
			}
		}

		fmt.Printf("\nLAST VERIFIED\nCommit %s\n", cp.Commit)

		if cp.StateData.NextAction != "" {
			fmt.Printf("\nNEXT ACTION\n%s\n", cp.StateData.NextAction)
		}

		fmt.Printf("\nSTATE CONFIDENCE\n%s\n", result.ConfidenceLevel)

		fmt.Println("\n--- CURRENT REPOSITORY DELTA ---")
		if result.Status == reconcile.StatusSafe {
			fmt.Println("No modifications since last checkpoint. Safe to resume from Next Action.")
		} else {
			fmt.Printf("Status: %s\n", result.Status)

			if packet.Guidance != "" {
				// We still print the changed files if there's any
				if !strings.Contains(packet.Guidance, "CRITICAL") && len(result.ChangedFiles) > 0 {
					fmt.Println("The following files have changed since the AI paused:")
					for _, f := range result.ChangedFiles {
						fmt.Printf(" - %s\n", f)
					}
				}
				fmt.Printf("\n%s\n", packet.Guidance)
			}
		}
		fmt.Println("======================================================================")

		return nil
	},
}

func init() {
	resumeCmd.Flags().StringVar(&resumeTaskID, "task-id", "", "Task UUID to resume (optional)")
	resumeCmd.Flags().StringVar(&resumeDir, "dir", "", "Repository directory (defaults to current)")
	rootCmd.AddCommand(resumeCmd)
}

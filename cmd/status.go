package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AshleyImmanuel/Wake/internal/db"
	"github.com/AshleyImmanuel/Wake/internal/git"
	"github.com/AshleyImmanuel/Wake/internal/reconcile"
	"github.com/AshleyImmanuel/Wake/internal/service"
	"github.com/spf13/cobra"
)

var (
	statusTaskID string
	statusDir    string
	statusJSON   bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the current execution state and reconciliation status of the active task",
	Long:  "Compares the latest saved task checkpoint against live Git repository state to evaluate if the state is SAFE, STALE, or in CONFLICT.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if statusDir == "" {
			var err error
			statusDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current working directory: %w", err)
			}
		}

		gitClient := git.NewClient(nil)
		repoRoot, err := gitClient.GetRepoRoot(cmd.Context(), statusDir)
		if err != nil {
			repoRoot = statusDir
		}

		database, err := db.InitDB(repoRoot)
		if err != nil {
			return fmt.Errorf("failed to initialize WAKE database: %w", err)
		}
		defer database.Close()

		svc := service.NewTaskService(database, gitClient)
		result, err := svc.GetStatus(cmd.Context(), service.StatusRequest{
			TaskID: statusTaskID,
			Dir:    statusDir,
		})

		if err != nil {
			if statusJSON {
				out, _ := json.MarshalIndent(map[string]string{
					"status":  "UNKNOWN",
					"message": "No active task checkpoint found or error. Run 'WAKE checkpoint' first.",
				}, "", "  ")
				fmt.Println(string(out))
				return nil
			}
			fmt.Println("======================================================================")
			fmt.Println("WAKE STATUS: NO CHECKPOINT FOUND OR ERROR")
			fmt.Println("======================================================================")
			fmt.Printf("Error: %v\n", err)
			fmt.Println("Run 'WAKE checkpoint' to create an initial state snapshot.")
			fmt.Println("======================================================================")
			return nil
		}

		if statusJSON {
			out, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to format json output: %w", err)
			}
			fmt.Println(string(out))
			return nil
		}

		cp, err := db.GetLatestCheckpoint(cmd.Context(), database, statusTaskID)
		if err != nil {
			return fmt.Errorf("failed to get latest checkpoint: %w", err)
		}

		// Text output formatting
		fmt.Println("======================================================================")
		fmt.Println("WAKE STATUS")
		fmt.Println("======================================================================")

		fmt.Println(service.FormatVisualStatus(cp))

		fmt.Println("\n======================================================================")
		fmt.Println("WAKE TASK RECONCILIATION REPORT")
		fmt.Println("======================================================================")
		fmt.Printf("Task ID:            %s\n", cp.TaskID.String())
		if cp.StateData.Objective != "" {
			fmt.Printf("Objective:          %s\n", cp.StateData.Objective)
		}
		fmt.Printf("Status:             [%s]\n", result.Status)
		fmt.Printf("Confidence:         %s\n", result.ConfidenceLevel)
		if result.Reason != "" {
			fmt.Printf("Evaluation Reason:  %s\n", result.Reason)
		}

		fmt.Println("\n--- Repository State ---")
		fmt.Printf("Checkpoint Commit:  %s\n", result.CheckpointCommit)
		fmt.Printf("Current Commit:     %s\n", result.CurrentCommit)
		branchMatchText := "Yes"
		if !result.BranchMatch {
			branchMatchText = "No (Mismatch)"
		}
		fmt.Printf("Branch Match:       %s\n", branchMatchText)

		fmt.Println("\n--- Evaluation Summary ---")
		fmt.Printf("Total Changed Files:   %d\n", len(result.ChangedFiles))
		fmt.Printf("Task-Related Changes:  %d\n", len(result.TaskRelatedChanges))
		fmt.Printf("Unrelated Changes:     %d\n", len(result.UnrelatedChanges))
		fmt.Printf("Constraint Violations: %d\n", len(result.ConstraintViolations))
		fmt.Printf("Invalidated Claims:    %d\n", len(result.InvalidatedClaims))

		if len(result.ConstraintViolations) > 0 {
			fmt.Println("\n--- Constraint Violations ---")
			for _, v := range result.ConstraintViolations {
				fmt.Printf(" [!] %s\n", v)
			}
		}

		if len(result.InvalidatedClaims) > 0 {
			fmt.Println("\n--- Invalidated Claims ---")
			for _, c := range result.InvalidatedClaims {
				fmt.Printf(" [!] %s\n", c)
			}
		}

		if len(result.ChangedFiles) > 0 {
			fmt.Println("\n--- Changed Files ---")
			for _, f := range result.ChangedFiles {
				fmt.Printf(" [*] %s\n", f)
			}
		}

		fmt.Println("\n--- Guidance ---")
		switch result.Status {
		case reconcile.StatusSafe:
			fmt.Println("[SAFE] Working tree is fully synchronized with checkpoint. Safe to continue agent execution.")
		case reconcile.StatusStale:
			fmt.Println("[STALE] Repository has drifted without violating constraints. State refresh recommended.")
		case reconcile.StatusConflict:
			fmt.Println("[CONFLICT] Critical constraint violation or claim invalidation detected. Manual review required.")
		}
		fmt.Println("======================================================================")

		return nil
	},
}

func init() {
	statusCmd.Flags().StringVar(&statusTaskID, "task-id", "", "Task UUID to check (optional)")
	statusCmd.Flags().StringVar(&statusDir, "dir", "", "Repository directory (defaults to current directory)")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "Output report as JSON")
	rootCmd.AddCommand(statusCmd)
}

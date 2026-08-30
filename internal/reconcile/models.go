package reconcile

import "wake/internal/state"

// ReconciliationStatus represents the outcome of comparing a state checkpoint against live repository state.
type ReconciliationStatus string

const (
	StatusSafe       ReconciliationStatus = "SAFE"
	StatusConflict   ReconciliationStatus = "CONFLICT"
	StatusHumanAhead ReconciliationStatus = "HUMAN_AHEAD"
	StatusAIAhead    ReconciliationStatus = "AI_AHEAD"
	StatusDiverged   ReconciliationStatus = "DIVERGED"
)

// ReconciliationResult holds the complete evaluation result of a reconciliation run.
type ReconciliationResult struct {
	Status               ReconciliationStatus  `json:"status"`
	Reason               string                `json:"reason"`
	CheckpointCommit     string                `json:"checkpoint_commit"`
	CurrentCommit        string                `json:"current_commit"`
	BranchMatch          bool                  `json:"branch_match"`
	ChangedFiles         []string              `json:"changed_files"`
	TaskRelatedChanges   []string              `json:"task_related_changes"`
	UnrelatedChanges     []string              `json:"unrelated_changes"`
	ConstraintViolations []string              `json:"constraint_violations"`
	InvalidatedClaims    []string              `json:"invalidated_claims"`
	ConfidenceLevel      state.ConfidenceLevel `json:"confidence_level,omitempty"`
}

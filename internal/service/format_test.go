package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"wake/internal/reconcile"
	"wake/internal/state"
)

func TestFormatResumePacket_Truncation(t *testing.T) {
	// Create 50 completed items
	completed := make([]string, 50)
	for i := 0; i < 50; i++ {
		completed[i] = fmt.Sprintf("Completed item %d", i)
	}

	packet := &ResumePacket{
		Checkpoint: state.Checkpoint{
			TaskID: uuid.New(),
			StateData: state.State{
				Completed: completed,
			},
		},
		ReconciliationResult: reconcile.ReconciliationResult{
			Status: reconcile.StatusSafe,
		},
	}

	output := FormatResumePacket(packet)

	// It should cap at 3 items. So item 47, 48, 49.
	if !strings.Contains(output, "Completed item 47") {
		t.Errorf("Expected output to contain 'Completed item 47', got: %s", output)
	}
	if strings.Contains(output, "Completed item 0") {
		t.Errorf("Expected output NOT to contain 'Completed item 0'")
	}
	if !strings.Contains(output, "+47 more") {
		t.Errorf("Expected output to contain '+47 more'")
	}
}

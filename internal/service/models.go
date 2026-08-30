package service

import (
	"github.com/AshleyImmanuel/Wake/internal/state"
	"github.com/AshleyImmanuel/Wake/internal/reconcile"
)

type CheckpointRequest struct {
	TaskID       string
	Objective    string
	Dir          string
	TrackedFiles []string
	Force        bool
}

type StatusRequest struct {
	TaskID string
	Dir    string
}

type ResumePacket struct {
	Checkpoint           state.Checkpoint
	ReconciliationResult reconcile.ReconciliationResult
	Guidance             string
}

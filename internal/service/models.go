package service

import (
	"wake/internal/reconcile"
	"wake/internal/state"
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

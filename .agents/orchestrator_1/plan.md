# Execution Plan: Phase 2 Reconciliation

## Objective
Implement Phase 2 (Reconciliation) of Sentinel MVP: Git CLI wrapper, Reconciliation engine evaluating Checkpoint vs live Git state for SAFE/STALE/CONFLICT, and automated Go test suite with simulated git repositories.

## Phase 0: Survey & Exploration
- Spawn 3 parallel Explorers:
  - Explorer 1: Inspect existing Phase 1 Go codebase, package structure, go.mod, SQLite schema, and Checkpoint data models/structs.
  - Explorer 2: Analyze Git CLI wrapper requirements (commands needed: commit hash, modified files, uncommitted changes, status parsing, error handling) and integration points.
  - Explorer 3: Analyze Reconciliation engine rules and comparison logic (SAFE vs STALE vs CONFLICT, handling modified files, commit hashes, untracked/staged/unstaged files).

## Phase 1: Architecture & Decomposition
- Synthesize survey findings into PROJECT.md.
- Specify interface contracts for Git CLI Wrapper and Reconciliation Engine.
- Outline Code Layout and Milestones.

## Phase 2: Implementation & Verification Loop
- **Milestone 1**: Git CLI Wrapper
  - Worker implements Git CLI wrapper package.
  - Unit tests for git wrapper.
- **Milestone 2**: Reconciliation Engine
  - Worker implements comparison function taking Checkpoint and live Git repository state.
  - Status logic: SAFE, STALE, CONFLICT.
- **Milestone 3**: Verification Test Suite
  - Go test suite creating isolated temporary git repos.
  - Simulation of SAFE, STALE, and CONFLICT scenarios.
  - Verify `go test ./...` passes cleanly without intervention.

## Phase 3: Gate & Quality Assurance
- Reviewers (2 independent reviews).
- Challenger (adversarial edge-case tests).
- Forensic Auditor (integrity and anti-cheat verification).
- Final Gate evaluation.
- Human report & handoff to parent.

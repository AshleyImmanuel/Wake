package reconcile

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// determineDriftStatus calculates if the changed files were modified by AI, Human, or both.
func determineDriftStatus(repoRoot string, changedFiles []string, checkpointTime string) (ReconciliationStatus, string) {
	if len(changedFiles) == 0 {
		return StatusSafe, "Clean"
	}

	// Parse checkpoint time
	cpTime, err := time.Parse(time.RFC3339, checkpointTime)
	if err != nil {
		// Fallback to assuming everything is human if checkpoint is invalid
		cpTime = time.Time{}
	}

	logPath := filepath.Join(repoRoot, ".wake", "attribution.log")
	file, err := os.Open(logPath)

	// If there's no log file, we assume all changes were made by the human
	if err != nil {
		return StatusHumanAhead, "Workspace is HUMAN_AHEAD. Repository has uncommitted changed file(s)"
	}
	defer file.Close()

	// Map of filename -> latest AI modification timestamp
	aiMods := make(map[string]time.Time)

	stat, err := file.Stat()
	if err == nil && stat.Size() > 0 {
		size := stat.Size()
		var bufSize int64 = 4096
		if size < bufSize {
			bufSize = size
		}

		buf := make([]byte, bufSize)
		var cursor int64 = size
		var leftover string

		for cursor > 0 {
			readSize := bufSize
			if cursor < bufSize {
				readSize = cursor
			}
			cursor -= readSize

			file.Seek(cursor, 0) // io.SeekStart = 0
			file.Read(buf[:readSize])

			chunk := string(buf[:readSize]) + leftover
			lines := strings.Split(chunk, "\n")

			// The first line might be incomplete, save it for the next iteration
			if cursor > 0 {
				leftover = lines[0]
				// OOM DOS GUARD: If a single line exceeds 64KB, it's malicious. Discard it.
				if len(leftover) > 65536 {
					leftover = ""
				}
				lines = lines[1:]
			} else {
				leftover = ""
			}

			stopParsing := false
			// Process lines in reverse order
			for i := len(lines) - 1; i >= 0; i-- {
				line := strings.TrimSpace(lines[i])
				if line == "" {
					continue
				}

				parts := strings.Split(line, "|")
				if len(parts) == 3 {
					timestampStr := parts[0]
					filename := parts[1]
					author := parts[2]

					ts, err := strconv.ParseInt(timestampStr, 10, 64)
					if err == nil {
						modTime := time.Unix(ts, 0)

						// EARLY TERMINATION: If we reach an event older than the checkpoint, we can stop.
						// We use Before instead of !After to account for same-second events (timestamp truncation)
						if modTime.Before(cpTime) {
							stopParsing = true
							break
						}

						if author == "AI" {
							// Only set it if we haven't seen it yet (since we are reading backwards, first seen is newest)
							if _, exists := aiMods[filename]; !exists {
								aiMods[filename] = modTime
							}
						}
					}
				}
			}

			if stopParsing {
				break
			}
		}
	}

	aiModifiedCount := 0
	humanModifiedCount := 0

	for _, file := range changedFiles {
		aiModTime, hasAIMod := aiMods[file]

		// If the AI modified this file AFTER or exactly AT the checkpoint time, we attribute the change to the AI
		if hasAIMod && !aiModTime.Before(cpTime) {
			aiModifiedCount++
		} else {
			humanModifiedCount++
		}
	}

	if aiModifiedCount > 0 && humanModifiedCount > 0 {
		return StatusDiverged, "Workspace has DIVERGED. Both Human and AI have made uncommitted edits"
	} else if aiModifiedCount > 0 {
		return StatusAIAhead, "Workspace is AI_AHEAD. AI agent has made uncommitted edits"
	} else {
		return StatusHumanAhead, "Workspace is HUMAN_AHEAD. Repository has uncommitted changed file(s)"
	}
}

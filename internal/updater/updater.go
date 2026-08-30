package updater

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	repoAPIURL    = "https://api.github.com/repos/AshleyImmanuel/Wake/releases/latest"
	updateFile    = ".wake_update.json"
	checkInterval = 24 * time.Hour
)

type updateState struct {
	LastCheck time.Time `json:"last_check"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

// CheckForUpdates checks GitHub for a new release. It rate-limits itself to once per 24 hours.
// If a new version is found, it prompts the user to update.
func CheckForUpdates(currentVersion string) {
	// 1. Rate Limiting Check
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return
	}
	statePath := filepath.Join(homeDir, updateFile)

	var state updateState
	if b, err := os.ReadFile(statePath); err == nil {
		_ = json.Unmarshal(b, &state)
	}

	if time.Since(state.LastCheck) < checkInterval {
		return // Too soon to check again
	}

	// 2. Fetch Latest Release
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, repoAPIURL, nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	// Adding user-agent as best practice for github API
	req.Header.Set("User-Agent", "Wake-CLI-Updater")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return
	}

	// 3. Compare Versions
	// Ensure both have 'v' prefix for semver
	if !strings.HasPrefix(currentVersion, "v") {
		currentVersion = "v" + currentVersion
	}
	latestVersion := release.TagName
	if !strings.HasPrefix(latestVersion, "v") {
		latestVersion = "v" + latestVersion
	}

	// If latest is logically greater than current
	if semver.Compare(latestVersion, currentVersion) > 0 {
		promptUpdate(currentVersion, latestVersion, release.Body)
	}

	// 4. Update the timestamp
	state.LastCheck = time.Now()
	if b, err := json.Marshal(state); err == nil {
		// #nosec G306
		_ = os.WriteFile(statePath, b, 0600)
	}
}

func promptUpdate(current, latest, notes string) {
	fmt.Printf("\n🚀 A new version of Wake is available! (%s -> %s)\n", current, latest)
	fmt.Println("--------------------------------------------------")
	// Print a truncated or full version of notes
	lines := strings.Split(strings.TrimSpace(notes), "\n")
	for i, line := range lines {
		if i > 10 {
			fmt.Println("... (and more)")
			break
		}
		fmt.Println(line)
	}
	fmt.Println("--------------------------------------------------")
	fmt.Print("Would you like to auto-update now? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	ans, _ := reader.ReadString('\n')
	ans = strings.TrimSpace(strings.ToLower(ans))

	if ans == "y" || ans == "yes" || ans == "" {
		fmt.Println("Installing update...")
		// #nosec G204
		cmd := exec.Command("go", "install", "github.com/AshleyImmanuel/Wake@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ Failed to install update: %v\n", err)
			fmt.Println("Please run manually: go install github.com/AshleyImmanuel/Wake@latest")
		} else {
			fmt.Println("✅ Successfully updated Wake! Please restart your command.")
			os.Exit(0)
		}
	} else {
		fmt.Println("Skipping update. You can update later manually using: go install github.com/AshleyImmanuel/Wake@latest")
	}
	fmt.Println()
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Manage the passive background daemon",
}

var daemonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install and start the daemon to run on Windows startup",
	Run: func(cmd *cobra.Command, args []string) {
		exePath, err := os.Executable()
		if err != nil {
			fmt.Printf("Error getting executable path: %v\n", err)
			return
		}

		appData, err := os.UserConfigDir()
		if err != nil {
			fmt.Printf("Error getting AppData dir: %v\n", err)
			return
		}
		
		startupDir := filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`)
		vbsPath := filepath.Join(startupDir, "wake_daemon.vbs")
		batPath := filepath.Join(appData, "wake_daemon_runner.bat")
		
		cwd, _ := os.Getwd()
		
		batContent := fmt.Sprintf(`@echo off
"%s" watch --dir "%s" > "%%TEMP%%\wake_daemon_watch.log" 2>&1
`, exePath, cwd)
		err = os.WriteFile(batPath, []byte(batContent), 0644)
		if err != nil {
			fmt.Printf("Error writing bat script: %v\n", err)
			return
		}
		
		// Create VBScript that runs the bat invisibly
		vbsContent := fmt.Sprintf(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s""", 0, False
`, batPath)

		err = os.WriteFile(vbsPath, []byte(vbsContent), 0644)
		if err != nil {
			fmt.Printf("Error writing startup script: %v\n", err)
			return
		}

		fmt.Println("Successfully installed Wake daemon to Windows startup folder.")
		fmt.Printf("Script located at: %s\n", vbsPath)
		
		// Start it now
		daemonStartCmd.Flags().Set("dir", cwd)
		daemonStartCmd.Run(daemonStartCmd, []string{})
	},
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the background watcher daemon invisibly",
	Run: func(cmd *cobra.Command, args []string) {
		exePath, err := os.Executable()
		if err != nil {
			return
		}

		watchDirLocal, _ := cmd.Flags().GetString("dir")
		if watchDirLocal == "" {
			watchDirLocal, _ = os.Getwd()
		}

		tempBat := filepath.Join(os.TempDir(), "wake_start_temp.bat")
		batContent := fmt.Sprintf(`@echo off
"%s" watch --dir "%s" > "%%TEMP%%\wake_daemon_watch.log" 2>&1
`, exePath, watchDirLocal)
		_ = os.WriteFile(tempBat, []byte(batContent), 0644)

		tempVbs := filepath.Join(os.TempDir(), "wake_start_temp.vbs")
		vbsContent := fmt.Sprintf(`Set WshShell = CreateObject("WScript.Shell")
WshShell.Run """%s""", 0, False
`, tempBat)
		_ = os.WriteFile(tempVbs, []byte(vbsContent), 0644)

		c := exec.Command("wscript", tempVbs)
		err = c.Start()
		if err != nil {
			fmt.Printf("Failed to start background daemon: %v\n", err)
		} else {
			fmt.Println("Background daemon started invisibly.")
		}
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the background watcher daemon and remove from startup",
	Run: func(cmd *cobra.Command, args []string) {
		appData, _ := os.UserConfigDir()
		vbsPath := filepath.Join(appData, `Microsoft\Windows\Start Menu\Programs\Startup`, "wake_daemon.vbs")
		if err := os.Remove(vbsPath); err == nil {
			fmt.Println("Removed Wake daemon from Windows startup folder.")
		}

		killCmd := exec.Command("powershell", "-Command", "Get-WmiObject Win32_Process | Where-Object { $_.CommandLine -match 'wake.exe.*watch' -or $_.CommandLine -match 'wake_start_temp' -or $_.CommandLine -match 'wake_daemon_runner' } | ForEach-Object { $_.Terminate() }")
		err := killCmd.Run()
		if err != nil {
			fmt.Printf("Warning during process kill: %v\n", err)
		} else {
			fmt.Println("Successfully stopped running background daemon.")
		}
	},
}

func init() {
	daemonStartCmd.Flags().String("dir", "", "Directory to watch")
	daemonCmd.AddCommand(daemonInstallCmd)
	daemonCmd.AddCommand(daemonStartCmd)
	daemonCmd.AddCommand(daemonStopCmd)
	rootCmd.AddCommand(daemonCmd)
}

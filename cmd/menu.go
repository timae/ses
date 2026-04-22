package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	menuInstall   bool
	menuUninstall bool
)

const (
	menuAgentLabel = "io.timae.ses.menu"
	// legacyMenuLabel is the pre-v0.6 label, kept only for migration cleanup.
	legacyMenuLabel = "ai.rel.ses.menu"
)

var menuCmd = &cobra.Command{
	Use:   "menu",
	Short: "Launch the menu bar companion app",
	Long: `Launch a robot icon in the macOS menu bar showing daemon status,
recent sessions, and quick stats.

  ses menu               Launch in foreground
  ses menu --install     Install as LaunchAgent (starts on login)
  ses menu --uninstall   Remove the LaunchAgent`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if menuInstall {
			return installMenuAgent()
		}
		if menuUninstall {
			return uninstallMenuAgent()
		}

		// Launch ses-menu binary
		bin, err := findMenuBinary()
		if err != nil {
			return err
		}

		proc := exec.Command(bin)
		proc.Stdout = os.Stdout
		proc.Stderr = os.Stderr
		return proc.Run()
	},
}

func findMenuBinary() (string, error) {
	// Check next to ses binary
	exe, _ := os.Executable()
	dir := filepath.Dir(exe)
	candidate := filepath.Join(dir, "ses-menu")
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}

	// Check in PATH
	if path, err := exec.LookPath("ses-menu"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("ses-menu binary not found — build it with: go install github.com/timae/ses/cmd/ses-menu@latest")
}

func menuAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", menuAgentLabel+".plist")
}

func installMenuAgent() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("menu bar app is only supported on macOS")
	}

	bin, err := findMenuBinary()
	if err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	logPath := filepath.Join(home, ".ses", "menu.log")

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`, menuAgentLabel, bin, logPath, logPath)

	plistPath := menuAgentPath()
	// Migrate any pre-v0.6 agent + its plist so the menu bar doesn't
	// end up with two robots.
	migrateLegacyAgent(legacyMenuLabel)
	bootoutAgent(menuAgentLabel)

	if err := os.MkdirAll(filepath.Dir(plistPath), 0755); err != nil {
		return err
	}

	if err := os.WriteFile(plistPath, []byte(plist), 0644); err != nil {
		return err
	}

	if err := bootstrapAgent(plistPath, menuAgentLabel); err != nil {
		return fmt.Errorf("loading LaunchAgent: %w", err)
	}

	color.New(color.FgGreen).Println("Menu bar app installed and started.")
	fmt.Printf("  Binary:  %s\n", bin)
	fmt.Printf("  Plist:   %s\n", plistPath)
	fmt.Println("\nThe robot will appear in your menu bar on every login.")
	return nil
}

func uninstallMenuAgent() error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("menu bar app is only supported on macOS")
	}

	plistPath := menuAgentPath()
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		fmt.Println("Menu bar agent is not installed.")
		return nil
	}

	bootoutAgent(menuAgentLabel)
	migrateLegacyAgent(legacyMenuLabel)
	os.Remove(plistPath)

	color.New(color.FgYellow).Println("Menu bar agent uninstalled.")
	return nil
}

func init() {
	menuCmd.Flags().BoolVar(&menuInstall, "install", false, "install as macOS LaunchAgent (starts on login)")
	menuCmd.Flags().BoolVar(&menuUninstall, "uninstall", false, "remove the LaunchAgent")
	rootCmd.AddCommand(menuCmd)
}

package cmd

import (
	"fmt"
	"os"
	"os/exec"
)

// LaunchAgent load/unload helpers that target the modern `launchctl` API
// (bootstrap / bootout, introduced in macOS 10.11). The legacy `load` /
// `unload` subcommands fail with I/O errors on recent macOS — see
// https://developer.apple.com/library/archive/technotes/tn2083/_index.html
// for the domain model these modern subcommands operate against.

// userDomain returns the GUI-session domain string (e.g. "gui/501") that
// scopes a per-user LaunchAgent.
func userDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

// serviceTarget returns the domain-qualified service target
// (e.g. "gui/501/ai.rel.ses.menu") used with bootout / kickstart.
func serviceTarget(label string) string {
	return fmt.Sprintf("%s/%s", userDomain(), label)
}

// bootstrapAgent loads (or reloads) the agent described by plistPath under
// the current user's GUI domain. Any previously loaded instance of label is
// booted out first so overwrites work cleanly.
func bootstrapAgent(plistPath, label string) error {
	// Ignore error: service may not be loaded, or may have been loaded via
	// the legacy path — either way, we want it gone before bootstrap.
	exec.Command("launchctl", "bootout", serviceTarget(label)).Run()
	if err := exec.Command("launchctl", "bootstrap", userDomain(), plistPath).Run(); err != nil {
		return fmt.Errorf("launchctl bootstrap %s: %w", plistPath, err)
	}
	return nil
}

// bootoutAgent unloads the agent named by label. It is intentionally
// tolerant of "service not loaded" errors so uninstall flows don't fail
// when the user never successfully loaded the agent in the first place.
func bootoutAgent(label string) {
	exec.Command("launchctl", "bootout", serviceTarget(label)).Run()
}

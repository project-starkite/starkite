// Package commands provides ai-only CLI commands for the ai edition.
package commands

import (
	"sync"

	"github.com/spf13/cobra"
)

var registerOnce sync.Once

// Register adds ai-specific commands to the root command.
// It is idempotent — safe to call multiple times.
// Phase 0: no commands registered yet; ai subcommands arrive in later phases.
func Register(root *cobra.Command) {
	registerOnce.Do(func() {
		// ai-specific commands will be registered here in later phases
	})
}

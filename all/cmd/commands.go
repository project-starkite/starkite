// Package commands provides the all-edition cobra command registration.
// It chains every edition's Register function so the all-edition binary
// exposes every subcommand from base + cloud + ai.
package commands

import (
	"sync"

	aicmd "github.com/project-starkite/starkite/ai/cmd"
	cloudcmd "github.com/project-starkite/starkite/cloud/cmd"
	"github.com/spf13/cobra"
)

var registerOnce sync.Once

// Register adds every edition's subcommands to the root command.
// Idempotent — safe to call multiple times.
func Register(root *cobra.Command) {
	registerOnce.Do(func() {
		cloudcmd.Register(root)
		aicmd.Register(root)
	})
}

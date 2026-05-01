// Package commands provides cloud-only CLI commands for the cloud edition.
package commands

import (
	"sync"

	"github.com/spf13/cobra"
)

var registerOnce sync.Once

// Register adds cloud-specific commands to the root command.
// It is idempotent — safe to call multiple times.
func Register(root *cobra.Command) {
	registerOnce.Do(func() {
		root.AddCommand(kubeCmd)
	})
}

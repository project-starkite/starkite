// Package commands provides cloud-only CLI commands for the cloud edition.
package commands

import (
	"fmt"
	"sync"

	"github.com/spf13/cobra"
)

var registerOnce sync.Once

// Register adds cloud-specific commands to the root command.
// It is idempotent — safe to call multiple times.
func Register(root *cobra.Command) {
	registerOnce.Do(func() {
		root.AddCommand(applyCmd)
		root.AddCommand(deployCmd)
		root.AddCommand(driftCmd)
		root.AddCommand(kubeCmd)
	})
}

var applyCmd = &cobra.Command{
	Use:   "apply <manifest.star>",
	Short: "Apply cloud infrastructure from a starkite manifest",
	Long: `Apply cloud infrastructure changes defined in a starkite manifest.

This command evaluates the manifest and applies the resulting resources
to your cloud environment (Kubernetes, cloud provider, etc.).

Examples:
  kite apply infra.star
  kite apply infra.star --var env=production
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("apply: not yet implemented")
	},
}

var deployCmd = &cobra.Command{
	Use:   "deploy <manifest.star>",
	Short: "Deploy an application using a starkite manifest",
	Long: `Deploy an application using a starkite deployment manifest.

This command orchestrates a full deployment: build, push, and rollout.

Examples:
  kite deploy app.star
  kite deploy app.star --var image_tag=v1.2.0
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("deploy: not yet implemented")
	},
}

var driftCmd = &cobra.Command{
	Use:   "drift [manifest.star]",
	Short: "Detect configuration drift between manifest and live state",
	Long: `Detect configuration drift between a starkite manifest and live infrastructure.

Compares the desired state defined in the manifest against the actual state
of deployed resources and reports any differences.

Examples:
  kite drift infra.star
  kite drift infra.star --output yaml
`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return fmt.Errorf("drift: not yet implemented")
	},
}

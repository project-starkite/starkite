package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/starkite/core/edition"
	"github.com/vladimirvivien/starkite/core/version"
)

var editionCmd = &cobra.Command{
	Use:   "edition",
	Short: "Manage starkite editions",
	Long: `Manage starkite editions.

Editions provide different feature sets. The base edition includes core
automation modules. The cloud edition adds Kubernetes, container, and
cloud provider modules.

Examples:
  # Switch to cloud edition (downloads if not installed)
  kite edition use cloud

  # Switch to cloud from a local binary
  kite edition use cloud --from ./kite-cloud

  # Switch back to base
  kite edition use base

  # Show current edition status
  kite edition status

  # Remove an edition
  kite edition remove cloud
`,
}

var editionUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Switch to an edition",
	Long: `Switch the active edition.

If the edition is not installed, it will be downloaded from GitHub Releases
automatically. Use --from to install from a local binary instead.

All subsequent kite commands will be handled by the selected edition.
Use "kite edition use base" to return to the base edition.

Examples:
  kite edition use cloud
  kite edition use cloud --from ./kite-cloud
  kite edition use base
`,
	Args: cobra.ExactArgs(1),
	RunE: runEditionUse,
}

var editionRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Short:   "Remove an installed edition",
	Aliases: []string{"rm", "uninstall"},
	Args:    cobra.ExactArgs(1),
	RunE:    runEditionRemove,
}

var editionStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current edition status",
	Args:  cobra.NoArgs,
	RunE:  runEditionStatus,
}

// Flags
var (
	editionUseFrom  string
	editionUseForce bool
)

func init() {
	editionUseCmd.Flags().StringVar(&editionUseFrom, "from", "", "Install from local binary path")
	editionUseCmd.Flags().BoolVar(&editionUseForce, "force", false, "Overwrite existing installation")

	editionCmd.AddCommand(editionUseCmd)
	editionCmd.AddCommand(editionRemoveCmd)
	editionCmd.AddCommand(editionStatusCmd)

	rootCmd.AddCommand(editionCmd)
}

func runEditionUse(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Switching to base never requires install
	if name == edition.EditionBase {
		if err := edition.SetActiveEdition(name); err != nil {
			return err
		}
		fmt.Printf("Switched to %s edition.\n", name)
		return nil
	}

	// Check if already installed
	binaryPath, err := edition.EditionBinaryPath(name)
	if err != nil {
		return err
	}

	needsInstall := false
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		needsInstall = true
	} else if editionUseFrom != "" && editionUseForce {
		needsInstall = true
	}

	if needsInstall {
		opts := edition.InstallOptions{
			FromPath: editionUseFrom,
			Force:    editionUseForce,
		}
		fmt.Printf("Installing %s edition...\n", name)
		if err := edition.Install(name, opts); err != nil {
			return err
		}
	}

	if err := edition.SetActiveEdition(name); err != nil {
		return err
	}

	fmt.Printf("Switched to %s edition.\n", name)
	return nil
}

func runEditionRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := edition.Remove(name); err != nil {
		return err
	}

	fmt.Printf("Removed %s edition.\n", name)
	return nil
}

func runEditionStatus(cmd *cobra.Command, args []string) error {
	active := edition.ActiveEdition()

	fmt.Printf("Current edition: %s\n", active)
	fmt.Printf("Version:         %s\n", version.Version)
	fmt.Printf("Binary edition:  %s\n", version.EditionName())
	fmt.Println()

	editions, err := edition.ListInstalled()
	if err != nil {
		return err
	}

	fmt.Println("Installed editions:")
	for _, e := range editions {
		marker := "  "
		if e.Active {
			marker = "* "
		}
		fmt.Printf("  %s%s", marker, e.Name)
		if e.Size > 0 {
			fmt.Printf(" (%s)", formatSize(e.Size))
		}
		fmt.Println()
	}

	return nil
}

func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

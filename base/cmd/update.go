package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/base/edition"
	"github.com/project-starkite/starkite/base/version"
)

var (
	updateCheck bool
	updateForce bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update starkite to the latest version",
	Long: `Check for and install the latest version of starkite.

Downloads the latest release from GitHub and replaces the current binary.
Any installed edition binaries are also updated to the same version.

Examples:
  # Update to the latest version
  kite update

  # Check for updates without installing
  kite update --check

  # Force update even if already up-to-date or running a dev build
  kite update --force
`,
	Args: cobra.NoArgs,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&updateCheck, "check", false, "Check for updates without installing")
	updateCmd.Flags().BoolVar(&updateForce, "force", false, "Force update even if versions match or dev build")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Refuse dev builds unless --force
	if edition.IsDevBuild() && !updateForce {
		return fmt.Errorf("running a dev build (%s); use --force to update anyway", version.Version)
	}

	// Fetch latest release from GitHub
	fmt.Println("Checking for updates...")
	release, err := edition.FetchLatestRelease()
	if err != nil {
		return fmt.Errorf("cannot check for updates: %w", err)
	}

	latestVersion := release.LatestVersion()
	fmt.Printf("Current version: %s\n", version.Version)
	fmt.Printf("Latest version:  %s\n", latestVersion)

	// Compare versions
	if version.Version == latestVersion && !updateForce {
		fmt.Println("Already up-to-date.")
		return nil
	}

	// --check: just print and stop
	if updateCheck {
		if version.Version != latestVersion {
			fmt.Printf("\nUpdate available: %s → %s\n", version.Version, latestVersion)
			fmt.Println("Run 'kite update' to install.")
		}
		return nil
	}

	// Update the base binary
	fmt.Printf("\nUpdating kite to %s...\n", latestVersion)
	path, err := edition.UpdateSelf(latestVersion)
	if err != nil {
		return err
	}
	fmt.Printf("Updated: %s\n", path)

	// Update installed edition binaries
	editions, err := edition.ListInstalled()
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: cannot list editions: %v\n", err)
		return nil
	}

	for _, e := range editions {
		if e.Name == edition.EditionBase || !e.Installed {
			continue
		}
		fmt.Printf("Updating %s edition...\n", e.Name)
		if err := edition.UpdateEdition(e.Name, latestVersion); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: failed to update %s edition: %v\n", e.Name, err)
		} else {
			fmt.Printf("Updated %s edition.\n", e.Name)
		}
	}

	fmt.Printf("\nkite updated to %s successfully.\n", latestVersion)
	return nil
}

package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/project-starkite/starkite/basekite/manager"
	"github.com/project-starkite/starkite/libkite"
	"github.com/spf13/cobra"
)

var moduleCmd = &cobra.Command{
	Use:   "module",
	Short: "Manage external starkite modules",
	Long: `Manage external starkite modules.

Modules are Starlark script modules that extend starkite with additional
functionality, installed from a git repository or a local directory.

Examples:
  # Install a module from a git host
  kite module install gitlab.com/user/kite-helm

  # Install with specific version
  kite module install gitlab.com/user/kite-helm@v1.0.0

  # Install with custom name
  kite module install gitlab.com/user/kite-helm --as helm

  # List installed modules
  kite module list

  # Update a module
  kite module update helm

  # Remove a module
  kite module remove helm

  # Show module info
  kite module info helm
`,
}

var moduleInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install a module from a git repository or local path",
	Long: `Install a module from a git repository or local directory.

A module's identity (namespace/name) comes from its mod.yaml. A source is
host-agnostic — any git host works, not just well-known ones.

Supported source formats:
  host.tld/org/repo           HTTPS clone from any git host (gitlab, internal, …)
  host.tld/org/repo@v1.0.0    Specific tag, branch, or commit
  git@host.tld:org/repo.git   SSH clone
  https://host.tld/org/repo   Full URL
  ./path/to/module            Local directory (copied, not cloned)

When the manifest omits a namespace, it falls back to the source's org; for a
local directory install --as <namespace>/<name> may be required.

Examples:
  kite module install gitlab.com/acme/kite-helm
  kite module install git.internal/acme/kite-helm@v1.0.0
  kite module install ./my-module
  kite module install ./my-module --as acme/helm
`,
	Args: cobra.ExactArgs(1),
	RunE: runModuleInstall,
}

var moduleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed modules",
	Long: `List all installed modules.

Shows the module name, type, version (if available), and source repository.
`,
	Args: cobra.NoArgs,
	RunE: runModuleList,
}

var moduleUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Update an installed module",
	Long: `Update an installed module to the latest version.

This pulls the latest changes from the module's git repository.
`,
	Args: cobra.ExactArgs(1),
	RunE: runModuleUpdate,
}

var moduleRemoveCmd = &cobra.Command{
	Use:   "remove <name>[@rev]",
	Short: "Remove an installed module",
	Long: `Remove an installed module.

Bare namespace/name deletes every cached revision; a @rev suffix deletes only
that revision.
`,
	Aliases: []string{"rm", "uninstall"},
	Args:    cobra.ExactArgs(1),
	RunE:    runModuleRemove,
}

var moduleInfoCmd = &cobra.Command{
	Use:   "info <name>[@rev]",
	Short: "Show information about an installed module",
	Long: `Show detailed information about an installed module.

Displays the module's name, revision, version, repository, and entry point.
Bare namespace/name lists every installed revision; a @rev suffix shows one.
`,
	Args: cobra.ExactArgs(1),
	RunE: runModuleInfo,
}

var moduleVerifyCmd = &cobra.Command{
	Use:   "verify [name[@rev]]",
	Short: "Verify installed modules against their recorded hash",
	Long: `Re-hash installed modules and compare against the content hash recorded
at install, detecting on-disk tampering or corruption.

With no argument, every installed module is checked. With a namespace/name, only
that module is checked. Exits non-zero if any module fails.
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runModuleVerify,
}

// Flags
var (
	moduleInstallAs    string
	moduleInstallForce bool
)

func init() {
	// Install flags
	moduleInstallCmd.Flags().StringVar(&moduleInstallAs, "as", "", "Install with custom name")
	moduleInstallCmd.Flags().BoolVar(&moduleInstallForce, "force", false, "Overwrite existing module")

	// Add subcommands
	moduleCmd.AddCommand(moduleInstallCmd)
	moduleCmd.AddCommand(moduleListCmd)
	moduleCmd.AddCommand(moduleUpdateCmd)
	moduleCmd.AddCommand(moduleRemoveCmd)
	moduleCmd.AddCommand(moduleInfoCmd)
	moduleCmd.AddCommand(moduleVerifyCmd)

	// Add to root
	rootCmd.AddCommand(moduleCmd)
}

func runModuleInstall(cmd *cobra.Command, args []string) error {
	source := args[0]

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	opts := manager.InstallOptions{
		Name:  moduleInstallAs,
		Force: moduleInstallForce,
	}

	if !manager.GitAvailable() {
		return fmt.Errorf("git is required but not found in PATH")
	}

	fmt.Printf("Installing module from %s...\n", source)

	info, err := mgr.Install(source, opts)
	if err != nil {
		return err
	}

	qualified := info.Name
	if info.Namespace != "" {
		qualified = info.Namespace + "/" + info.Name
	}
	fmt.Printf("Installed %s", qualified)
	if info.Version != "" {
		fmt.Printf(" (%s)", info.Version)
	}
	fmt.Printf(" to %s\n", info.Path)

	return nil
}

func runModuleList(cmd *cobra.Command, args []string) error {
	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	modules, err := mgr.List()
	if err != nil {
		return err
	}

	if len(modules) == 0 {
		fmt.Println("No modules installed.")
		fmt.Println("\nInstall modules with:")
		fmt.Println("  kite module install host.tld/org/repo")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tREV\tTYPE\tVERSION\tSOURCE")
	fmt.Fprintln(w, "----\t---\t----\t-------\t------")

	for _, m := range modules {
		version := m.Version
		if version == "" {
			version = "-"
		}
		rev := m.Rev
		if rev == "" {
			rev = "-"
		}
		source := m.Repository
		if source == "" {
			source = "(local)"
		} else {
			source = shortenRepoURL(source)
		}
		name := m.Name
		if m.Namespace != "" {
			name = m.Namespace + "/" + m.Name
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", name, rev, m.Type, version, source)
	}

	w.Flush()
	return nil
}

func runModuleUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	fmt.Printf("Updating %s...\n", name)

	// A git source needs git; a local source does not. Update reads the recorded
	// source, so let the install step surface a missing-git error only when the
	// source actually requires it.
	info, err := mgr.Update(name)
	if err != nil {
		return err
	}

	ref := info.Name
	if info.Namespace != "" {
		ref = info.Namespace + "/" + info.Name
	}
	detail := info.Version
	if detail == "" {
		detail = info.Rev
	}
	fmt.Printf("Updated %s (%s)\n", ref, detail)
	return nil
}

func runModuleRemove(cmd *cobra.Command, args []string) error {
	identity, rev := libkite.SplitModuleRev(args[0])

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	if err := mgr.Remove(identity, rev); err != nil {
		return err
	}

	if rev != "" {
		fmt.Printf("Removed module %s@%s\n", identity, rev)
	} else {
		fmt.Printf("Removed module %s\n", identity)
	}
	return nil
}

func runModuleInfo(cmd *cobra.Command, args []string) error {
	identity, rev := libkite.SplitModuleRev(args[0])

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	if rev != "" {
		info, err := mgr.Resolve(identity, rev)
		if err != nil {
			return err
		}
		printModuleInfo(info)
		return nil
	}

	revs, err := mgr.Revisions(identity)
	if err != nil {
		return err
	}
	if len(revs) == 0 {
		return fmt.Errorf("module %q not installed", identity)
	}

	if len(revs) > 1 {
		fmt.Printf("%s has %d installed revisions:\n\n", identity, len(revs))
	}
	for i, info := range revs {
		if i > 0 {
			fmt.Println()
		}
		printModuleInfo(info)
	}
	return nil
}

func printModuleInfo(info *manager.ModuleInfo) {
	fmt.Printf("Name:        %s\n", info.Name)
	if info.Rev != "" {
		fmt.Printf("Revision:    %s\n", info.Rev)
	}
	fmt.Printf("Type:        %s\n", info.Type)
	fmt.Printf("Path:        %s\n", info.Path)

	if info.Version != "" {
		fmt.Printf("Version:     %s\n", info.Version)
	}
	if info.Repository != "" {
		fmt.Printf("Repository:  %s\n", info.Repository)
	}
	if info.Description != "" {
		fmt.Printf("Description: %s\n", info.Description)
	}
	if info.EntryPoint != "" {
		fmt.Printf("Entry point: %s\n", info.EntryPoint)
	}
}

func runModuleVerify(cmd *cobra.Command, args []string) error {
	var identity, rev string
	if len(args) == 1 {
		identity, rev = libkite.SplitModuleRev(args[0])
	}

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	results, err := mgr.Verify(identity, rev)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("No modules installed.")
		return nil
	}

	failed := 0
	for _, r := range results {
		label := r.Identity
		if r.Rev != "" {
			label += "@" + r.Rev
		}
		if r.OK {
			fmt.Printf("ok\t%s\n", label)
			continue
		}
		failed++
		fmt.Printf("FAIL\t%s\t%s\n", label, r.Reason)
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d module(s) failed verification", failed, len(results))
	}
	return nil
}

// shortenRepoURL shortens a repository URL for display.
func shortenRepoURL(url string) string {
	// Remove https:// prefix
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")

	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")

	// Convert git@ format
	if strings.HasPrefix(url, "git@") {
		url = strings.TrimPrefix(url, "git@")
		url = strings.Replace(url, ":", "/", 1)
	}

	return url
}

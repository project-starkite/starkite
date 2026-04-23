package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/starkite/core/manager"
)

var moduleCmd = &cobra.Command{
	Use:   "module",
	Short: "Manage external starkite modules",
	Long: `Manage external starkite modules.

Modules extend starkite with additional functionality. Supported module types:
  - starlark: Script modules written in Starlark (installed from git)
  - wasm:     WebAssembly modules (installed from local paths or git)

Examples:
  # Install a starlark module from GitHub
  kite module install github.com/user/kite-helm

  # Install with specific version
  kite module install github.com/user/kite-helm@v1.0.0

  # Install with custom name
  kite module install github.com/user/kite-helm --as helm

  # Install a WASM module from a local directory
  kite module install --type wasm ./path/to/wasm-module

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
	Long: `Install a module from a git repository or local path.

For starlark modules, supported source formats:
  github.com/user/repo          HTTPS clone from GitHub
  gitlab.com/user/repo          HTTPS clone from GitLab
  bitbucket.org/user/repo       HTTPS clone from Bitbucket
  user/repo                     Short form for github.com/user/repo
  github.com/user/repo@v1.0.0   Specific tag/version
  github.com/user/repo@main     Specific branch
  github.com/user/repo@abc1234  Specific commit
  git@github.com:user/repo.git  SSH clone

For WASM modules, source can be a local directory containing a module.yaml
and .wasm file, or a git repository. Use --type wasm to force WASM install.

The module name is inferred from the repository name or manifest, but can
be overridden with --as flag.

Examples:
  kite module install github.com/user/kite-helm
  kite module install user/helm-module --as helm
  kite module install github.com/user/kite-helm@v1.0.0
  kite module install --force github.com/user/kite-helm
  kite module install --type wasm ./path/to/echo
  kite module install --type wasm github.com/user/wasm-plugin
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
WASM modules cannot be updated; reinstall with --force instead.
`,
	Args: cobra.ExactArgs(1),
	RunE: runModuleUpdate,
}

var moduleRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed module",
	Long: `Remove an installed module.

This permanently deletes the module and its files.
`,
	Aliases: []string{"rm", "uninstall"},
	Args:    cobra.ExactArgs(1),
	RunE:    runModuleRemove,
}

var moduleInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show information about an installed module",
	Long: `Show detailed information about an installed module.

Displays the module's name, type, version, repository, and entry point.
For WASM modules, also shows the WASM file, exported functions, and permissions.
`,
	Args: cobra.ExactArgs(1),
	RunE: runModuleInfo,
}

// Flags
var (
	moduleInstallAs    string
	moduleInstallForce bool
	moduleInstallType  string
)

func init() {
	// Install flags
	moduleInstallCmd.Flags().StringVar(&moduleInstallAs, "as", "", "Install with custom name")
	moduleInstallCmd.Flags().BoolVar(&moduleInstallForce, "force", false, "Overwrite existing module")
	moduleInstallCmd.Flags().StringVar(&moduleInstallType, "type", "", "Module type: starlark or wasm (auto-detected if omitted)")

	// Add subcommands
	moduleCmd.AddCommand(moduleInstallCmd)
	moduleCmd.AddCommand(moduleListCmd)
	moduleCmd.AddCommand(moduleUpdateCmd)
	moduleCmd.AddCommand(moduleRemoveCmd)
	moduleCmd.AddCommand(moduleInfoCmd)

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

	// Determine if this is a WASM install
	if detectWasmInstall(source, moduleInstallType) {
		return runWasmInstall(mgr, source, opts)
	}

	// Starlark install (requires git)
	if !manager.GitAvailable() {
		return fmt.Errorf("git is required but not found in PATH")
	}

	fmt.Printf("Installing module from %s...\n", source)

	info, err := mgr.Install(source, opts)
	if err != nil {
		return err
	}

	fmt.Printf("Installed %s", info.Name)
	if info.Version != "" {
		fmt.Printf(" (%s)", info.Version)
	}
	fmt.Printf(" to %s\n", info.Path)

	return nil
}

func runWasmInstall(mgr *manager.Manager, source string, opts manager.InstallOptions) error {
	fmt.Printf("Installing WASM module from %s...\n", source)

	info, err := mgr.InstallWasm(source, opts)
	if err != nil {
		return err
	}

	fmt.Printf("Installed %s", info.Name)
	if info.Version != "" {
		fmt.Printf(" (%s)", info.Version)
	}
	fmt.Printf(" to %s\n", info.Path)

	return nil
}

// detectWasmInstall determines if the install should use the WASM path.
func detectWasmInstall(source, typeFlag string) bool {
	if typeFlag == "wasm" {
		return true
	}
	if typeFlag == "starlark" {
		return false
	}

	// Auto-detect: .wasm file extension
	if strings.HasSuffix(source, ".wasm") {
		return true
	}

	// Auto-detect: local directory with a module.yaml containing a "wasm:" field
	abs, err := filepath.Abs(source)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return false
	}
	data, err := os.ReadFile(filepath.Join(abs, "module.yaml"))
	if err != nil {
		return false
	}
	// Simple heuristic: if module.yaml has a "wasm:" field, it's a WASM module
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "wasm:") {
			return true
		}
	}
	return false
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
		fmt.Println("  kite module install github.com/user/repo")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tVERSION\tSOURCE")
	fmt.Fprintln(w, "----\t----\t-------\t------")

	for _, m := range modules {
		version := m.Version
		if version == "" {
			version = "-"
		}
		source := m.Repository
		if source == "" {
			source = "(local)"
		} else {
			source = shortenRepoURL(source)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", m.Name, m.Type, version, source)
	}

	w.Flush()
	return nil
}

func runModuleUpdate(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Check if git is available
	if !manager.GitAvailable() {
		return fmt.Errorf("git is required but not found in PATH")
	}

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	fmt.Printf("Updating %s...\n", name)

	info, err := mgr.Update(name)
	if err != nil {
		return err
	}

	fmt.Printf("Updated %s to %s\n", info.Name, info.Version)
	return nil
}

func runModuleRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	if err := mgr.Remove(name); err != nil {
		return err
	}

	fmt.Printf("Removed module %s\n", name)
	return nil
}

func runModuleInfo(cmd *cobra.Command, args []string) error {
	name := args[0]

	mgr, err := manager.New("")
	if err != nil {
		return fmt.Errorf("failed to initialize module manager: %w", err)
	}

	info, err := mgr.Get(name)
	if err != nil {
		return err
	}

	fmt.Printf("Name:        %s\n", info.Name)
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

	// WASM-specific fields
	if info.Type == "wasm" {
		if info.WasmFile != "" {
			fmt.Printf("WASM file:   %s\n", info.WasmFile)
		}
		if len(info.Functions) > 0 {
			fmt.Printf("Functions:   %s\n", strings.Join(info.Functions, ", "))
		}
		if len(info.Permissions) > 0 {
			fmt.Printf("Permissions: %s\n", strings.Join(info.Permissions, ", "))
		}
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

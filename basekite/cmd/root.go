package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/project-starkite/starkite/basekite/edition"
	"github.com/project-starkite/starkite/basekite/varstore"
	"github.com/project-starkite/starkite/basekite/version"
	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/permissions"
	"github.com/project-starkite/starkite/libkite/sandbox"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
	// Global flags
	outputFormat string
	debugMode    bool
	dryRun       bool
	timeout      int
	variables    []string
	varFiles     []string

	// Permission flags
	permissionsMode string

	// permissionProfileFlags are boolean aliases for --permissions=<name>,
	// one per built-in profile (e.g. --allow-fs == --permissions=allow-fs).
	// Cobra enforces that at most one — including --permissions — is set.
	permissionProfileFlags = []string{
		"deny-all", "allow-fs", "allow-net", "allow-local", "allow-all",
	}

	// Sandbox flag (Linux: gVisor; other OSes return a friendly error).
	// Set via --sandbox on the CLI; STARKITE_SECURITY_SANDBOX env var
	// is the alternative entry point for shebang-launched scripts (and
	// works for CLI invocations too). The flag wins when both are set.
	sandboxMode string
)

// permissionAlias is a bool-style pflag.Value that sets the shared
// --permissions target to a fixed profile name when its flag is present, so
// --allow-fs is exactly --permissions=allow-fs. Mutual exclusion with
// --permissions and the other aliases is enforced by MarkFlagsMutuallyExclusive.
type permissionAlias struct{ profile string }

func (permissionAlias) String() string   { return "" }
func (permissionAlias) Type() string     { return "bool" }
func (permissionAlias) IsBoolFlag() bool { return true }
func (a permissionAlias) Set(string) error {
	permissionsMode = a.profile
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "kite [script.star]",
	Short: "Starkite - A powerful automation tool for cloud-native infrastructure",
	Long: `kite is the CLI for starkite, a powerful automation tool for cloud-native infrastructure.
It provides a unified scripting interface using Starlark, a Python-like language, to execute
commands across local machines, remote servers, containers, and Kubernetes clusters.

Examples:
  # Execute a script (these are equivalent)
  kite script.star
  kite run script.star
  ./script.star              # with shebang: #!/usr/bin/env kite

  # Execute inline Starlark code
  kite exec 'print(local.exec("hostname").stdout)'

  # Interactive REPL
  kite repl

  # Pipe output to other tools
  kite manifest.star | kubectl apply -f -
`,
	Version: version.String(),

	// SilenceUsage prevents cobra from printing usage text after RunE errors.
	// Cobra still prints usage for its own command-parsing errors (unknown
	// subcommand, missing args, unknown flags) because those are handled
	// before RunE runs.
	SilenceUsage: true,

	// SilenceErrors leaves error printing to Execute(), which suppresses
	// ExitError messages to avoid double-printing.
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format: text, json, yaml, table")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview commands without executing")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 300, "Script execution timeout in seconds")
	rootCmd.PersistentFlags().StringArrayVar(&variables, "var", nil, "Set script variable: --var key=value")
	rootCmd.PersistentFlags().StringArrayVar(&varFiles, "var-file", nil, "Load variables from YAML file: --var-file=values.yaml")

	// Permission flags
	rootCmd.PersistentFlags().StringVar(&permissionsMode, "permissions", "",
		"Permission profile: a built-in (deny-all|allow-fs|allow-net|allow-local|allow-all), "+
			"a named/config profile, inline rules (allow:…;deny:…), or a file path")

	// Boolean aliases for the built-in profiles, e.g. --allow-fs == --permissions=allow-fs.
	for _, p := range permissionProfileFlags {
		rootCmd.PersistentFlags().Var(permissionAlias{p}, p, fmt.Sprintf("Alias for --permissions=%s", p))
		rootCmd.PersistentFlags().Lookup(p).NoOptDefVal = "true" // usable as a bare flag
	}

	// Sandbox flag — Linux only; non-Linux returns a clear error.
	// `--sandbox` (no value) resolves to the built-in "default" profile;
	// `--sandbox=<name>` resolves a built-in, file path, or named user
	// profile from ~/.starkite/security.yaml. Shebang-launched scripts
	// can use STARKITE_SECURITY_SANDBOX env var instead — see
	// docs/guides/sandbox.md.
	rootCmd.PersistentFlags().StringVar(&sandboxMode, "sandbox", "",
		"Sandbox profile for OS-level isolation (Linux). "+
			"Use --sandbox alone for the built-in \"default\" profile, "+
			"or --sandbox=<name> for a user-defined profile.")
	rootCmd.PersistentFlags().Lookup("sandbox").NoOptDefVal = "default"

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnvDefaults()
		return checkPermissionFlagConflict(rootCmd.PersistentFlags())
	}
}

// checkPermissionFlagConflict rejects setting more than one permission selector
// — the profile aliases (--allow-fs, …) and --permissions all target the same
// value, so combining them is contradictory.
func checkPermissionFlagConflict(flags *pflag.FlagSet) error {
	var set []string
	if flags.Lookup("permissions").Changed {
		set = append(set, "--permissions")
	}
	for _, p := range permissionProfileFlags {
		if flags.Lookup(p).Changed {
			set = append(set, "--"+p)
		}
	}
	if len(set) > 1 {
		return fmt.Errorf("only one permission flag may be set; got %s", strings.Join(set, " and "))
	}
	return nil
}

// applyEnvDefaults applies STARKITE_* environment variables for any flag
// that wasn't explicitly set on the command line.
// Priority: CLI flags > env vars > defaults.
func applyEnvDefaults() {
	flags := rootCmd.PersistentFlags()

	if !flags.Lookup("debug").Changed {
		if v := os.Getenv("STARKITE_DEBUG"); v == "1" || v == "true" {
			debugMode = true
			fmt.Fprintln(os.Stderr, "[DEBUG] Debug mode enabled via STARKITE_DEBUG")
		}
	}

	if !flags.Lookup("output").Changed {
		if v := os.Getenv("STARKITE_OUTPUT"); v != "" {
			outputFormat = v
		}
	}

	if !flags.Lookup("timeout").Changed {
		if v := os.Getenv("STARKITE_TIMEOUT"); v != "" {
			if t, err := strconv.Atoi(v); err == nil && t > 0 {
				timeout = t
			}
		}
	}
}

// RootCmd returns the root cobra command for edition command registration.
func RootCmd() *cobra.Command {
	return rootCmd
}

// Execute runs the root command and returns the exit code
func Execute() int {
	// Let edition binaries register their commands before execution.
	if RegisterEditionCommands != nil {
		RegisterEditionCommands(rootCmd)
	}

	// Edition handoff: if base edition and a non-base edition is active,
	// exec the edition binary (replaces this process).
	if version.IsBaseEdition() && shouldHandoff() {
		active := edition.ActiveEdition()
		if active != edition.EditionBase {
			if binaryPath, err := edition.EditionBinaryPath(active); err == nil {
				if _, err := os.Stat(binaryPath); err == nil {
					if err := edition.ExecHandoff(binaryPath); err != nil {
						fmt.Fprintf(os.Stderr, "warning: edition handoff failed: %v (falling back to base)\n", err)
					}
				}
			}
		}
	}

	// Implicit run: if the first arg is a run target (a .star file, an existing
	// file, a module directory, or an @namespace/name reference) rather than a
	// flag or subcommand, insert "run".
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		if !strings.HasPrefix(firstArg, "-") && looksLikeRunTarget(firstArg) {
			os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		// ExitError with code 0 — silent success (e.g. exit(0))
		var exitErr *libkite.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}

		// All other errors — print to stderr
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return exitCodeFromError(err)
	}
	return 0
}

// looksLikeRunTarget reports whether arg is something `kite run` can execute —
// a .star file, an existing file, a module directory (contains a manifest), or
// an @namespace/name reference — so it can be run without an explicit "run".
func looksLikeRunTarget(arg string) bool {
	if strings.HasPrefix(arg, "@") || strings.HasSuffix(arg, ".star") {
		return true
	}
	info, err := os.Stat(arg)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return true
	}
	// A directory is a run target only when it is a module.
	_, err = os.Stat(filepath.Join(arg, libkite.ManifestFile))
	return err == nil
}

// exitCodeFromError extracts an exit code from an error
func exitCodeFromError(err error) int {
	// Check for libkite errors with exit codes
	var scriptErr *libkite.ScriptError
	if errors.As(err, &scriptErr) {
		return scriptErr.ExitCode
	}

	// Check for exit() calls
	var exitErr *libkite.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}

	// Default to generic error code
	return 1
}

// GetOutputFormat returns the current output format
func GetOutputFormat() string {
	return outputFormat
}

// IsDebugMode returns whether debug mode is enabled
func IsDebugMode() bool {
	return debugMode
}

// IsDryRun returns whether dry-run mode is enabled
func IsDryRun() bool {
	return dryRun
}

// GetTimeout returns the configured timeout
func GetTimeout() int {
	return timeout
}

// execContext returns a context and cancel func derived from the given
// timeout in seconds. A timeout of 0 or less returns context.Background and
// a no-op cancel.
func execContext(timeoutSec int) (context.Context, context.CancelFunc) {
	if timeoutSec <= 0 {
		return context.Background(), func() {}
	}
	return context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
}

// GetVariables returns the configured variables
func GetVariables() []string {
	return variables
}

// GetVarFiles returns the configured variable files
func GetVarFiles() []string {
	return varFiles
}

// PrintDebug prints debug messages if debug mode is enabled
func PrintDebug(format string, args ...interface{}) {
	if debugMode {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// configPermissions returns the permission profiles defined in config.yaml's
// `permissions:` map, or nil if none are configured.
func configPermissions() map[string]permissions.ProfileSpec {
	vs := varstore.New()
	if err := vs.LoadDefaults(); err != nil {
		return nil
	}
	return vs.Permissions
}

// GetPermissions resolves the --permissions flag against the config-defined
// profiles. With no flag, it falls back to the configured `default` profile, or
// deny-all. Used by commands without a single script file (exec, repl, test).
func GetPermissions() (*libkite.PermissionConfig, error) {
	return permissions.Resolve(permissionsMode, configPermissions())
}

// GetSandbox resolves the sandbox profile from two equivalent inputs:
//
//  1. The --sandbox CLI flag (typed by the user explicitly invoking kite).
//  2. The STARKITE_SECURITY_SANDBOX env var (the path shebang-launched
//     scripts use, since shebang lines can't easily carry flags).
//
// The flag wins when both are set — same precedence convention as the
// other env-resolved options (STARKITE_DEBUG, etc.). An unset flag and
// unset env var means "no sandbox" (zero Profile). When either is set
// but no sandbox backend is registered (macOS/Windows), returns
// PlatformError so the caller can surface a clear message.
func GetSandbox() (sandbox.Profile, error) {
	value := sandboxMode
	if value == "" {
		value = os.Getenv(sandbox.EngagementEnvVar)
	}
	if value == "" {
		return sandbox.Profile{}, nil
	}
	if !sandbox.Available() {
		return sandbox.Profile{}, sandbox.PlatformError()
	}
	return sandbox.LoadProfile(value)
}

// MaybeHandoffToSandbox checks whether a sandbox is requested (via
// --sandbox flag or STARKITE_SECURITY_SANDBOX env var) and routes
// execution through sandbox.Backend if so. Returns (true, err) when
// the backend handled execution (caller must return immediately,
// propagating err); (false, nil) when the caller should continue
// running natively; (false, err) when the sandbox config was invalid.
//
// Subcommands that re-enter starkite (e.g. via gVisor's argv-rewrite
// for boot/gofer) check sandbox.InsideEnvVar to avoid recursion.
func MaybeHandoffToSandbox(ctx context.Context) (bool, error) {
	if os.Getenv(sandbox.InsideEnvVar) == "1" {
		return false, nil
	}
	profile, err := GetSandbox()
	if err != nil {
		return false, err
	}
	if profile.IsZero() {
		return false, nil
	}
	return true, sandbox.Backend.Run(ctx, sandbox.ExecSpec{Profile: profile})
}

// shouldHandoff returns true if this invocation should attempt edition handoff.
// Edition management and self-update commands always run in the base binary.
func shouldHandoff() bool {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "edition", "update":
			return false
		}
	}
	return true
}

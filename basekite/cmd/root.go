package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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

	// sandboxProfileFlags are boolean aliases for --sandbox=<rung>, one per
	// built-in rung (e.g. --sandbox-opaque == --sandbox=opaque). At most one
	// sandbox selector — including --sandbox — may be set.
	sandboxProfileFlags = []string{
		"sandbox-opaque", "sandbox-net-access", "sandbox-host",
	}
)

// sandboxAlias is a bool-style pflag.Value that sets the shared --sandbox
// target to a fixed rung name when its flag is present, so --sandbox-opaque
// is exactly --sandbox=opaque.
type sandboxAlias struct{ profile string }

func (sandboxAlias) String() string   { return "" }
func (sandboxAlias) Type() string     { return "bool" }
func (sandboxAlias) IsBoolFlag() bool { return true }
func (a sandboxAlias) Set(string) error {
	sandboxMode = a.profile
	return nil
}

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
	Use:   "kite [target]",
	Short: "Starkite - A powerful automation tool for cloud-native infrastructure",
	Long: `kite is the CLI for starkite, a powerful automation tool for cloud-native infrastructure.
It provides a unified scripting interface using Starlark, a Python-like language, to execute
commands across local machines, remote servers, containers, and Kubernetes clusters.

Examples:
  # Execute a script (these are equivalent)
  kite ./script.star
  kite run ./script.star
  ./script.star              # with shebang: #!/usr/bin/env kite

  # Execute inline Starlark code
  kite exec 'print(exec("hostname"))'

  # Interactive REPL
  kite repl

  # Pipe output to other tools
  kite ./manifest.star | kubectl apply -f -
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
		"Permission profile name: a built-in (deny-all|allow-fs|allow-net|allow-local|allow-all) "+
			"or a profile defined in config.yaml's permissions: section")

	// Boolean aliases for the built-in profiles, e.g. --allow-fs == --permissions=allow-fs.
	for _, p := range permissionProfileFlags {
		rootCmd.PersistentFlags().Var(permissionAlias{p}, p, fmt.Sprintf("Alias for --permissions=%s", p))
		rootCmd.PersistentFlags().Lookup(p).NoOptDefVal = "true" // usable as a bare flag
	}

	// Sandbox flag — Linux only; non-Linux returns a clear error.
	// `--sandbox` (no value) resolves to the built-in "default" profile;
	// `--sandbox=<name>` resolves a built-in or a profile defined in
	// config.yaml's sandbox: section. Shebang-launched scripts
	// can use STARKITE_SECURITY_SANDBOX env var instead — see
	// docs/guides/sandbox.md.
	rootCmd.PersistentFlags().StringVar(&sandboxMode, "sandbox", "",
		"Sandbox profile name for OS-level isolation (Linux): a built-in rung "+
			"(opaque|net-access|host) or a profile defined in config.yaml's sandbox: "+
			"section. --sandbox alone selects the config-defined \"default\" profile, "+
			"or the opaque rung when none is defined.")
	rootCmd.PersistentFlags().Lookup("sandbox").NoOptDefVal = "default"

	// Boolean aliases for the built-in rungs, e.g. --sandbox-opaque == --sandbox=opaque.
	for _, f := range sandboxProfileFlags {
		rung := strings.TrimPrefix(f, "sandbox-")
		rootCmd.PersistentFlags().Var(sandboxAlias{rung}, f, fmt.Sprintf("Alias for --sandbox=%s", rung))
		rootCmd.PersistentFlags().Lookup(f).NoOptDefVal = "true" // usable as a bare flag
	}

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnvDefaults()
		if err := checkPermissionFlagConflict(rootCmd.PersistentFlags()); err != nil {
			return err
		}
		return checkSandboxFlagConflict(rootCmd.PersistentFlags())
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

// checkSandboxFlagConflict rejects setting more than one sandbox selector —
// the rung aliases (--sandbox-opaque, …) and --sandbox all target the same
// value, so combining them is contradictory.
func checkSandboxFlagConflict(flags *pflag.FlagSet) error {
	var set []string
	if flags.Lookup("sandbox").Changed {
		set = append(set, "--sandbox")
	}
	for _, f := range sandboxProfileFlags {
		if flags.Lookup(f).Changed {
			set = append(set, "--"+f)
		}
	}
	if len(set) > 1 {
		return fmt.Errorf("only one sandbox flag may be set; got %s", strings.Join(set, " and "))
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

	// Implicit run: if the first arg is a run target (a .star file, an existing
	// path reference, an installed namespace/name identity, or a .star arg) rather
	// flag or subcommand, insert "run".
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		if !strings.HasPrefix(firstArg, "-") && looksLikeRunTarget(firstArg) {
			os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		// ExitError with code 0 — silent success (e.g. exit(0))
		if exitErr, ok := errors.AsType[*libkite.ExitError](err); ok {
			return exitErr.Code
		}

		// All other errors — print to stderr
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return exitCodeFromError(err)
	}
	return 0
}

// looksLikeRunTarget reports whether arg is something `kite run` can execute —
// a path reference (./, ../, /, .\, ..\, \), an installed namespace/name identity, or a
// .star argument — so it can be run without an explicit "run". Subcommand names
// never contain a slash or backslash, and malformed forms (e.g. a bare "file.star" missing
// its path prefix) still route to run so resolveRunTarget can produce the
// precise error.
func looksLikeRunTarget(arg string) bool {
	return isPathArg(arg) || strings.Contains(arg, "/") || strings.Contains(arg, "\\") || strings.HasSuffix(arg, ".star")
}

// exitCodeFromError extracts an exit code from an error
func exitCodeFromError(err error) int {
	// Check for libkite errors with exit codes
	if scriptErr, ok := errors.AsType[*libkite.ScriptError](err); ok {
		return scriptErr.ExitCode
	}

	// Check for exit() calls
	if exitErr, ok := errors.AsType[*libkite.ExitError](err); ok {
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
func PrintDebug(format string, args ...any) {
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

// loadVarStore builds the variable store every script-executing command uses,
// applying the documented priority order: environment (lowest), config-file
// defaults (~/.starkite/config.yaml, ./config.yaml), --var-file files, then
// --var flags (highest).
func loadVarStore() (*varstore.Vars, error) {
	vs := varstore.New()
	vs.LoadFromEnv()
	if err := vs.LoadDefaults(); err != nil {
		return nil, fmt.Errorf("failed to load default config: %w", err)
	}
	if err := vs.LoadFromFiles(varFiles); err != nil {
		return nil, fmt.Errorf("failed to load var files: %w", err)
	}
	if err := vs.LoadFromCLI(variables); err != nil {
		return nil, fmt.Errorf("failed to parse variables: %w", err)
	}
	return vs, nil
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
// GetSandbox resolves the active sandbox profile from the CLI or environment.
// Supports compound syntax (<driver>:<profile>, e.g. "podman:ci-builder", "seatbelt:dev")
// as well as simple profile names ("opaque", "strict") and bare flags ("default").
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

	driverOverride := ""
	profileName := value
	if parts := strings.SplitN(value, ":", 2); len(parts) == 2 {
		driverOverride = parts[0]
		profileName = parts[1]
	}

	profile, err := sandbox.LoadProfile(profileName)
	if err != nil {
		return sandbox.Profile{}, err
	}
	if driverOverride != "" {
		profile.Driver = driverOverride
	}
	return profile, nil
}

// MaybeHandoffToSandbox checks whether a sandbox is requested (via
// --sandbox flag or STARKITE_SECURITY_SANDBOX env var) and routes
// execution through the resolved sandbox driver.
//
// Returns:
//   - (false, nil) when execution continues natively (either unsandboxed or under in-process native confinement)
//   - (true, err) when the sandbox driver handled execution in a subprocess/container (caller must return/exit immediately)
//   - (false, err) when sandbox initialization failed.
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

	// Legacy Backend support if registered
	if sandbox.Backend != nil {
		return true, sandbox.Backend.Run(ctx, sandbox.ExecSpec{Profile: profile})
	}

	driver, err := sandbox.Resolve(profile.Driver)
	if err != nil {
		return false, err
	}

	cwd, _ := os.Getwd()
	spec := &sandbox.ExecutionSpec{
		Command:     os.Args,
		Cwd:         cwd,
		Env:         os.Environ(),
		Network:     profile.Network,
		Mounts:      profile.Mounts,
		MaxMemoryMB: profile.MaxMemoryMB,
		Timeout:     profile.Timeout,
		Image:       profile.Image,
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
	}

	// Case 1: In-process native driver (e.g. Landlock on Linux, Seatbelt on macOS)
	// Apply rules to current running process and continue native execution!
	if inProc, ok := driver.(sandbox.InProcessDriver); ok {
		if err := inProc.ApplyInProcess(spec); err != nil {
			return false, fmt.Errorf("failed to apply in-process sandbox (%s): %w", driver.Name(), err)
		}
		os.Setenv(sandbox.InsideEnvVar, "1")
		return false, nil
	}

	// Case 2: Out-of-process container / external provider (podman, docker, nerdctl, gvisor)
	// Execute the command wrapped inside the sandbox container.
	os.Setenv(sandbox.InsideEnvVar, "1")
	spec.Env = append(spec.Env, fmt.Sprintf("%s=1", sandbox.InsideEnvVar))

	res, execErr := driver.Exec(ctx, spec)
	if execErr != nil {
		if res != nil && res.ExitCode != 0 {
			os.Exit(res.ExitCode)
		}
		return true, execErr
	}
	if res.ExitCode != 0 {
		os.Exit(res.ExitCode)
	}
	return true, nil
}

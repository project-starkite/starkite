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

	// Sandbox flags
	sandboxed      bool
	sandboxProfile string
	sandboxDriver  string

	// sandboxProfileAliases maps shortcut flag names to their target profile names.
	sandboxProfileAliases = map[string]string{
		"sandbox-opaque":     "opaque",
		"sandbox-net":        "net-access",
		"sandbox-net-access": "net-access",
		"sandbox-host":       "host",
	}

	sandboxProfileFlags = []string{
		"sandbox-opaque", "sandbox-net", "sandbox-net-access", "sandbox-host",
	}
)

// sandboxAlias is a bool-style pflag.Value that sets the shared --sandbox-profile
// target to a fixed profile name when its flag is present, so --sandbox-opaque
// is exactly --sandbox-profile=opaque.
type sandboxAlias struct{ profile string }

func (sandboxAlias) String() string     { return "" }
func (sandboxAlias) Type() string       { return "bool" }
func (a sandboxAlias) IsBoolFlag() bool { return true }
func (a sandboxAlias) Set(string) error {
	sandboxProfile = a.profile
	sandboxed = true
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
  kite run ./deploy.star
  kite ./deploy.star
  ./deploy.star       # with shebang: #!/usr/bin/env kite

  # Run with sandboxing enabled
  kite ./deploy.star --sandboxed
  kite ./deploy.star --sandbox-opaque
  kite ./deploy.star --sandbox-profile=ci-builder

  # Run with permissions
  kite ./deploy.star --permissions=allow-net
  kite ./deploy.star --allow-net

  # Launch interactive REPL
  kite repl

  # Format starlark files
  kite fmt ./scripts/

  # Execute inline Starlark code
  kite exec 'print("Hello from Starlark!")'

  # Test starlark scripts
  kite test ./tests/
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
	// Persistent flags (available to all commands)
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json, yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode with detailed logging")
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

	// Sandbox flags
	rootCmd.PersistentFlags().BoolVar(&sandboxed, "sandboxed", false,
		"Enable OS-level sandbox isolation using the default profile")

	rootCmd.PersistentFlags().StringVar(&sandboxProfile, "sandbox-profile", "",
		"Sandbox profile name: a built-in (opaque|net-access|host) or a profile defined in config.yaml's sandbox: section")

	rootCmd.PersistentFlags().StringVar(&sandboxDriver, "sandbox-driver", "",
		"Sandbox execution driver (auto|landlock|seatbelt|podman|docker|nerdctl|gvisor). "+
			"Overrides the driver configured in the sandbox profile.")

	// Boolean shortcut aliases for the built-in rungs
	for flagName, profile := range sandboxProfileAliases {
		rootCmd.PersistentFlags().Var(sandboxAlias{profile}, flagName, fmt.Sprintf("Shortcut for --sandbox-profile=%s", profile))
		rootCmd.PersistentFlags().Lookup(flagName).NoOptDefVal = "true" // usable as a bare flag
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

// checkSandboxFlagConflict rejects setting more than one sandbox profile selector —
// the profile aliases (--sandbox-opaque, …) and --sandbox-profile all target the same
// value, so combining them is contradictory.
func checkSandboxFlagConflict(flags *pflag.FlagSet) error {
	var set []string
	if flags.Lookup("sandbox-profile").Changed {
		set = append(set, "--sandbox-profile")
	}
	for _, f := range sandboxProfileFlags {
		if flags.Lookup(f).Changed {
			set = append(set, "--"+f)
		}
	}
	if len(set) > 1 {
		return fmt.Errorf("only one sandbox profile flag may be set; got %s", strings.Join(set, " and "))
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

	if !flags.Lookup("sandbox-profile").Changed {
		if v := os.Getenv(sandbox.ProfileEnvVar); v != "" {
			sandboxProfile = v
		}
	}

	if !flags.Lookup("sandbox-driver").Changed {
		if v := os.Getenv(sandbox.DriverEnvVar); v != "" {
			sandboxDriver = v
		}
	}
}

// RootCmd returns the root cobra command for edition command registration.
func RootCmd() *cobra.Command {
	return rootCmd
}

// isSubcommandName reports whether arg matches a registered subcommand or alias.
func isSubcommandName(arg string) bool {
	for _, c := range rootCmd.Commands() {
		if c.Name() == arg || c.HasAlias(arg) {
			return true
		}
	}
	return arg == "help" || arg == "completion"
}

// needsImplicitRun reports whether args contain a run target without an explicit subcommand.
func needsImplicitRun(args []string) bool {
	if len(args) <= 1 {
		return false
	}
	hasRunTarget := false
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if isSubcommandName(arg) {
			return false
		}
		if looksLikeRunTarget(arg) {
			hasRunTarget = true
		}
	}
	return hasRunTarget
}

// Execute runs the root command and returns the exit code
func Execute() int {
	// Let edition binaries register their commands before execution.
	if RegisterEditionCommands != nil {
		RegisterEditionCommands(rootCmd)
	}

	// Implicit run: if a run target is specified without an explicit subcommand,
	// insert "run" so flag placement before or after the target works seamlessly.
	if needsImplicitRun(os.Args) {
		os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
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

// GetSandbox resolves the active sandbox profile from the CLI flags or environment.
// --sandboxed (bool), --sandbox-profile (or STARKITE_SANDBOX_PROFILE), and --sandbox-driver
// (or STARKITE_SANDBOX_DRIVER) configure sandbox execution.
func GetSandbox() (sandbox.Profile, error) {
	profileName := sandboxProfile
	if profileName == "" {
		profileName = os.Getenv(sandbox.ProfileEnvVar)
	}
	driverOverride := sandboxDriver
	if driverOverride == "" {
		driverOverride = os.Getenv(sandbox.DriverEnvVar)
	}

	// If neither --sandboxed, a profile, nor a driver is specified, execution is unsandboxed.
	if !sandboxed && profileName == "" && driverOverride == "" {
		return sandbox.Profile{}, nil
	}

	if !sandbox.Available() {
		return sandbox.Profile{}, sandbox.PlatformError()
	}

	// If sandboxed is true or driver is set, but no profile was named, default to "default".
	if profileName == "" {
		profileName = sandbox.DefaultProfileName
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

package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/project-starkite/starkite/basekite/varstore"
	"github.com/project-starkite/starkite/basekite/version"
	"github.com/project-starkite/starkite/libkite"
	runtimemod "github.com/project-starkite/starkite/libkite/modules/runtime"
	"github.com/project-starkite/starkite/libkite/sandbox"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	outputFormat string
	debugMode    bool
	dryRun       bool
	timeout      int
	variables    []string
	varFiles     []string

	// Sandbox flags
	sandboxEnabled   bool
	sandboxProfile   string
	sandboxDriver    string
	sandboxPathRead  []string
	sandboxPathWrite []string
	sandboxCallAllow []string
	sandboxCallDeny  []string
	sandboxExecAllow []string
	sandboxExecDeny  []string
	sandboxNet       bool

	// Environment flags
	envFlags   []string
	envInherit bool
)

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

  # Run with sandboxing enabled (default)
  kite ./deploy.star
  kite ./deploy.star --sandbox-profile=sandbox-net
  kite ./deploy.star --sandbox-net
  kite ./deploy.star --sandbox-path-read=/data

  # Run unconfined
  kite ./deploy.star --sandbox-enabled=false

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
	if runtimemod.Version == "dev" && version.Version != "" && version.Version != "0.0.1" {
		runtimemod.Version = version.Version
	}

	// Persistent flags (available to all commands)
	rootCmd.PersistentFlags().StringVarP(&outputFormat, "output", "o", "text", "Output format (text, json, yaml)")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false, "Enable debug mode with detailed logging")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview commands without executing")
	rootCmd.PersistentFlags().IntVar(&timeout, "timeout", 300, "Script execution timeout in seconds")
	rootCmd.PersistentFlags().StringArrayVar(&variables, "var", nil, "Set script variable: --var key=value")
	rootCmd.PersistentFlags().StringArrayVar(&varFiles, "var-file", nil, "Load variables from YAML file: --var-file=values.yaml")

	// Sandbox flags
	rootCmd.PersistentFlags().BoolVar(&sandboxEnabled, "sandbox-enabled", true,
		"Enable sandbox confinement (default true; pass --sandbox-enabled=false to run unconfined)")
	rootCmd.PersistentFlags().StringVar(&sandboxProfile, "sandbox-profile", "",
		"Sandbox profile name: a built-in (deny-all|sandbox-path|sandbox-exec|sandbox-net|unconfined) or a profile in config.yaml")
	rootCmd.PersistentFlags().StringVar(&sandboxDriver, "sandbox-driver", "",
		"Sandbox execution driver (auto|landlock|seatbelt|podman|docker|nerdctl|gvisor). Overrides the driver configured in the sandbox profile.")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxPathRead, "sandbox-path-read", nil,
		"Grant read access to host path (repeatable)")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxPathWrite, "sandbox-path-write", nil,
		"Grant read and write access to host path (repeatable)")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxCallAllow, "sandbox-call-allow", nil,
		"Allow Starlark API call pattern, e.g. os.env or http.* (repeatable)")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxCallDeny, "sandbox-call-deny", nil,
		"Deny Starlark API call pattern (repeatable)")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxExecAllow, "sandbox-exec-allow", nil,
		"Allow execution of binary name/path or '*' (repeatable, comma-separated)")
	rootCmd.PersistentFlags().StringArrayVar(&sandboxExecDeny, "sandbox-exec-deny", nil,
		"Deny execution of binary name/path (repeatable, comma-separated)")
	rootCmd.PersistentFlags().BoolVar(&sandboxNet, "sandbox-net", false,
		"Enable outbound network access and unveil host DNS/TLS assets")

	// Environment flags
	rootCmd.PersistentFlags().StringArrayVarP(&envFlags, "env", "e", nil,
		"Set or forward environment variables (KEY=VALUE or KEY)")
	rootCmd.PersistentFlags().BoolVar(&envInherit, "env-inherit", false,
		"Inherit all host environment variables (bypasses zero-trust env scrubbing)")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		applyEnvDefaults()
		return nil
	}
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

// configSandbox returns the sandbox configuration defined in config.yaml's
// `sandbox:` section, or nil if none are configured.
func configSandbox() *sandbox.SandboxConfig {
	vs := varstore.New()
	if err := vs.LoadDefaults(); err != nil {
		return nil
	}
	return vs.Sandbox
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

func currentCLIOverrides() sandbox.CLIOverrides {
	var enabledOverride *bool
	if f := rootCmd.PersistentFlags().Lookup("sandbox-enabled"); f != nil && f.Changed {
		v := sandboxEnabled
		enabledOverride = &v
	} else if !sandboxEnabled {
		v := false
		enabledOverride = &v
	}

	drv := strings.TrimSpace(sandboxDriver)
	if drv == "" {
		drv = strings.TrimSpace(os.Getenv(sandbox.DriverEnvVar))
	}

	return sandbox.CLIOverrides{
		Enabled:   enabledOverride,
		Driver:    drv,
		PathRead:  sandboxPathRead,
		PathWrite: sandboxPathWrite,
		CallAllow: sandboxCallAllow,
		CallDeny:  sandboxCallDeny,
		ExecAllow: sandboxExecAllow,
		ExecDeny:  sandboxExecDeny,
		Net:       sandboxNet,
	}
}

// ResolveActiveProfile resolves the effective sandbox.Profile for the current CLI invocation.
// By default (when sandboxEnabled is true and no profile is specified), it resolves "default"
// (which falls back to "deny-all" if not defined in config.yaml) and applies all CLI overrides.
func ResolveActiveProfile() (sandbox.Profile, error) {
	overrides := currentCLIOverrides()

	// If --sandbox-enabled=false was set without an explicit profile, return unconfined with overrides.
	if overrides.Enabled != nil && !*overrides.Enabled && strings.TrimSpace(sandboxProfile) == "" && strings.TrimSpace(os.Getenv(sandbox.ProfileEnvVar)) == "" {
		p, err := sandbox.LoadProfile(sandbox.ProfileUnconfined)
		if err != nil {
			return sandbox.Profile{}, err
		}
		if err := p.ApplyCLIOverrides(overrides); err != nil {
			return sandbox.Profile{}, err
		}
		return p, nil
	}

	value := strings.TrimSpace(sandboxProfile)
	if value == "" {
		value = strings.TrimSpace(os.Getenv(sandbox.ProfileEnvVar))
	}
	if value == "" {
		value = sandbox.DefaultProfileName
	}

	cfg := configSandbox()

	p, err := sandbox.ResolveProfile(value, cfg)
	if err != nil {
		return sandbox.Profile{}, err
	}
	if err := p.ApplyCLIOverrides(overrides); err != nil {
		return sandbox.Profile{}, err
	}
	return p, nil
}

// GetSandbox resolves the active sandbox profile from the CLI flags or environment.
// It returns a zero Profile when the effective profile has Enabled=false (unconfined).
func GetSandbox() (sandbox.Profile, error) {
	profile, err := ResolveActiveProfile()
	if err != nil {
		return sandbox.Profile{}, err
	}
	if !profile.Enabled {
		return sandbox.Profile{}, nil
	}
	return profile, nil
}

// stripSandboxArgs removes sandbox-related CLI flags from an argument slice.
func stripSandboxArgs(args []string) []string {
	valueFlags := map[string]bool{
		"sandbox-profile":    true,
		"sandbox-driver":     true,
		"sandbox-path-read":  true,
		"sandbox-path-write": true,
		"sandbox-call-allow": true,
		"sandbox-call-deny":  true,
		"sandbox-exec-allow": true,
		"sandbox-exec-deny":  true,
	}
	boolFlags := map[string]bool{
		"sandbox-enabled": true,
		"sandbox-net":     true,
	}

	var out []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			out = append(out, arg)
			continue
		}

		trimmed := strings.TrimLeft(arg, "-")
		parts := strings.SplitN(trimmed, "=", 2)
		flagName := parts[0]

		if valueFlags[flagName] {
			if len(parts) == 1 {
				// Flag value is in the next arg if present
				i++
			}
			continue
		}

		if boolFlags[flagName] {
			continue
		}

		out = append(out, arg)
	}
	return out
}

// ResolveRuntimeEnv compiles the effective runtime environment table and sorted slice
// by synthesizing Tier 1 (baseline or inherited), Tier 2 (profile config), and Tier 3 (CLI flags).
func ResolveRuntimeEnv(prof *sandbox.Profile) (map[string]string, []string, error) {
	if prof == nil {
		prof = &sandbox.Profile{}
	}
	envMap, err := prof.SynthesizeEnvMap(os.Environ(), envFlags, envInherit)
	if err != nil {
		return nil, nil, err
	}
	envSlice, err := prof.SynthesizeEnv(os.Environ(), envFlags, envInherit)
	if err != nil {
		return nil, nil, err
	}
	return envMap, envSlice, nil
}

var (
	activePreloadedClosureMu sync.RWMutex
	activePreloadedClosure   *libkite.StaticLoadClosure
)

// GetActivePreloadedClosure returns the statically preloaded module closure
// discovered prior to sandbox confinement, if any.
func GetActivePreloadedClosure() *libkite.StaticLoadClosure {
	activePreloadedClosureMu.RLock()
	defer activePreloadedClosureMu.RUnlock()
	return activePreloadedClosure
}

// SetActivePreloadedClosure records the statically preloaded module closure.
func SetActivePreloadedClosure(closure *libkite.StaticLoadClosure) {
	activePreloadedClosureMu.Lock()
	defer activePreloadedClosureMu.Unlock()
	activePreloadedClosure = closure
}

// MaybeHandoffToSandbox checks whether a sandbox is requested (via
func isRunningInTest() bool {
	if os.Getenv("STARKITE_TEST_FORCE_SANDBOX") != "" {
		return false
	}
	base := filepath.Base(os.Args[0])
	return strings.HasSuffix(base, ".test") || strings.HasSuffix(base, ".test.exe")
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
	return MaybeHandoffToSandboxWithScript(ctx, "")
}

// MaybeHandoffToSandboxWithScript checks whether a sandbox is requested and routes
// execution through the resolved sandbox driver, passing scriptPath for runtime asset baseline protection.
func MaybeHandoffToSandboxWithScript(ctx context.Context, scriptPath string) (bool, error) {
	if os.Getenv(sandbox.InsideEnvVar) == "1" {
		return false, nil
	}
	profile, err := GetSandbox()
	if err != nil {
		return false, err
	}
	if profile.IsZero() || !profile.Enabled {
		return false, nil
	}

	cwd, _ := os.Getwd()
	if scriptPath != "" {
		if closure, err := libkite.ResolveStaticLoadClosure(scriptPath, cwd, nil); err == nil && closure != nil {
			SetActivePreloadedClosure(closure)
		}
	}

	if isRunningInTest() {
		return false, nil
	}

	// Legacy Backend support if registered
	if sandbox.Backend != nil {
		return true, sandbox.Backend.Run(ctx, sandbox.ExecSpec{Profile: profile})
	}

	explicitDriver := strings.TrimSpace(sandboxDriver) != "" ||
		strings.TrimSpace(os.Getenv(sandbox.DriverEnvVar)) != "" ||
		(profile.Driver != "" && profile.Driver != sandbox.DriverDefault && profile.Driver != "auto")

	drvName := profile.Driver
	if drvName == "" || drvName == sandbox.DriverDefault || drvName == "auto" {
		if !explicitDriver && sandbox.Backend == nil {
			d, err := sandbox.AutoDetect()
			if err != nil {
				// No host in-process driver available (e.g. Windows) and no container driver was explicitly requested.
				return false, nil
			}
			if _, isInProc := d.(sandbox.InProcessDriver); !isInProc {
				// Driver is container-based but not explicitly requested; continue in-process with PermissionChecker.
				return false, nil
			}
			drvName = d.Name()
		}
	}

	driver, err := sandbox.Resolve(drvName)
	if err != nil {
		return false, err
	}

	envSlice, err := profile.SynthesizeEnv(os.Environ(), envFlags, envInherit)
	if err != nil {
		return false, err
	}
	spec := &sandbox.ExecutionSpec{
		Command:          os.Args,
		Cwd:              cwd,
		Env:              envSlice,
		Network:          profile.Network,
		Mounts:           profile.Mounts,
		MaxMemoryMB:      profile.MaxMemoryMB,
		Timeout:          profile.Timeout,
		Image:            profile.Image,
		ScriptFile:       scriptPath,
		NoExec:           profile.NoExec,
		AllowedExecPaths: profile.AllowedExecPaths,
		Stdin:            os.Stdin,
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
	}

	if closure := GetActivePreloadedClosure(); closure != nil {
		for _, p := range closure.ReadPaths {
			cleanP := filepath.Clean(p)
			alreadyMounted := false
			for _, m := range spec.Mounts {
				if filepath.Clean(m.Destination) == cleanP {
					alreadyMounted = true
					break
				}
			}
			if !alreadyMounted {
				spec.Mounts = append(spec.Mounts, sandbox.Mount{
					Source:      cleanP,
					Destination: cleanP,
					Type:        sandbox.MountBind,
					Mode:        sandbox.MountRO,
				})
			}
		}
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

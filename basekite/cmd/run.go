package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/project-starkite/starkite/libkite"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.starlark.net/syntax"
)

var runCmd = &cobra.Command{
	Use:   "run <target>",
	Short: "Execute a starkite script",
	Long: `Execute a starkite script file or module directory.

The script is written in Starlark (a deterministic Python-like language) and
typically uses the .star extension.

You can also run scripts directly without the 'run' subcommand:
  kite ./script.star
  ./script.star    # with shebang: #!/usr/bin/env kite

Variables can be injected from multiple sources (highest to lowest priority):
  1. CLI flags:      --var key=value
  2. Variable files: --var-file=values.yaml (repeatable)
  3. Default config: ~/.starkite/config.yaml and ./config.yaml
  4. Environment:    STARKITE_VAR_<NAME>=value

Sandbox & Capabilities (enabled by default under the 'deny-all' profile):
  --sandbox-profile=<name>       Select sandbox profile (deny-all, sandbox-path, sandbox-exec, sandbox-net, unconfined, or custom)
  --sandbox-path-read=<paths>    Grant read access to host paths (comma-separated or repeatable)
  --sandbox-path-write=<paths>   Grant read/write access to host paths (comma-separated or repeatable)
  --sandbox-exec-allow=<bins>    Allow execution of specific binaries or '*' (comma-separated or repeatable)
  --sandbox-exec-deny=<bins>     Deny execution of specific binaries (comma-separated or repeatable)
  --sandbox-call-allow=<rules>   Allow specific Starlark module calls (e.g. fs.read, http.client)
  --sandbox-call-deny=<rules>    Deny specific Starlark module calls
  --sandbox-net                  Grant outbound network access (equivalent to network: host)
  --sandbox-driver=<driver>      Select sandbox driver (auto, landlock, seatbelt, windows, podman, docker, nerdctl, gvisor)
  --sandbox-enabled=false        Disable sandbox confinement (run unconfined)
  --env, -e <KEY[=VALUE]>        Forward host environment variable (KEY) or inject assignment (KEY=VALUE)
  --env-inherit                  Inherit the full ambient host environment

Dry run mode:
  --dry-run                      Simulate execution without making changes

Examples:
  # Run a pure Starlark script (default zero-trust sandbox)
  kite run ./calc.star

  # Grant working directory read/write and outbound network access
  kite run ./deploy.star --sandbox-path-write=. --sandbox-net

  # Grant execution of specific CLI tools and forward KUBECONFIG
  kite run ./rollout.star --sandbox-exec-allow=kubectl,helm --sandbox-net --env KUBECONFIG

  # Run with variable overrides and a variable file
  kite run ./deploy.star --var env=prod --var replicas=5 --var-file=prod.yaml

  # Pipe generated manifests to kubectl
  kite ./manifest.star | kubectl apply -f -
`,
	DisableFlagParsing: true,
	Args:               cobra.MinimumNArgs(1),
	RunE:               runScript,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runScript(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "--help" || args[0] == "-h")) {
		return cmd.Help()
	}

	target, scriptArgs, err := parseScriptCommandLine(rootCmd.PersistentFlags(), args)
	if err != nil {
		return err
	}
	if target == "" {
		return &libkite.ScriptError{
			Message:  "missing required run target",
			ExitCode: libkite.ExitUsageError,
		}
	}

	// Resolve the run target: a script file, a directory module, or an
	// installed namespace/name. Module runs require a main() entry point.
	scriptPath, isModule, err := resolveRunTarget(target)
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitFileError,
		}
	}

	// Resolve the module's declared dependency closure into the cache (and write
	// mod.lock for an owned module) before execution, so load() of a declared
	// dependency resolves.
	if isModule {
		if err := resolveModuleDeps(filepath.Dir(scriptPath)); err != nil {
			if _, ok := errors.AsType[syntax.Error](err); ok {
				return &libkite.ScriptError{
					Message:  fmt.Sprintf("syntax error: %v", err),
					ExitCode: libkite.ExitScriptError,
				}
			}
			return &libkite.ScriptError{
				Message:  fmt.Sprintf("failed to resolve dependencies: %v", err),
				ExitCode: libkite.ExitFileError,
			}
		}
	} else {
		if err := resolveLooseDeps(scriptPath); err != nil {
			if _, ok := errors.AsType[syntax.Error](err); ok {
				return &libkite.ScriptError{
					Message:  fmt.Sprintf("syntax error: %v", err),
					ExitCode: libkite.ExitScriptError,
				}
			}
			return &libkite.ScriptError{
				Message:  fmt.Sprintf("failed to resolve dependencies: %v", err),
				ExitCode: libkite.ExitFileError,
			}
		}
	}

	// Read the resolved entry file.
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to read script: %v", err),
			ExitCode: libkite.ExitFileError,
		}
	}

	// Create and populate variable store before sandboxing
	// (allows the Go runtime to load defaults from ~/.starkite/config.yaml).
	varStore, err := loadVarStore()
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitConfigError,
		}
	}

	profile, err := ResolveActiveProfile()
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to resolve sandbox profile: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}
	policy := profile.PolicySummary(envFlags, envInherit, target)
	perms := profile.CompilePermissions()
	checker, _ := libkite.NewPermissionChecker(perms)

	preloaded := GetActivePreloadedClosure()
	if preloaded == nil && scriptPath != "" {
		if closure, err := libkite.ResolveStaticLoadClosure(scriptPath, filepath.Dir(scriptPath), nil); err == nil {
			preloaded = closure
			SetActivePreloadedClosure(preloaded)
		}
	}

	if err := libkite.ValidateClosureManifests(preloaded, policy, checker, filepath.Dir(scriptPath)); err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitScriptError,
		}
	}

	// Sandbox handoff: if STARKITE_SECURITY_SANDBOX is set, hand the
	// entire script execution to the OS-level sandbox backend and
	// return its result.
	if handled, err := MaybeHandoffToSandboxWithScript(context.Background(), scriptPath); handled || err != nil {
		return err
	}

	if debugMode {
		PrintDebug("Loaded variables: %v", varStore.All())
		if len(varStore.ProviderDefaults) > 0 {
			PrintDebug("Provider defaults: %v", varStore.ProviderDefaults)
		}
	}

	envMap, _, err := ResolveRuntimeEnv(&profile)
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitConfigError,
		}
	}

	// Create module config
	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
		Env:      envMap,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:        scriptPath,
		ScriptArgs:        scriptArgs,
		OutputFormat:      outputFormat,
		Debug:             debugMode,
		DryRun:            dryRun,
		VarStore:          varStore,
		Permissions:       perms,
		SandboxPolicy:     policy,
		Registry:          registry,
		Env:               envMap,
		EntryPoint:        "main",
		RequireEntryPoint: isModule,
		Preloaded:         preloaded,
	}

	// Create and run the runtime
	rt, err := libkite.New(cfg)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to create runtime: %v", err),
			ExitCode: libkite.ExitScriptError,
		}
	}
	defer rt.Cleanup()

	ctx, cancel := execContext(timeout)
	defer cancel()

	// Execute the script
	if err := rt.Execute(ctx, string(content)); err != nil {
		// Check if it's already a typed error
		if _, ok := err.(*libkite.ScriptError); ok {
			return err
		}
		if _, ok := err.(*libkite.ExitError); ok {
			return err
		}
		// Wrap other errors — use err.Error() as Message without Cause
		// to avoid double-nesting (ScriptError.Error() concatenates Message + Cause)
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("%v", err),
			ExitCode: libkite.ExitScriptError,
		}
	}

	// Strict unhandled arguments check: if arguments were passed to the script
	// but the script never called args.parse(), fail immediately with ExitUsageError.
	if sCtx := libkite.GetScriptArgs(rt.Thread()); sCtx != nil {
		if len(sCtx.RawArgs) > 0 && !sCtx.Parsed {
			return &libkite.ScriptError{
				Message: fmt.Sprintf("unhandled arguments: %s\n  Script '%s' did not process these arguments (args.parse() was not called).",
					strings.Join(sCtx.RawArgs, " "), scriptPath),
				ExitCode: libkite.ExitUsageError,
			}
		}
	}

	return nil
}

// parseKnownFlags parses known flags from args using flagSet,
// consuming only known flags and leaving all unknown flags and positional args in remainder.
func parseKnownFlags(flagSet *pflag.FlagSet, args []string) (remainder []string, err error) {
	var targetFound bool
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			remainder = append(remainder, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") && !targetFound {
			targetFound = true
			remainder = append(remainder, arg)
			continue
		}
		if targetFound && (arg == "--help" || arg == "-h") {
			remainder = append(remainder, arg)
			continue
		}
		if strings.HasPrefix(arg, "-") && arg != "-" {
			name := strings.TrimLeft(strings.Split(arg, "=")[0], "-")
			f := flagSet.Lookup(name)
			if f == nil && len(name) == 1 {
				f = flagSet.ShorthandLookup(name)
			}
			if f != nil {
				isZeroArg := strings.Contains(arg, "=") || f.Value.Type() == "bool" || f.NoOptDefVal != ""
				if isZeroArg {
					if err := flagSet.Parse([]string{arg}); err != nil {
						return nil, err
					}
				} else if i+1 < len(args) {
					if err := flagSet.Parse([]string{arg, args[i+1]}); err != nil {
						return nil, err
					}
					i++
				} else {
					return nil, flagSet.Parse([]string{arg})
				}
				continue
			}
		}
		remainder = append(remainder, arg)
	}
	return remainder, nil
}

// parseScriptCommandLine parses the command line arguments for the run command,
// extracting the script target and any forwarded script arguments.
func parseScriptCommandLine(flagSet *pflag.FlagSet, rawArgs []string) (target string, scriptArgs []string, err error) {
	rem, err := parseKnownFlags(flagSet, rawArgs)
	if err != nil {
		return "", nil, err
	}
	if len(rem) == 0 {
		return "", nil, nil
	}
	return rem[0], rem[1:], nil
}

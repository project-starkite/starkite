package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/permissions"
	"github.com/spf13/cobra"
	"go.starlark.net/syntax"
)

var runCmd = &cobra.Command{
	Use:   "run <target>",
	Short: "Execute a starkite script",
	Long: `Execute a starkite script file.

The script should be written in Starlark (a Python-like language) and typically
has the .star extension.

Note: You can also run scripts directly without the 'run' subcommand:
  kite ./script.star
  ./script.star    # with shebang: #!/usr/bin/env kite

Variables can be injected from multiple sources with the following priority
(highest to lowest):
  1. CLI flags:      --var key=value
  2. Variable files: --var-file=values.yaml (can specify multiple)
  3. Default config: ~/.starkite/config.yaml (always loaded if present)

Parameters can be passed to functions with:
  --param name=value
  --param-file=params.yaml

Dry run mode:
  --dry-run: Simulate execution without making changes

Permissions (default: interactive prompt for dangerous operations):
  --allow-all:       Allow all permissions
  --allow-fs:        Allow filesystem access (read/write)
  --allow-fs-read:   Allow filesystem read access only
  --allow-fs-write:  Allow filesystem write access only
  --allow-net:       Allow network access (HTTP, SSH, etc.)
  --allow-exec:      Allow command execution
  --deny-all:        Deny all operations requiring permission
  --trust:           Trust script and prompt only for explicitly denied ops

Examples:
  # Run a script
  kite run ./deploy.star

  # Run with variable overrides
  kite run ./deploy.star --var env=prod --var replicas=5

  # Run with variable file
  kite run ./deploy.star --var-file=prod.yaml

  # Dry run
  kite run ./deploy.star --dry-run

  # Allow network and filesystem
  kite run ./deploy.star --allow-net --allow-fs

  # Pipe output to kubectl
  kite ./manifest.star | kubectl apply -f -
`,
	Args: cobra.ExactArgs(1),
	RunE: runScript,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runScript(cmd *cobra.Command, args []string) error {
	// Resolve the run target: a script file, a directory module, or an
	// installed namespace/name. Module runs require a main() entry point.
	scriptPath, isModule, err := resolveRunTarget(args[0])
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

	// Sandbox handoff: if STARKITE_SECURITY_SANDBOX is set, hand the
	// entire script execution to the OS-level sandbox backend and
	// return its result.
	if handled, err := MaybeHandoffToSandbox(context.Background()); handled || err != nil {
		return err
	}

	// Read the resolved entry file.
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to read script: %v", err),
			ExitCode: libkite.ExitFileError,
		}
	}

	// Create and populate variable store
	varStore, err := loadVarStore()
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitConfigError,
		}
	}

	if debugMode {
		PrintDebug("Loaded variables: %v", varStore.All())
		if len(varStore.ProviderDefaults) > 0 {
			PrintDebug("Provider defaults: %v", varStore.ProviderDefaults)
		}
	}

	// Create module config
	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	perms, err := permissions.Resolve(permissionsMode, varStore.Permissions)
	if err != nil {
		return err
	}

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:        scriptPath,
		OutputFormat:      outputFormat,
		Debug:             debugMode,
		DryRun:            dryRun,
		VarStore:          varStore,
		Permissions:       perms,
		Registry:          registry,
		EntryPoint:        "main",
		RequireEntryPoint: isModule,
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

	return nil
}

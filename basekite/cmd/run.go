package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/project-starkite/starkite/libkite"
	"github.com/project-starkite/starkite/libkite/permissions"
	"github.com/spf13/cobra"
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
  4. Environment:    STARKITE_VAR_key=value
  5. Script default: var("key", "default")

Examples:
  # Run a script
  kite ./deploy.star
  kite run ./deploy.star

  # Run with variables
  kite ./deploy.star --var image_tag=v1.0.0 --var replicas=3

  # Run with variable file (merges with ~/.starkite/config.yaml)
  kite ./deploy.star --var-file=prod.yaml

  # Run with multiple variable files (later files override earlier)
  kite ./deploy.star --var-file=base.yaml --var-file=prod.yaml

  # Combine sources (CLI overrides all)
  kite ./deploy.star --var-file=base.yaml --var image_tag=v2.0.0

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
			return &libkite.ScriptError{
				Message:  fmt.Sprintf("failed to resolve dependencies: %v", err),
				ExitCode: libkite.ExitFileError,
			}
		}
	} else {
		if err := resolveLooseDeps(scriptPath); err != nil {
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

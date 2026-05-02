package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/base/varstore"
	"github.com/project-starkite/starkite/libkite"
)

var runCmd = &cobra.Command{
	Use:   "run <script.star>",
	Short: "Execute a starkite script",
	Long: `Execute a starkite script file.

The script should be written in Starlark (a Python-like language) and typically
has the .star extension.

Note: You can also run scripts directly without the 'run' subcommand:
  kite script.star
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
  kite deploy.star
  kite run deploy.star

  # Run with variables
  kite deploy.star --var image_tag=v1.0.0 --var replicas=3

  # Run with variable file (merges with ~/.starkite/config.yaml)
  kite deploy.star --var-file=prod.yaml

  # Run with multiple variable files (later files override earlier)
  kite deploy.star --var-file=base.yaml --var-file=prod.yaml

  # Combine sources (CLI overrides all)
  kite deploy.star --var-file=base.yaml --var image_tag=v2.0.0

  # Pipe output to kubectl
  kite manifest.star | kubectl apply -f -
`,
	Args: cobra.ExactArgs(1),
	RunE: runScript,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runScript(cmd *cobra.Command, args []string) error {
	scriptPath := args[0]

	// Check if file exists
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("script file not found: %s", scriptPath),
			ExitCode: libkite.ExitFileError,
		}
	}

	// Read script content
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to read script: %v", err),
			ExitCode: libkite.ExitFileError,
		}
	}

	// Create and populate variable store
	varStore := varstore.New()

	// Load from environment (lowest priority for external sources)
	varStore.LoadFromEnv()

	// Load default config (starkite.yaml) - second lowest priority
	if err := varStore.LoadDefaults(); err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to load default config: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}

	// Load from var files (medium priority)
	if err := varStore.LoadFromFiles(varFiles); err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to load var files: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}

	// Load from CLI (highest priority)
	if err := varStore.LoadFromCLI(variables); err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to parse variables: %v", err),
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

	perms, err := resolvePermissionsForScript(scriptPath)
	if err != nil {
		return err
	}

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:   scriptPath,
		OutputFormat: outputFormat,
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		Permissions:  perms,
		Registry:     registry,
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

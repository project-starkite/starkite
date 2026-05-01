package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/base/varstore"
	"github.com/project-starkite/starkite/libkite"
)

var execCmd = &cobra.Command{
	Use:   "exec <code>",
	Short: "Execute inline Starlark code",
	Long: `Execute inline Starlark code directly from the command line.

Examples:
  # Simple command execution
  kite exec 'print(local.exec("hostname").value)'

  # Multi-line code (use quotes)
  kite exec '
    result = local.exec("ls -la")
    print(result.value)
  '

  # With variables
  kite exec 'print(env("HOME"))' --var HOME=/custom/home
`,
	Args: cobra.ExactArgs(1),
	RunE: execCode,
}

func init() {
	rootCmd.AddCommand(execCmd)
}

func execCode(cmd *cobra.Command, args []string) error {
	code := args[0]

	// Create and populate variable store
	varStore := varstore.New()
	varStore.LoadFromEnv()
	if err := varStore.LoadFromCLI(variables); err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to parse variables: %v", err),
			ExitCode: libkite.ExitConfigError,
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

	// Create runtime configuration
	cfg := &libkite.Config{
		ScriptPath:   "<inline>",
		OutputFormat: outputFormat,
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		Permissions:  GetPermissions(),
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

	// Execute the code
	if err := rt.Execute(ctx, code); err != nil {
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

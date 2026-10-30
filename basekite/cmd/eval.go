package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/project-starkite/starkite/libkite"
	"github.com/spf13/cobra"
)

var evalCmd = &cobra.Command{
	Use:   "eval <code>",
	Short: "Evaluate inline Starlark code",
	Long: `Evaluate inline Starlark code directly from the command line.

Examples:
  # Simple expression evaluation
  kite eval 'print(1 + 2)'

  # Multi-line code execution
  kite eval '
    data = {"status": "ok", "items": [1, 2, 3]}
    print(json.encode(data))
  '

  # Evaluation with variable injection
  kite eval 'print(var_str("ENV"))' --var ENV=production

  # Sandboxed evaluation with network egress
  kite eval 'print(http.get("https://api.github.com/zen").body)' --sandbox-net-allow
`,
	Args: cobra.ExactArgs(1),
	RunE: evalCode,
}

func init() {
	rootCmd.AddCommand(evalCmd)
}

func evalCode(cmd *cobra.Command, args []string) error {
	code := args[0]

	// Create and populate variable store before sandboxing
	// (allows the Go runtime to load defaults from ~/.starkite/config.yaml).
	varStore, err := loadVarStore()
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitConfigError,
		}
	}

	if handled, err := MaybeHandoffToSandbox(context.Background()); handled || err != nil {
		return err
	}

	profile, err := ResolveActiveProfile()
	if err != nil {
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("failed to resolve sandbox profile: %v", err),
			ExitCode: libkite.ExitConfigError,
		}
	}
	perms := profile.CompilePermissions()
	envMap, _, err := ResolveRuntimeEnv(&profile)
	if err != nil {
		return &libkite.ScriptError{
			Message:  err.Error(),
			ExitCode: libkite.ExitConfigError,
		}
	}

	moduleConfig := &libkite.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
		Env:      envMap,
	}

	registry := NewRegistry(moduleConfig)

	cfg := &libkite.Config{
		ScriptPath:   "<inline>",
		OutputFormat: outputFormat,
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		Permissions:  perms,
		Registry:     registry,
		Env:          envMap,
		EntryPoint:   "main",
	}

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

	if err := rt.Execute(ctx, code); err != nil {
		if _, ok := errors.AsType[*libkite.ScriptError](err); ok {
			return err
		}
		if _, ok := errors.AsType[*libkite.ExitError](err); ok {
			return err
		}
		return &libkite.ScriptError{
			Message:  fmt.Sprintf("%v", err),
			ExitCode: libkite.ExitScriptError,
		}
	}

	return nil
}

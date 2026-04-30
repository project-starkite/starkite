package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/basekite/edition"
	"github.com/project-starkite/starkite/basekite/version"
	"github.com/project-starkite/starkite/starbase"
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
	trustMode   bool
	sandboxMode bool
)

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

	// SilenceErrors lets us handle error printing ourselves in Execute(),
	// so we can suppress ExitError messages and avoid double-printing.
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
	rootCmd.PersistentFlags().BoolVar(&trustMode, "trust", false, "Trust mode: allow all operations (default)")
	rootCmd.PersistentFlags().BoolVar(&sandboxMode, "sandbox", false, "Sandbox mode: restrict to safe operations only")
	rootCmd.MarkFlagsMutuallyExclusive("trust", "sandbox")

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

	// Handle shebang: if first arg looks like a script, insert "run" command
	if len(os.Args) > 1 {
		firstArg := os.Args[1]
		// Check if it's not a flag and looks like a script file
		if !strings.HasPrefix(firstArg, "-") {
			if strings.HasSuffix(firstArg, ".star") || isScriptFile(firstArg) {
				// Insert "run" as the command
				os.Args = append([]string{os.Args[0], "run"}, os.Args[1:]...)
			}
		}
	}

	if err := rootCmd.Execute(); err != nil {
		// ExitError with code 0 — silent success (e.g. exit(0))
		var exitErr *starbase.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}

		// All other errors — print to stderr
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return exitCodeFromError(err)
	}
	return 0
}

// isScriptFile checks if the path is an existing file (for shebang support)
func isScriptFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// exitCodeFromError extracts an exit code from an error
func exitCodeFromError(err error) int {
	// Check for starbase errors with exit codes
	var scriptErr *starbase.ScriptError
	if errors.As(err, &scriptErr) {
		return scriptErr.ExitCode
	}

	// Check for exit() calls
	var exitErr *starbase.ExitError
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

// GetPermissions returns the permission config based on CLI flags.
// Returns nil for trusted mode (default), which allows all operations.
func GetPermissions() *starbase.PermissionConfig {
	if sandboxMode {
		return starbase.SandboxedPermissions()
	}
	// Default and --trust both mean trusted mode (nil = allow all)
	return nil
}

// IsSandboxMode returns whether sandbox mode is enabled
func IsSandboxMode() bool {
	return sandboxMode
}

// IsTrustMode returns whether trust mode is explicitly enabled
func IsTrustMode() bool {
	return trustMode
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

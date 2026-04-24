package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/project-starkite/starkite/core/varstore"
	"github.com/project-starkite/starkite/core/version"
	"github.com/project-starkite/starkite/starbase"
)

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Start an interactive REPL",
	Long: `Start an interactive Read-Eval-Print-Loop (REPL) for starkite.

The REPL allows you to execute Starlark code interactively, test commands,
and explore the starkite API.

Special commands:
  .help     - Show help
  .exit     - Exit the REPL
  .clear    - Clear the screen
  .vars     - Show defined variables

Examples:
  # Start REPL
  kite repl

  # In REPL
  >>> result = local.exec("hostname")
  >>> print(result.value)
`,
	RunE: startRepl,
}

func init() {
	rootCmd.AddCommand(replCmd)
}

func startRepl(cmd *cobra.Command, args []string) error {
	// Create and populate variable store
	varStore := varstore.New()
	varStore.LoadFromEnv()
	_ = varStore.LoadFromCLI(variables)

	// Create module config
	moduleConfig := &starbase.ModuleConfig{
		DryRun:   dryRun,
		Debug:    debugMode,
		VarStore: varStore,
	}

	// Create registry with all modules
	registry := NewRegistry(moduleConfig)

	// Create runtime configuration
	cfg := &starbase.Config{
		ScriptPath:   "<repl>",
		OutputFormat: outputFormat,
		Debug:        debugMode,
		DryRun:       dryRun,
		VarStore:     varStore,
		Permissions:  GetPermissions(),
		Registry:     registry,
	}

	// Create runtime
	rt, err := starbase.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}
	defer rt.Cleanup()

	// Print welcome message
	fmt.Printf("starkite %s - Starlark Controller\n", version.Version)
	fmt.Println("Type .help for help, .exit to quit")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	var multilineBuffer strings.Builder
	inMultiline := false

	for {
		// Print prompt
		if inMultiline {
			fmt.Print("... ")
		} else {
			fmt.Print(">>> ")
		}

		// Read line
		line, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println()
			break
		}
		line = strings.TrimRight(line, "\r\n")

		// Handle special commands
		if !inMultiline {
			switch strings.TrimSpace(line) {
			case ".exit", ".quit":
				fmt.Println("Goodbye!")
				return nil
			case ".help":
				printReplHelp()
				continue
			case ".clear":
				fmt.Print("\033[H\033[2J")
				continue
			case ".vars":
				rt.PrintVariables()
				continue
			case "":
				continue
			}
		}

		// Handle multiline input
		if strings.HasSuffix(line, "\\") {
			multilineBuffer.WriteString(strings.TrimSuffix(line, "\\"))
			multilineBuffer.WriteString("\n")
			inMultiline = true
			continue
		}

		// Check for block start (ends with :)
		trimmed := strings.TrimSpace(line)
		if strings.HasSuffix(trimmed, ":") && !inMultiline {
			multilineBuffer.WriteString(line)
			multilineBuffer.WriteString("\n")
			inMultiline = true
			continue
		}

		// In multiline mode, empty line ends the block
		if inMultiline {
			if trimmed == "" {
				// Execute multiline code
				code := multilineBuffer.String()
				multilineBuffer.Reset()
				inMultiline = false

				ctx, cancel := execContext(timeout)
				err := rt.ExecuteRepl(ctx, code)
				cancel()
				if err != nil {
					fmt.Printf("Error: %v\n", err)
				}
				continue
			}
			multilineBuffer.WriteString(line)
			multilineBuffer.WriteString("\n")
			continue
		}

		// Execute single line
		ctx, cancel := execContext(timeout)
		err = rt.ExecuteRepl(ctx, line)
		cancel()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		}
	}

	return nil
}

func printReplHelp() {
	fmt.Print(`
starkite REPL Commands:
  .help     Show this help message
  .exit     Exit the REPL
  .quit     Exit the REPL
  .clear    Clear the screen
  .vars     Show all defined variables

Built-in Providers:
  local     Execute commands on the local machine
  ssh       Execute commands on remote machines via SSH

Built-in Functions:
  print()           Print to stdout
  printf()          Formatted print
  sprintf()         Format a string
  env()             Get environment variable
  setenv()          Set environment variable
  json.encode()     Encode to JSON
  json.decode()     Decode from JSON
  yaml.encode()     Encode to YAML
  yaml.decode()     Decode from YAML
  strings.*         String manipulation functions
  time.*            Time functions
  path.*            Path manipulation functions

Examples:
  >>> result = local.exec("hostname")
  >>> print(result.value)
  >>> hosts = ["host1", "host2"]
  >>> for h in hosts:
  ...     print(h)
  ...
`)
}

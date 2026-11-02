package cmd

import (
	"fmt"
	"os"

	"github.com/M31-Labs/starlsp"
	"github.com/spf13/cobra"

	"github.com/project-starkite/starkite/libkite/lsp"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the language server for Starkite scripts",
	Long: `Run a Language Server Protocol server for Starkite scripts over stdin and stdout.

The server is powered by starlsp, a Starlark language server, configured with
the Starkite runtime host. Diagnostics originate from the same parser the runtime
uses, ensuring editor diagnostics and "kite run" behavior cannot disagree.
Completion, hover, and signature help are served directly from the active module
registry.

Editors launch this command automatically over stdio; it is not usually run by hand.

Examples:
  # Inspect the server's symbol catalog and surface:
  kite lsp --probe

  # Editor configuration reference:
  see docs/references/cli/lsp.md`,
	Args: cobra.NoArgs,
	RunE: runLSP,
}

var lspProbe bool

func init() {
	lspCmd.Flags().BoolVar(&lspProbe, "probe", false, "print the server's surface and exit")
	rootCmd.AddCommand(lspCmd)
}

func runLSP(cmd *cobra.Command, args []string) error {
	// SilenceUsage prevents Cobra CLI usage banners from corrupting the JSON-RPC stdio protocol stream
	cmd.SilenceUsage = true

	// The Starkite host layers over the vanilla specification host, so a script
	// receives both Starlark universe builtins and runtime standard library modules.
	host := starlsp.Hosts(starlsp.NewVanilla(), lsp.NewHost(NewRegistry))

	server, err := starlsp.New(starlsp.Options{
		Host: host,
		In:   cmd.InOrStdin(),
		Out:  cmd.OutOrStdout(),
		Log:  os.Stderr,
	})
	if err != nil {
		return err
	}

	if lspProbe {
		fmt.Fprint(cmd.OutOrStdout(), server.Probe())
		return nil
	}

	return server.Run()
}

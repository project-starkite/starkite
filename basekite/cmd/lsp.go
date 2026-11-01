//go:build lsp

// The language server is behind a build tag so the default binary is
// unchanged. Starkite ships into init containers, edge nodes, and CI runners
// under restricted permission profiles, where an editor server has no purpose
// and its parser would be dead weight.
//
//	go build                       # no language server, no size change
//	go build -tags lsp             # kite lsp available
//
// Pair it with the parser's subset tag to embed only the grammar this server
// needs:
//
//	go build -tags 'lsp,grammar_subset,grammar_subset_starlark'

package cmd

import (
	"fmt"
	"os"

	"github.com/M31-Labs/starlsp"
	"github.com/spf13/cobra"

	kitelsp "github.com/project-starkite/starkite/libkite/lsp"
)

var lspCmd = &cobra.Command{
	Use:   "lsp",
	Short: "Run the language server for Starkite scripts",
	Long: `Run a Language Server Protocol server for Starkite scripts over stdin and stdout.

The server is starlsp, a Starlark language server, configured with a Starkite
host. Diagnostics come from the same parser the runtime uses, so what an editor
shows and what "kite run" reports cannot disagree. Completion, hover, and
signature help are read from this binary's own module registry, which means they
describe the edition you are running rather than a stored list.

Editors launch this command themselves; it is not usually run by hand.

Examples:
  # Inspect what this binary would offer.
  kite lsp --probe

  # VS Code, Neovim, Helix, and Zed configuration.
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
	// The Starkite host layers over the specification host, so a script gets
	// both the Starlark builtins and the runtime's modules. On a name
	// collision the later host wins, which is the Starkite name.
	host := starlsp.Hosts(starlsp.NewVanilla(), kitelsp.NewHost(NewRegistry))

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

	// Cobra's error printing would write to stdout and corrupt the protocol
	// stream, so the serve loop reports failures itself.
	cmd.SilenceUsage = true
	return server.Run()
}

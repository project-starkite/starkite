//go:build lsp

package main

import (
	"fmt"
	"os"

	"github.com/M31-Labs/starlsp"
	"github.com/spf13/cobra"

	"github.com/project-starkite/starkite/basekite/version"
	kiteloader "github.com/project-starkite/starkite/kite/loader"
	kitelsp "github.com/project-starkite/starkite/libkite/lsp"
)

var (
	probeFlag bool
)

var rootCmd = &cobra.Command{
	Use:   "kitelsp",
	Short: "Language Server Protocol server for Starkite automation scripts",
	Long: `kitelsp runs a Language Server Protocol server for Starkite scripts over stdin and stdout.

Diagnostics come from the Starkite runtime parser, ensuring editor diagnostics and
"kite run" behavior cannot disagree. Completions, hovers, and signature help are
introspected from the complete Starkite module catalog.

Editors launch this binary directly over stdio; it is not usually run by hand.

Examples:
  # Inspect the server's symbol catalog and surface:
  kitelsp --probe

  # VS Code, Neovim, Helix, and Zed configuration:
  see docs/references/cli/lsp.md`,
	Args: cobra.NoArgs,
	RunE: runServer,
}

func init() {
	rootCmd.Flags().BoolVar(&probeFlag, "probe", false, "print the server's surface and exit")
	rootCmd.Version = version.String()
}

func runServer(cmd *cobra.Command, args []string) error {
	host := starlsp.Hosts(starlsp.NewVanilla(), kitelsp.NewHost(kiteloader.NewAllRegistry))

	server, err := starlsp.New(starlsp.Options{
		Host: host,
		In:   cmd.InOrStdin(),
		Out:  cmd.OutOrStdout(),
		Log:  os.Stderr,
	})
	if err != nil {
		return err
	}

	if probeFlag {
		fmt.Fprint(cmd.OutOrStdout(), server.Probe())
		return nil
	}

	// SilenceUsage prevents usage errors from corrupting the JSON-RPC stdio protocol stream
	cmd.SilenceUsage = true
	return server.Run()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

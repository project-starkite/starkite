package cmd

import (
	stdlog "log"

	"github.com/spf13/cobra"
	"github.com/vladimirvivien/starkite/starbase"
	"github.com/vladimirvivien/starkite/starbase/loader"
	"github.com/vladimirvivien/starkite/wasm"
)

// newDefaultRegistryWithWASM creates the default registry with built-in modules + WASM plugins.
func newDefaultRegistryWithWASM(config *starbase.ModuleConfig) *starbase.Registry {
	r := loader.NewDefaultRegistry(config)
	if err := wasm.RegisterPlugins(r, ""); err != nil {
		stdlog.Printf("wasm: plugin discovery error: %v", err)
	}
	return r
}

// NewRegistry is the function used by all script-executing commands to create
// a module registry. It defaults to the base loader which registers all built-in
// modules + WASM plugins. Edition binaries (e.g. cloud) override this before
// calling Execute() to inject additional modules.
var NewRegistry func(config *starbase.ModuleConfig) *starbase.Registry = newDefaultRegistryWithWASM

// RegisterEditionCommands is called at the start of Execute() to allow edition
// binaries to register additional cobra commands on the root command.
// Nil by default (base edition has no extra commands).
var RegisterEditionCommands func(root *cobra.Command)

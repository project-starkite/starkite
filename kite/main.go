// Binary kite-all is the all-in-one starkite edition: base + cloud + ai modules
// in one binary. The lean editions (libkite, kitecloud, kiteai) remain available
// for users who want a smaller binary or smaller attack surface.
//
// This main package deliberately imports the cloud and ai *loader* and *cmd*
// subpackages — never their main packages — so their init() side effects
// don't fight over corecmd.NewRegistry.
package main

import (
	"os"

	corecmd "github.com/project-starkite/starkite/basekite/cmd"
	"github.com/project-starkite/starkite/basekite/version"
	allcmd "github.com/project-starkite/starkite/kite/cmd"
	"github.com/project-starkite/starkite/kite/loader"
)

func init() {
	version.Edition = "all"
	corecmd.NewRegistry = loader.NewAllRegistry
	corecmd.RegisterEditionCommands = allcmd.Register
}

func main() {
	os.Exit(corecmd.Execute())
}

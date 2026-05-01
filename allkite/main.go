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

	allcmd "github.com/project-starkite/starkite/allkite/cmd"
	"github.com/project-starkite/starkite/allkite/loader"
	corecmd "github.com/project-starkite/starkite/base/cmd"
	"github.com/project-starkite/starkite/base/version"
)

func init() {
	version.Edition = "all"
	corecmd.NewRegistry = loader.NewAllRegistry
	corecmd.RegisterEditionCommands = allcmd.Register
}

func main() {
	os.Exit(corecmd.Execute())
}

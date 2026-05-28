package main

import (
	"os"

	aicmd "github.com/project-starkite/starkite/aikite/cmd"
	"github.com/project-starkite/starkite/aikite/loader"
	corecmd "github.com/project-starkite/starkite/basekite/cmd"
	"github.com/project-starkite/starkite/basekite/version"
)

func init() {
	version.Edition = "ai"
	corecmd.NewRegistry = loader.NewAIRegistry
	corecmd.RegisterEditionCommands = aicmd.Register
}

func main() {
	exitCode := corecmd.Execute()
	os.Exit(exitCode)
}

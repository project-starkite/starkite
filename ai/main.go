package main

import (
	"os"

	aicmd "github.com/project-starkite/starkite/ai/cmd"
	"github.com/project-starkite/starkite/ai/loader"
	corecmd "github.com/project-starkite/starkite/core/cmd"
	"github.com/project-starkite/starkite/core/version"
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

package main

import (
	"os"

	aicmd "github.com/vladimirvivien/starkite/ai/cmd"
	"github.com/vladimirvivien/starkite/ai/loader"
	corecmd "github.com/vladimirvivien/starkite/core/cmd"
	"github.com/vladimirvivien/starkite/core/version"
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

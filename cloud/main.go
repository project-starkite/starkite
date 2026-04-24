package main

import (
	"os"

	cloudcmd "github.com/project-starkite/starkite/cloud/cmd"
	"github.com/project-starkite/starkite/cloud/loader"
	corecmd "github.com/project-starkite/starkite/core/cmd"
	"github.com/project-starkite/starkite/core/version"
)

func init() {
	version.Edition = "cloud"
	corecmd.NewRegistry = loader.NewCloudRegistry
	corecmd.RegisterEditionCommands = cloudcmd.Register
}

func main() {
	exitCode := corecmd.Execute()
	os.Exit(exitCode)
}

package main

import (
	"os"

	cloudcmd "github.com/project-starkite/starkite/cloudkite/cmd"
	"github.com/project-starkite/starkite/cloudkite/loader"
	corecmd "github.com/project-starkite/starkite/basekite/cmd"
	"github.com/project-starkite/starkite/basekite/version"
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

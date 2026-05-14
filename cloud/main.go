package main

import (
	"os"

	corecmd "github.com/project-starkite/starkite/base/cmd"
	"github.com/project-starkite/starkite/base/version"
	cloudcmd "github.com/project-starkite/starkite/cloud/cmd"
	"github.com/project-starkite/starkite/cloud/loader"
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

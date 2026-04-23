package main

import (
	"os"

	cloudcmd "github.com/vladimirvivien/starkite/cloud/cmd"
	"github.com/vladimirvivien/starkite/cloud/loader"
	corecmd "github.com/vladimirvivien/starkite/core/cmd"
	"github.com/vladimirvivien/starkite/core/version"
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

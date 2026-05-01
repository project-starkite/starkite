package main

import (
	"os"

	"github.com/project-starkite/starkite/base/cmd"
)

func main() {
	exitCode := cmd.Execute()
	os.Exit(exitCode)
}

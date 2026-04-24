package main

import (
	"os"

	"github.com/project-starkite/starkite/core/cmd"
)

func main() {
	exitCode := cmd.Execute()
	os.Exit(exitCode)
}

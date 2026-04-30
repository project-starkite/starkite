package main

import (
	"os"

	"github.com/project-starkite/starkite/basekite/cmd"
)

func main() {
	exitCode := cmd.Execute()
	os.Exit(exitCode)
}

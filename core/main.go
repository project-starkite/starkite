package main

import (
	"os"

	"github.com/vladimirvivien/starkite/core/cmd"
)

func main() {
	exitCode := cmd.Execute()
	os.Exit(exitCode)
}

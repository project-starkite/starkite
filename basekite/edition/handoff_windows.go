//go:build windows

package edition

import (
	"fmt"
	"os"
	"os/exec"
)

// ExecHandoff runs the edition binary as a subprocess and exits with its code.
// On Windows, syscall.Exec is not available, so we use os/exec instead.
func ExecHandoff(binaryPath string) error {
	cmd := exec.Command(binaryPath, os.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("exec handoff failed: %w", err)
	}

	os.Exit(0)
	return nil // unreachable
}

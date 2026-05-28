//go:build !windows

package edition

import (
	"fmt"
	"os"
	"syscall"
)

// ExecHandoff replaces the current process with the edition binary.
// On Unix systems, this uses syscall.Exec for zero-overhead handoff.
func ExecHandoff(binaryPath string) error {
	argv := append([]string{binaryPath}, os.Args[1:]...)
	if err := syscall.Exec(binaryPath, argv, os.Environ()); err != nil {
		return fmt.Errorf("exec handoff failed: %w", err)
	}
	// unreachable if Exec succeeds
	return nil
}

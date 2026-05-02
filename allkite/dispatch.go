//go:build !linux

package main

// dispatchSandboxSubprocess is a no-op on non-Linux platforms; gVisor
// self-execs only happen on Linux.
func dispatchSandboxSubprocess() bool { return false }

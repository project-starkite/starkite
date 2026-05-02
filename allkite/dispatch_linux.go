//go:build linux

package main

import gvisorpkg "github.com/project-starkite/starkite/sandbox/gvisor"

// dispatchSandboxSubprocess routes gVisor's self-exec personalities to the
// gvisor package on Linux. Returns true when a subprocess was dispatched
// (in which case the call does not return).
func dispatchSandboxSubprocess() bool {
	return gvisorpkg.DispatchSubprocess()
}

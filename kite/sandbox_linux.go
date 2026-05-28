//go:build linux

package main

import (
	"github.com/project-starkite/starkite/libkite/sandbox"
	gvisorpkg "github.com/project-starkite/starkite/sandbox/gvisor"
)

// init registers the gVisor backend on Linux. Non-Linux builds skip this
// file entirely (no init runs), leaving sandbox.Backend == nil — the
// CLI's GetSandbox() then surfaces sandbox.PlatformError.
func init() {
	sandbox.Backend = gvisorpkg.Runner{}
}

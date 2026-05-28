//go:build !linux

// Package gvisor on non-Linux platforms exposes a Runner that always returns
// the standard platform error. This lets kite import the package
// unconditionally; the build tag ensures no gVisor code is compiled in.
package gvisor

import (
	"context"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// Runner satisfies sandbox.Runner; on non-Linux it always errors.
type Runner struct{}

// Run always returns sandbox.PlatformError on non-Linux.
func (Runner) Run(ctx context.Context, spec sandbox.ExecSpec) error {
	return sandbox.PlatformError()
}

// DispatchSubprocess is a no-op on non-Linux; gVisor self-execs only happen
// on Linux. Returns false so the normal CLI flow continues.
func DispatchSubprocess() bool {
	return false
}

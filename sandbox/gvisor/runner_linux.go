//go:build linux

// Package gvisor provides a Linux-only sandbox.Runner backed by gVisor's
// runsc components. Phase 4b.3 wires the real container.New / Start /
// Wait / Destroy flow; this file is still the 4a stub Runner.
//
// Dependency note: gvisor.dev/gvisor MUST be pinned to a commit on the
// upstream `go` branch (the synthetic Go-tool-compatible branch). The
// proxy's @latest ref resolves to `master`, which is Bazel-only and
// will not build with `go build`. See sandbox/gvisor/GVISOR_VERSION.md.
package gvisor

import (
	"context"
	"fmt"
	"os"

	"github.com/project-starkite/starkite/libkite/sandbox"

	// Anchor imports: keep gvisor.dev/gvisor's container/spec graph in
	// the require set during `go mod tidy` until 4b.3 imports them for
	// real. (runsc/cmd is now imported for real in dispatch_linux.go.)
	_ "gvisor.dev/gvisor/runsc/config"
	_ "gvisor.dev/gvisor/runsc/container"
	_ "gvisor.dev/gvisor/runsc/specutils"
)

// Runner satisfies sandbox.Runner using gVisor.
type Runner struct{}

// Run executes the script described by spec inside a gVisor sandbox.
//
// Phase 4a stub: print the intended action to stderr and return nil.
// The caller treats nil-return as "sandbox handled execution," so the
// script does NOT also run natively. This makes the stub observably
// different from "no sandbox" — useful for end-to-end plumbing tests.
//
// Phase 4b.3 will replace this with the real container.New/Start/Wait
// /Destroy flow.
func (Runner) Run(ctx context.Context, spec sandbox.ExecSpec) error {
	fmt.Fprintf(os.Stderr,
		"[sandbox stub] would run %s inside gvisor (profile=%s, network=%t)\n",
		spec.ScriptPath, spec.Profile.Name, spec.Profile.Network)
	return nil
}

//go:build linux

// Package gvisor provides a Linux-only sandbox.Runner backed by gVisor's
// runsc components. Phase 4a ships a stub that prints what it would do;
// Phase 4b wires the real container.New / Start / Wait / Destroy flow.
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

	// Anchor imports: keep gvisor.dev/gvisor and its transitive graph in
	// the require set during `go mod tidy`. These are the packages 4b.2
	// (DispatchSubprocess) and 4b.3 (Runner.Run) will actually use; pulling
	// them now gives a realistic binary-size measurement up front.
	_ "gvisor.dev/gvisor/runsc/cmd"
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
func (Runner) Run(ctx context.Context, spec sandbox.ExecSpec) error {
	fmt.Fprintf(os.Stderr,
		"[sandbox stub] would run %s inside gvisor (profile=%s, network=%t)\n",
		spec.ScriptPath, spec.Profile.Name, spec.Profile.Network)
	return nil
}

// DispatchSubprocess inspects argv for gVisor's self-exec personalities
// (boot, gofer, umount, __runtime__) and routes them to the appropriate
// handler. Returns true when the call was dispatched (caller exits
// immediately); false to fall through to normal CLI handling.
//
// Phase 4a stub: recognise the personalities and exit cleanly without
// running real gVisor code. Phase 4b replaces this with calls into
// runsc/cmd handlers.
func DispatchSubprocess() bool {
	if len(os.Args) < 2 {
		return false
	}
	switch os.Args[1] {
	case "boot", "gofer", "umount", "__runtime__":
		fmt.Fprintf(os.Stderr, "[sandbox stub] dispatched %s subprocess\n", os.Args[1])
		os.Exit(0)
	}
	return false
}

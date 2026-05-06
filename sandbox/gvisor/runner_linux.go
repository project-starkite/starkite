//go:build linux

// Package gvisor provides a Linux-only sandbox.Runner backed by gVisor's
// runsc components.
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
	"os/exec"
	"path/filepath"

	"gvisor.dev/gvisor/runsc/config"
	"gvisor.dev/gvisor/runsc/container"
	"gvisor.dev/gvisor/runsc/flag"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// Runner satisfies sandbox.Runner using gVisor.
type Runner struct{}

// Run executes the script described by spec inside a gVisor sandbox.
//
// Flow:
//  1. Build an OCI spec for the requested profile (currently only "strict").
//  2. Create a temp bundle directory containing config.json + empty rootfs.
//  3. Build a *config.Config from gVisor's defaults, then mutate for our
//     constraints (NetworkNone, RootDir, Rootless, no-debug-output).
//  4. Call container.Run(conf, args) — gVisor's synchronous one-shot
//     create+start+wait+destroy convenience.
//  5. Translate gVisor's unix.WaitStatus into a Go error: nil on exit 0,
//     ExitError on non-zero, error on signal/setup failure.
func (Runner) Run(ctx context.Context, spec sandbox.ExecSpec) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// Resolve the host kite binary so we can bind-mount it into the sandbox
	// as the contained process's exe. /proc/self/exe doesn't work as
	// Process.Args[0]: at the moment gVisor's sentry tries to load the
	// binary, /proc isn't yet mounted in the sandbox.
	hostBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving host binary: %w", err)
	}

	// What argv should the inner kite see? Strip our own argv[0] (binary
	// path) plus the --sandbox flag (which would cause infinite recursion
	// despite the env-var guard, because flags can survive env wipes
	// across exec).
	innerArgs := stripSandboxFlag(os.Args[1:])

	bun, err := allocBundle()
	if err != nil {
		return fmt.Errorf("creating bundle: %w", err)
	}
	defer bun.cleanup()

	ociSpec, err := buildSpec(spec.Profile, innerArgs, cwd, hostBinary, bun)
	if err != nil {
		return fmt.Errorf("building spec: %w", err)
	}
	if err := bun.writeSpec(ociSpec); err != nil {
		return fmt.Errorf("writing spec: %w", err)
	}

	conf, err := defaultConfig()
	if err != nil {
		return fmt.Errorf("building gvisor config: %w", err)
	}
	conf.RootDir = filepath.Join(os.TempDir(), "starkite-sandbox-state-"+bun.id)
	// NetworkHost: the contained process shares the host's network
	// namespace, giving full network reachability without the overhead
	// of gVisor's user-space netstack. Trade-off: network syscalls
	// bypass gVisor's kernel mediation. Custom profiles (Phase 5) can
	// switch this to NetworkSandbox for full kernel isolation including
	// the network path.
	conf.Network = config.NetworkHost
	conf.Rootless = true
	// Disable overlayfs for both root and submounts. The default would
	// stack a writable tmpfs on top of every mount; we don't need that
	// for one-shot script execution and it fails on rootless without
	// a writable host filestore. "none" means: bind mounts are exposed
	// directly, no overlay layer.
	if err := conf.Overlay2.Set("none"); err != nil {
		return fmt.Errorf("setting overlay2=none: %w", err)
	}
	// Verbose gVisor logs are off by default. Users can enable them via
	// kite's --debug flag once 4c wires that through (today: silent).
	if err := os.MkdirAll(conf.RootDir, 0o711); err != nil {
		return fmt.Errorf("creating runsc state dir: %w", err)
	}
	defer os.RemoveAll(conf.RootDir)

	args := container.Args{
		ID:        bun.id,
		Spec:      ociSpec,
		BundleDir: bun.dir,
		Attached:  true, // block until the contained process exits
	}

	ws, err := container.Run(conf, args)
	if err != nil {
		return fmt.Errorf("sandbox run: %w", err)
	}

	switch {
	case ws.Exited() && ws.ExitStatus() == 0:
		return nil
	case ws.Exited():
		return &exec.ExitError{ProcessState: nil, Stderr: nil}
	case ws.Signaled():
		return fmt.Errorf("sandbox process killed by signal %v", ws.Signal())
	default:
		return fmt.Errorf("sandbox process exited with unexpected status %v", ws)
	}
}

// defaultConfig instantiates a *config.Config populated with gVisor's
// default flag values. We do this by registering all config flags onto a
// fresh FlagSet and calling NewFromFlags; that's the only path gVisor
// supports for building a Config (config.Config has no zero-value-friendly
// constructor — many fields require validated default values).
func defaultConfig() (*config.Config, error) {
	fs := flag.NewFlagSet("kite-sandbox", flag.ContinueOnError)
	config.RegisterFlags(fs)
	if err := fs.Parse(nil); err != nil {
		return nil, err
	}
	return config.NewFromFlags(fs)
}

// stripSandboxFlag removes any --sandbox=... or `--sandbox X` token from
// the argv slice so the inner kite invocation doesn't try to re-sandbox.
// (The env-var guard already prevents recursion, but argv hygiene avoids
// confusing the user if they ever inspect the contained process's argv.)
func stripSandboxFlag(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--sandbox":
			i++ // skip its value
			continue
		case len(a) >= 10 && a[:10] == "--sandbox=":
			continue
		}
		out = append(out, a)
	}
	return out
}

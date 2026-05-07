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

	gvlog "gvisor.dev/gvisor/pkg/log"
	"gvisor.dev/gvisor/runsc/config"
	"gvisor.dev/gvisor/runsc/container"
	"gvisor.dev/gvisor/runsc/flag"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// init quiets gVisor's stderr noise for normal kite users. Without this,
// every sandbox run prints "Setting up network", "Host setting X is not
// optimal", etc. — informative to gVisor developers, noise to script
// authors. Users who want gVisor logs back can opt in via the standard
// pkg/log API at higher levels (Debug/Info), e.g. by calling
// gvlog.SetLevel from their own embedding code.
func init() {
	gvlog.SetLevel(gvlog.Warning)
}

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
	// Preflight: check known-blocking kernel state and emit friendly
	// errors before gVisor's container path produces cryptic failures.
	if err := preflight(); err != nil {
		return err
	}

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
	// path) plus the --sandbox flag. The InsideEnvVar marker prevents
	// re-sandboxing semantically, but stripping the flag keeps the inner
	// process's argv clean — no surprise tokens if the script ever
	// inspects its own arguments.
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
	// Profile-driven network mode. The default profile uses NetworkHost
	// (full host reachability, network syscalls bypass gVisor); strict
	// uses NetworkSandbox with no host bridging (in-sandbox loopback
	// only — packets cannot leave). The profile YAML's "network:" field
	// is the source of truth.
	netMode, err := networkModeFor(spec.Profile.Network)
	if err != nil {
		return err
	}
	conf.Network = netMode
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
// the argv slice so the inner kite invocation doesn't surface a stale
// flag when scripts inspect their own argv. The recursion guard itself
// is the InsideEnvVar marker, which is independent of argv hygiene.
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

// networkModeFor translates a profile NetworkMode into the runsc config
// equivalent.
//
//   - NetworkHost: contained process shares the host netns. Full
//     reachability; network syscalls bypass gVisor's netstack.
//   - NetworkNone: gVisor netstack runs with only the loopback interface.
//     In-sandbox loopback (127.0.0.1, ::1) works; nothing reaches the
//     host or external network because no host interfaces are mirrored
//     into the sandbox netstack and there's no bridge.
//
// We deliberately do NOT use config.NetworkSandbox for strict: that mode
// mirrors the host's network interfaces into the sentry's netstack
// (creating veth equivalents) and requires reading the host netns, which
// rootless mode cannot do across processes ("operation not permitted").
// NetworkNone gives the loopback-only behavior we want without that
// privilege requirement.
func networkModeFor(mode sandbox.NetworkMode) (config.NetworkType, error) {
	switch mode {
	case sandbox.NetworkHost:
		return config.NetworkHost, nil
	case sandbox.NetworkSandboxLoopback:
		return config.NetworkNone, nil
	default:
		return 0, fmt.Errorf("unsupported network mode %q", mode)
	}
}


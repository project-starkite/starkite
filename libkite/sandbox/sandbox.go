// Package sandbox describes the platform-agnostic interface for running
// kite scripts inside an OS-level sandbox (gVisor on Linux). Concrete
// backends register themselves via the Backend variable; on platforms
// without a registered backend, --sandbox returns a friendly error.
//
// This package intentionally has no dependency on gVisor or any other
// sandbox runtime — it stays portable so libkite embedders aren't forced
// to pull in heavyweight sandbox dependencies they don't use.
package sandbox

import (
	"context"
	"fmt"
	"runtime"
)

// ExecSpec describes a script execution that should happen inside a sandbox.
// The backend is responsible for re-executing the kite binary inside the
// sandboxed environment with the appropriate arguments.
type ExecSpec struct {
	ScriptPath string   // path to the .star file
	Args       []string // additional script arguments
	Env        []string // KEY=VALUE environment forwarded into the sandbox
	Cwd        string   // working directory for the sandboxed process
	Profile    Profile  // resolved sandbox profile
}

// Runner executes an ExecSpec inside a sandbox. When Run returns nil, the
// script has already executed inside the sandbox; the caller must not also
// run it natively.
type Runner interface {
	Run(ctx context.Context, spec ExecSpec) error
}

// Backend is the registered sandbox runner. nil means no backend is available
// on this platform/build (the typical case on macOS and Windows).
var Backend Runner

// InsideEnvVar is the environment variable that signals "this kite process
// is running inside an active sandbox." The sandbox backend sets it on the
// inner kite invocation; the CLI's MaybeHandoffToSandbox checks it to avoid
// recursive re-sandboxing. Any code that needs to know "am I inside a
// sandbox?" should consult this variable rather than hard-coding the name.
const InsideEnvVar = "STARKITE_INSIDE_SANDBOX"

// Available reports whether a backend is registered and usable.
func Available() bool {
	return Backend != nil
}

// PlatformError is the standard error returned when --sandbox is requested
// on a platform without a registered backend.
func PlatformError() error {
	return fmt.Errorf("sandbox not available on %s, use container isolation", runtime.GOOS)
}

// Profile describes the sandbox environment. Phase 4 ships one built-in
// (the "default" profile); Phase 5 adds YAML-loaded user profiles with
// custom mounts, network, etc. defined in ~/.starkite/security.yaml.
type Profile struct {
	Name    string  // empty for "no sandbox"
	Network bool    // currently informational; runner reads Name to dispatch
	Mounts  []Mount // future use (Phase 5)
}

// Mount describes a host-to-sandbox path mapping. Reserved for Phase 5.
type Mount struct {
	Source   string // path on the host
	Target   string // path inside the sandbox
	ReadOnly bool
}

// ProfileDefault is the name of the built-in default sandbox profile.
//
// The default profile relies on gVisor's intrinsic kernel-isolation
// guarantees (syscall mediation, gofer-mediated filesystem, sentry-level
// seccomp) and adds minimal additional spec hardening:
//
//   - host filesystem read-only via gofer
//   - $CWD bound writable at the same path inside the sandbox
//   - host network shared (NetworkHost — script can reach the network,
//     but network syscalls bypass gVisor's netstack for performance)
//   - pid/mount/ipc/uts/user/network namespaces isolated from host
//   - NoNewPrivileges set (cheap belt; default Linux capabilities preserved)
//   - in-process kite --permissions remains independent and unchanged
//
// Users wanting a tighter surface (no network, fs scoped to $CWD, caps
// dropped, etc.) define a custom profile in ~/.starkite/security.yaml.
// A "strict" recipe is documented in docs/guides/sandbox.md.
const ProfileDefault = "default"

// LoadProfile resolves a --sandbox value to a Profile. An empty value
// returns the zero Profile (no sandbox).
//
// Phase 4 only knows the "default" built-in; Phase 5 adds named user
// profiles loaded from ~/.starkite/security.yaml.
func LoadProfile(value string) (Profile, error) {
	if value == "" {
		return Profile{}, nil
	}
	if value == ProfileDefault {
		return defaultProfile(), nil
	}
	return Profile{}, fmt.Errorf(
		"unknown sandbox profile %q (built-in: %s; user-defined profiles "+
			"in ~/.starkite/security.yaml are not yet supported)",
		value, ProfileDefault)
}

// defaultProfile populates the Profile struct with the default settings.
// The runner consults Name to dispatch; richer fields land in Phase 5
// when user-defined profiles need to drive runtime behavior.
func defaultProfile() Profile {
	return Profile{
		Name:    ProfileDefault,
		Network: true,
		Mounts:  nil,
	}
}

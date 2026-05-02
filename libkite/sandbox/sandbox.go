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

// Available reports whether a backend is registered and usable.
func Available() bool {
	return Backend != nil
}

// PlatformError is the standard error returned when --sandbox is requested
// on a platform without a registered backend.
func PlatformError() error {
	return fmt.Errorf("sandbox not available on %s, use container isolation", runtime.GOOS)
}

// Profile describes the sandbox environment. Phase 4 ships only the "strict"
// built-in; Phase 5 adds YAML-loaded profiles with mounts, network, etc.
type Profile struct {
	Name    string  // empty for "no sandbox"
	Network bool    // false = network namespace isolated
	Mounts  []Mount // host paths exposed inside the sandbox
}

// Mount describes a host-to-sandbox path mapping.
type Mount struct {
	Source   string // path on the host
	Target   string // path inside the sandbox
	ReadOnly bool
}

// LoadProfile resolves a --sandbox value to a Profile. An empty value
// returns the zero Profile (no sandbox). Phase 4 only knows the "strict"
// built-in; Phase 5 adds named user profiles loaded from
// ~/.starkite/security.yaml.
func LoadProfile(value string) (Profile, error) {
	if value == "" {
		return Profile{}, nil
	}
	if value != "strict" {
		return Profile{}, fmt.Errorf("unknown sandbox profile %q (built-ins: strict)", value)
	}
	return strictProfile(), nil
}

// strictProfile is the minimal-environment sandbox: no network namespace,
// stdin/stdout/stderr only, no extra mounts. Phase 4b will add the
// $CWD-read-only and /etc/ssl/certs mounts that scripts typically need.
func strictProfile() Profile {
	return Profile{
		Name:    "strict",
		Network: false,
		Mounts:  nil,
	}
}

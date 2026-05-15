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

// EngagementEnvVar is the user-facing environment variable that selects
// the sandbox profile to engage. Setting it to a non-empty value causes
// the next kite invocation (whether triggered explicitly via `kite run
// script.star` or implicitly via shebang `./script.star`) to execute the
// script inside the named sandbox. Unset/empty means "no sandbox".
//
// The value uses the same syntax as LoadProfile: a built-in name
// ("default", "strict"), a file path, or a named profile under
// "sandbox:<name>" in ~/.starkite/security.yaml.
//
// Examples:
//
//	STARKITE_SECURITY_SANDBOX=strict kite analyze.star
//	export STARKITE_SECURITY_SANDBOX=default
//	./untrusted.star    # shebang: same env var, same effect
const EngagementEnvVar = "STARKITE_SECURITY_SANDBOX"

// Available reports whether a backend is registered and usable.
func Available() bool {
	return Backend != nil
}

// PlatformError is the standard error returned when --sandbox is requested
// on a platform without a registered backend.
func PlatformError() error {
	return fmt.Errorf("sandbox not available on %s, use container isolation", runtime.GOOS)
}

// NetworkMode is the sandbox network configuration as understood by the
// platform-agnostic profile schema. The runtime backend translates each
// value to its native network setting (e.g. gVisor's config.Network).
type NetworkMode string

const (
	// NetworkHost shares the host's network namespace with the contained
	// process. Network reachability matches the host. Used by the default
	// profile.
	NetworkHost NetworkMode = "host"

	// NetworkSandboxLoopback enables the runtime's network stack but does
	// not bridge it to the host. In-sandbox loopback works (servers and
	// clients within one script can talk); no packet leaves the sandbox.
	// Used by the strict profile.
	NetworkSandboxLoopback NetworkMode = "sandbox-loopback"
)

// MountType discriminates between supported sandbox mount kinds.
type MountType string

const (
	// MountBind mirrors a host path to a path inside the sandbox.
	MountBind MountType = "bind"

	// MountTmpfs creates an empty in-memory filesystem at the destination.
	// Source is ignored.
	MountTmpfs MountType = "tmpfs"
)

// MountMode is "rw" or "ro" — the writability of the mount inside the
// sandbox. The host file's permissions are unaffected by either choice.
type MountMode string

const (
	MountRW MountMode = "rw"
	MountRO MountMode = "ro"
)

// Mount describes a single host-to-sandbox path mapping in the profile.
//
// Source is required for Type=bind, ignored for Type=tmpfs. A bind whose
// source is missing on the host will fail at sandbox start unless
// Optional is true (which causes the runner to silently skip it; useful
// for profile entries like /etc/resolv.conf that may be absent on
// minimal hosts).
type Mount struct {
	Source      string    // host path (empty for tmpfs)
	Destination string    // path inside the sandbox
	Type        MountType // bind | tmpfs
	Mode        MountMode // rw | ro
	Optional    bool      // bind: skip silently when source is absent
}

// Profile is the resolved sandbox configuration: the platform-agnostic
// description that the runtime backend translates to a runsc/OCI spec.
//
// Profile is loaded by LoadProfile from one of the built-in YAML files
// embedded into the binary, from a file path on disk, or from
// ~/.starkite/security.yaml. Both built-ins live as .yaml files in
// libkite/sandbox/profiles/ and are embedded via go:embed.
type Profile struct {
	Name    string      // empty for "no sandbox"
	Network NetworkMode // host | sandbox-loopback
	Mounts  []Mount
}

// IsZero reports whether the profile is the empty value, i.e. the caller
// did not request a sandbox. Backends use this to short-circuit.
func (p Profile) IsZero() bool {
	return p.Name == ""
}

// Built-in profile names. Must match the basenames of the embedded YAML
// files under libkite/sandbox/profiles/.
const (
	ProfileDefault = "default"
	ProfileStrict  = "strict"
)

// LoadProfile resolves a --sandbox value to a Profile. An empty value
// returns the zero Profile (no sandbox).
//
// Resolution order:
//
//  1. Built-in name: "default", "strict".
//  2. File path: contains '/' or '\', or ends in .yaml/.yml. Optional
//     "#name" fragment selects a profile when the file holds more than
//     one (under a top-level "sandbox:" map).
//  3. Named user profile: looked up under "sandbox.<name>" in
//     ~/.starkite/security.yaml.
func LoadProfile(value string) (Profile, error) {
	if value == "" {
		return Profile{}, nil
	}
	switch value {
	case ProfileDefault, ProfileStrict:
		return loadBuiltin(value)
	}
	if isFilePath(value) {
		return loadFromFile(value)
	}
	return loadNamed(value)
}

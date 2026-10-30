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
	"time"
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

// ProfileEnvVar is the environment variable that selects the sandbox profile to engage.
// Setting it to a non-empty value causes the next kite invocation to execute the script
// inside the named sandbox profile. Unset/empty means "no sandbox".
//
// The value matches the syntax accepted by --sandbox-profile: a built-in name
// ("default", "deny-all", "sandbox-path", "sandbox-exec", "sandbox-net", "unconfined"),
// or a named profile under "sandbox:<name>" in config.yaml.
const ProfileEnvVar = "STARKITE_SANDBOX_PROFILE"

// EngagementEnvVar is an alias for ProfileEnvVar.
const EngagementEnvVar = ProfileEnvVar

// DriverEnvVar is the environment variable that overrides the sandbox execution driver engine.
const DriverEnvVar = "STARKITE_SANDBOX_DRIVER"

// Available reports whether a sandbox driver or backend is registered and usable.
func Available() bool {
	if Backend != nil {
		return true
	}
	_, err := AutoDetect()
	return err == nil
}

// PlatformError is the standard error returned when a sandbox is requested
// on a platform without a usable registered driver.
func PlatformError() error {
	return fmt.Errorf("no usable sandbox driver available on %s", runtime.GOOS)
}

// NetworkMode is the sandbox network configuration as understood by the
// platform-agnostic profile schema. The runtime backend translates each
// value to its native network setting (e.g. gVisor's config.Network).
type NetworkMode string

const (
	// NetworkNone disables all network capabilities.
	NetworkNone NetworkMode = "none"

	// NetworkHost shares the host's network namespace with the contained
	// process. Network reachability matches the host. Used by the
	// net-access and host rungs.
	NetworkHost NetworkMode = "host"

	// NetworkLoopback enables the runtime's network stack but does not
	// bridge it to the host. In-sandbox loopback works (servers and
	// clients within one script can talk); no packet leaves the sandbox.
	// Used by the opaque rung.
	NetworkLoopback NetworkMode = "loopback"
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
// Source is required for Type=bind, ignored for Type=tmpfs. A bind source
// must exist on the host: a user-profile mount with an absent source is a
// load-time error, and built-in profiles drop absent sources at load (so
// the rungs stay portable to minimal hosts).
type Mount struct {
	Source      string    // host path (empty for tmpfs)
	Destination string    // path inside the sandbox
	Type        MountType // bind | tmpfs
	Mode        MountMode // rw | ro
}

// PathsSpec declares readable and writable host paths for a sandbox profile.
type PathsSpec struct {
	Read  []string `yaml:"read,omitempty"`
	Write []string `yaml:"write,omitempty"`
}

// ExecPolicySpec declares allowed and denied binaries for process execution.
type ExecPolicySpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// CallsPolicySpec declares allowed and denied Starlark module.category(resource) calls.
type CallsPolicySpec struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// EnvPolicySpec declares environment variable inheritance and explicit/passthrough vars.
type EnvPolicySpec struct {
	Inherit bool               `yaml:"inherit,omitempty"`
	Vars    map[string]*string `yaml:"-"`
}

// Profile is the resolved sandbox configuration: the platform-agnostic
// description that the runtime backend translates to a driver execution spec.
type Profile struct {
	Name             string          `yaml:"name,omitempty" json:"name,omitempty"`                             // empty for "no sandbox"
	Enabled          bool            `yaml:"enabled,omitempty" json:"enabled,omitempty"`                       // true when OS sandbox confinement is active; false for unconfined
	Driver           string          `yaml:"driver,omitempty" json:"driver,omitempty"`                         // driver name: "default", "podman", "docker", "landlock", "seatbelt", etc.
	Image            string          `yaml:"image,omitempty" json:"image,omitempty"`                           // container image (for container/podman/docker drivers)
	Runtime          RuntimeType     `yaml:"runtime,omitempty" json:"runtime,omitempty"`                       // language runtime environment: "python", "node", "ruby"
	Network          NetworkMode     `yaml:"network,omitempty" json:"network,omitempty"`                       // none | host | loopback
	NetworkAllow     []string        `yaml:"network_allow,omitempty" json:"network_allow,omitempty"`           // allowed network endpoints
	NetworkDeny      []string        `yaml:"network_deny,omitempty" json:"network_deny,omitempty"`             // denied network endpoints
	Mounts           []Mount         `yaml:"mounts,omitempty" json:"mounts,omitempty"`                         // filesystem mount list
	Paths            PathsSpec       `yaml:"paths,omitempty" json:"paths"`                                     // readable and writable host paths
	Exec             ExecPolicySpec  `yaml:"exec,omitempty" json:"exec"`                                       // process execution policy
	Calls            CallsPolicySpec `yaml:"calls,omitempty" json:"calls"`                                     // Starlark API call policy
	Env              EnvPolicySpec   `yaml:"env,omitempty" json:"env"`                                         // environment policy
	MaxMemoryMB      int64           `yaml:"max_memory_mb,omitempty" json:"max_memory_mb,omitempty"`           // memory limit in megabytes (0 = unconstrained)
	Timeout          time.Duration   `yaml:"timeout,omitempty" json:"timeout,omitempty"`                       // execution timeout (0 = unconstrained)
	NoExec           bool            `yaml:"no_exec,omitempty" json:"no_exec,omitempty"`                       // disable subprocess execution inside the sandbox
	AllowedExecPaths []string        `yaml:"allowed_exec_paths,omitempty" json:"allowed_exec_paths,omitempty"` // explicit allowlist of executable paths
}

// IsZero reports whether the profile is the empty value, i.e. the caller
// did not request a sandbox. Backends use this to short-circuit.
func (p Profile) IsZero() bool {
	return p.Name == "" && !p.Enabled && p.Driver == "" && p.Image == "" &&
		p.Runtime == "" &&
		p.Network == "" && len(p.NetworkAllow) == 0 && len(p.NetworkDeny) == 0 &&
		len(p.Mounts) == 0 && len(p.Paths.Read) == 0 && len(p.Paths.Write) == 0 &&
		len(p.Exec.Allow) == 0 && len(p.Exec.Deny) == 0 &&
		len(p.Calls.Allow) == 0 && len(p.Calls.Deny) == 0 &&
		!p.Env.Inherit && len(p.Env.Vars) == 0 &&
		p.MaxMemoryMB == 0 && p.Timeout == 0 &&
		!p.NoExec && len(p.AllowedExecPaths) == 0
}

// Built-in profile names — the capability ladder, each rung a superset of
// the prior.
const (
	ProfileDenyAll     = "deny-all"
	ProfileSandboxPath = "sandbox-path"
	ProfileSandboxExec = "sandbox-exec"
	ProfileSandboxNet  = "sandbox-net"
	ProfileUnconfined  = "unconfined"
	ProfilePythonEnv   = "python-env"
	ProfileNodeEnv     = "node-env"
	ProfileRubyEnv     = "ruby-env"
)

// DefaultProfileName is the reserved profile name in config.yaml's sandbox:
// map consumed by a bare --sandbox (or --sandbox=default).
const DefaultProfileName = "default"

// LoadProfile resolves a --sandbox value to a Profile. An empty value
// returns the zero Profile (no sandbox).
func LoadProfile(value string) (Profile, error) {
	if value == "" {
		return Profile{}, nil
	}
	return loadProfileResolved(value)
}

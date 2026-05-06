//go:build linux

package gvisor

import (
	"os"

	specs "github.com/opencontainers/runtime-spec/specs-go"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// buildSpec produces an OCI runtime spec gVisor will accept.
//
// Inspired by `runsc do` (gVisor's own canonical "simplest spec that works"
// example): use the host's `/` as the rootfs and let gVisor's gofer expose
// it read-only. This avoids the "input/output error: creating root file
// system" failure mode that comes from a bundle-private empty rootfs plus
// a stack of submounts — the gofer struggles when the rootfs lacks the
// mountpoint directories. With Root.Path=/ the host's filesystem layout
// is naturally available; we don't need to bind-mount /bin, /usr, /lib,
// or the kite binary.
//
// Strict-profile isolation comes from:
//   - PID/mount/IPC/UTS/user/network namespaces
//   - All capabilities dropped, NoNewPrivileges
//   - conf.Network = NetworkNone (set by caller)
//   - The in-process --permissions rule engine (continues to enforce
//     module-level rules inside the sandbox)
//
// `innerArgs` is the argv the inner kite should see AFTER the
// `__runtime__` marker. `cwd` is the host CWD. `hostBinary` is the
// absolute path of the kite executable on the host (from os.Executable()).
// We pass the host binary path directly as Process.Args[0] — no need
// for /proc/self/exe (which is unresolvable at sandbox start) or a bind
// mount, since the host filesystem is already the sandbox rootfs.
func buildSpec(profile sandbox.Profile, innerArgs []string, cwd, hostBinary string) *specs.Spec {
	procArgs := append([]string{hostBinary, "__runtime__"}, innerArgs...)
	env := append(os.Environ(), sandbox.InsideEnvVar+"=1")

	return &specs.Spec{
		Version: "1.0.0",
		Process: &specs.Process{
			Terminal:        false,
			Args:            procArgs,
			Env:             env,
			Cwd:             cwd,
			Capabilities:    &specs.LinuxCapabilities{},
			NoNewPrivileges: true,
		},
		Root: &specs.Root{
			Path: "/", // host root, exposed read-only by gofer
		},
		Linux: &specs.Linux{
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.PIDNamespace},
				{Type: specs.MountNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.UserNamespace},
				{Type: specs.NetworkNamespace},
			},
			UIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getuid()), ContainerID: 0, Size: 1},
			},
			GIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getgid()), ContainerID: 0, Size: 1},
			},
		},
	}
}

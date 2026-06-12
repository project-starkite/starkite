//go:build linux

package gvisor

import (
	"fmt"
	"os"

	specs "github.com/opencontainers/runtime-spec/specs-go"
	"gvisor.dev/gvisor/runsc/specutils"

	"github.com/project-starkite/starkite/libkite/sandbox"
)

// sandboxKitePath is the path inside the sandbox where the kite binary
// is bind-mounted (read-only). The contained process is launched via
// this path. Top-level dotfile keeps it out of /work and away from any
// host path the user might expect to see.
const sandboxKitePath = "/.kite"

// buildSpec produces an OCI runtime spec from a resolved sandbox.Profile.
//
// The profile's `mounts:` list is the single source of truth for what's
// visible inside the sandbox. Everything else the spec needs (the kite
// binary entrypoint, /proc, /dev) is added by the runner as plumbing —
// the user's profile YAML never has to mention these.
//
// Profile semantics realized here:
//   - Root.Path = bundle's empty rootfs (NOT host /)
//   - Process.Args[0] = sandboxKitePath ("/.kite"), bind-mounted from
//     hostBinary
//   - Process.Capabilities = AllCapabilities (gVisor default)
//   - NoNewPrivileges = true
//   - pid/mount/ipc/uts/user namespaces isolated
//   - NetworkNamespace deliberately omitted; runner sets conf.Network
//     based on profile.Network (host vs loopback)
//   - Single-UID identity mapping
func buildSpec(profile sandbox.Profile, innerArgs []string, cwd, hostBinary string, bun *bundle) (*specs.Spec, error) {
	procArgs := append([]string{sandboxKitePath, "__runtime__"}, innerArgs...)
	env := append(os.Environ(), sandbox.InsideEnvVar+"=1")

	// Mountpoint preparation: every destination inside the rootfs needs
	// to exist before the gofer mounts onto it. Files for file-bind
	// mounts; dirs for everything else.
	mountpoints := []struct {
		path   string
		isFile bool
	}{
		{sandboxKitePath, true}, // kite binary entrypoint (file bind)
		{"/proc", false},        // procfs (plumbing)
		{"/dev", false},         // devtmpfs (plumbing)
	}
	for _, m := range profile.Mounts {
		mountpoints = append(mountpoints, struct {
			path   string
			isFile bool
		}{m.Destination, m.Type == sandbox.MountBind && isFileMount(m.Source)})
	}
	for _, mp := range mountpoints {
		if err := bun.prepMountpoint(mp.path, mp.isFile); err != nil {
			return nil, err
		}
	}

	// Plumbing mounts: always present regardless of profile.
	mounts := []specs.Mount{
		{
			Destination: sandboxKitePath,
			Type:        "bind",
			Source:      hostBinary,
			Options:     []string{"bind", "ro", "nosuid"},
		},
		{Destination: "/proc", Type: "proc", Source: "proc"},
		{Destination: "/dev", Type: "tmpfs", Source: "tmpfs"},
	}

	// Profile-driven mounts.
	for _, m := range profile.Mounts {
		ociMount, err := mountToOCI(m)
		if err != nil {
			return nil, err
		}
		mounts = append(mounts, ociMount)
	}

	// NetworkNamespace deliberately omitted for both profiles. The
	// runner sets conf.Network instead:
	//   - NetworkHost: sandbox shares host netns (full reachability).
	//   - NetworkSandbox: gVisor implements an internal netstack inside
	//     the sentry; the contained process sees only that netstack
	//     regardless of which host netns the gofer runs in. Adding a
	//     NetworkNamespace spec entry causes runsc rootless to attempt
	//     a netns join that fails ("operation not permitted").
	namespaces := []specs.LinuxNamespace{
		{Type: specs.PIDNamespace},
		{Type: specs.MountNamespace},
		{Type: specs.IPCNamespace},
		{Type: specs.UTSNamespace},
		{Type: specs.UserNamespace},
	}

	return &specs.Spec{
		Version: "1.0.0",
		Process: &specs.Process{
			Terminal:        false,
			Args:            procArgs,
			Env:             env,
			Cwd:             cwd,
			Capabilities:    specutils.AllCapabilities(),
			NoNewPrivileges: true,
		},
		Root: &specs.Root{
			Path: bun.rootfsPath,
		},
		Mounts: mounts,
		Linux: &specs.Linux{
			Namespaces: namespaces,
			UIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getuid()), ContainerID: 0, Size: 1},
			},
			GIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getgid()), ContainerID: 0, Size: 1},
			},
		},
	}, nil
}

// mountToOCI translates a sandbox.Mount into the OCI runtime-spec form
// the gofer consumes. Absent bind sources were already handled at profile
// load (built-ins filtered, user profiles rejected).
func mountToOCI(m sandbox.Mount) (specs.Mount, error) {
	switch m.Type {
	case sandbox.MountBind:
		opts := []string{"rbind", "nosuid", "nodev"}
		switch m.Mode {
		case sandbox.MountRO:
			opts = append(opts, "ro")
		case sandbox.MountRW:
			opts = append(opts, "rw")
		default:
			return specs.Mount{}, fmt.Errorf("mount %s: unknown mode %q", m.Destination, m.Mode)
		}
		return specs.Mount{
			Destination: m.Destination,
			Type:        "bind",
			Source:      m.Source,
			Options:     opts,
		}, nil
	case sandbox.MountTmpfs:
		return specs.Mount{
			Destination: m.Destination,
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "nodev"},
		}, nil
	default:
		return specs.Mount{}, fmt.Errorf("mount %s: unknown type %q", m.Destination, m.Type)
	}
}

// isFileMount reports whether a host path is a regular file (vs directory),
// which determines whether gVisor needs a placeholder file or directory at
// the sandbox-side mountpoint. For a path that doesn't exist on the host,
// default to false (directory) — the host-side stat would fail later anyway.
func isFileMount(path string) bool {
	st, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !st.IsDir()
}

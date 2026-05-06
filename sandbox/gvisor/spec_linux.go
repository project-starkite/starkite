//go:build linux

package gvisor

import (
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

// publicSystemFiles is the small curated set of host files mounted
// read-only into the default-profile sandbox to make network protocols
// (HTTP/HTTPS DNS+TLS resolution, SSH socket reachability) work out of
// the box without exposing user credentials or any portion of $HOME.
//
// What's intentionally OUT of this list:
//   - /etc/ssh/ssh_config — system-wide SSH client config can include
//     IdentityFile paths like `~/.ssh/foo`. Even though the referenced
//     files are NOT readable inside the sandbox (~/.ssh isn't mounted),
//     the config itself reveals host directory structure to a script,
//     which is information disclosure we'd rather not give.
//   - ~/.ssh/* (private keys, known_hosts, user config) — credentials.
//     Scripts needing SSH key auth must move/copy the key into $CWD.
//   - /etc/ssh/ssh_known_hosts — system-wide TOFU state.
//   - Any host file under $HOME or elsewhere — credentials risk.
//
// Custom profiles (Phase 5) defined in ~/.starkite/security.yaml can
// override any of this.
var publicSystemFiles = []string{
	"/etc/ssl/certs",     // TLS root certificates (HTTPS verify)
	"/etc/resolv.conf",   // DNS resolver config
	"/etc/hosts",         // hostname overrides
	"/etc/nsswitch.conf", // name service switch (DNS, etc.)
}

// buildSpec produces an OCI runtime spec for the sandbox default profile.
//
// Default profile semantics (locked 2026-05-06, tightened 2026-05-06):
//   - Root.Path = bundle's empty rootfs (NOT host /)
//   - $CWD bound writable at the same host path inside the sandbox
//   - kite binary bind-mounted read-only at /.kite (Process.Args[0])
//   - tmpfs at /tmp (writable, private, lost on exit)
//   - /proc and /dev as the pseudo-filesystems gVisor needs
//   - Curated set of public system files (TLS certs, DNS config) bound
//     read-only — no credentials or user data
//   - **No other host filesystem visible**
//   - Process.Capabilities = AllCapabilities (gVisor default)
//   - NoNewPrivileges = true
//   - pid/mount/ipc/uts/user namespaces isolated
//   - NetworkNamespace deliberately omitted (NetworkHost is in effect)
//   - Single-UID identity mapping
//
// The ~/.aws/credentials, ~/.ssh/, ~/.kube/, ~/.config/, $HOME/-anything
// paths are NOT visible inside the sandbox under this profile. Scripts
// needing broader fs access define a custom profile in
// ~/.starkite/security.yaml (Phase 5).
//
// `bun` provides the rootfs path AND mountpoint preparation hooks.
// `cwd` is the host CWD path that becomes the sandbox CWD too.
// `hostBinary` is the absolute kite executable path on the host.
func buildSpec(profile sandbox.Profile, innerArgs []string, cwd, hostBinary string, bun *bundle) (*specs.Spec, error) {
	procArgs := append([]string{sandboxKitePath, "__runtime__"}, innerArgs...)
	env := append(os.Environ(), sandbox.InsideEnvVar+"=1")

	// Pre-create mountpoints inside rootfs so the gofer can satisfy
	// each mount request. Files for file binds; dirs for dir binds /
	// pseudo-fs. Order doesn't matter; failures abort the whole spec.
	mountpoints := []struct {
		path   string
		isFile bool
	}{
		{sandboxKitePath, true},  // kite binary (file bind)
		{cwd, false},             // user's working dir (dir bind)
		{"/tmp", false},          // tmpfs
		{"/proc", false},         // procfs
		{"/dev", false},          // devtmpfs
	}
	for _, p := range publicSystemFiles {
		mountpoints = append(mountpoints, struct {
			path   string
			isFile bool
		}{p, isFileMount(p)})
	}
	for _, mp := range mountpoints {
		if err := bun.prepMountpoint(mp.path, mp.isFile); err != nil {
			return nil, err
		}
	}

	mounts := []specs.Mount{
		{
			Destination: sandboxKitePath,
			Type:        "bind",
			Source:      hostBinary,
			Options:     []string{"bind", "ro", "nosuid"},
		},
		{
			// User's working dir — the only writable host path.
			Destination: cwd,
			Type:        "bind",
			Source:      cwd,
			Options:     []string{"rbind", "rw", "nosuid", "nodev"},
		},
		{
			Destination: "/tmp",
			Type:        "tmpfs",
			Source:      "tmpfs",
			Options:     []string{"nosuid", "nodev"},
		},
		{Destination: "/proc", Type: "proc", Source: "proc"},
		{Destination: "/dev", Type: "tmpfs", Source: "tmpfs"},
	}
	for _, p := range publicSystemFiles {
		// Skip host paths that don't exist (e.g. some minimal containers).
		if _, err := os.Stat(p); err != nil {
			continue
		}
		mounts = append(mounts, specs.Mount{
			Destination: p,
			Type:        "bind",
			Source:      p,
			Options:     []string{"rbind", "ro", "nosuid", "nodev"},
		})
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
			Namespaces: []specs.LinuxNamespace{
				{Type: specs.PIDNamespace},
				{Type: specs.MountNamespace},
				{Type: specs.IPCNamespace},
				{Type: specs.UTSNamespace},
				{Type: specs.UserNamespace},
				// NetworkNamespace deliberately omitted: the default
				// profile uses runsc's NetworkHost mode. Custom profiles
				// pairing NetworkNone with isolation should add this back.
			},
			UIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getuid()), ContainerID: 0, Size: 1},
			},
			GIDMappings: []specs.LinuxIDMapping{
				{HostID: uint32(os.Getgid()), ContainerID: 0, Size: 1},
			},
		},
	}, nil
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

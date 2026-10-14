---
title: "Sandbox"
description: "Pluggable OS and container runtime isolation"
weight: 30
---

# Execution Sandbox

Starkite provides a pluggable OS-level sandbox architecture that isolates script execution across native operating system primitives and container runtimes. Confined scripts are restricted from accessing host credentials, unauthorized directories, and unapproved network surfaces.

## Supported Sandbox Drivers

| Driver | Platform | Mechanism | Overhead |
|---|---|---|---|
| `landlock` | Linux | Pure-Go unprivileged Landlock kernel syscalls (`landlock_create_ruleset`) | In-process (0 subprocess overhead) |
| `seatbelt` | macOS | Pure-Go dynamic dynamic binding (`sandbox_init` / SBPL profiles) | In-process (0 subprocess overhead) |
| `podman` | Linux, macOS | Ephemeral rootless OCI container with auto-bind mounts | Low subprocess overhead |
| `docker` | Linux, macOS | Ephemeral Docker container execution | Low subprocess overhead |
| `nerdctl` | Linux, macOS | Ephemeral containerd runtime execution | Low subprocess overhead |
| `gvisor` | Linux | External `runsc` application kernel (Sentry) | Container / MicroVM overhead |

When invoked without an explicit driver name, Starkite auto-detects the host driver: `landlock` on Linux, `seatbelt` on macOS, or a configured container runtime if native drivers are unavailable.

## Quick start

To execute a script within the default sandbox profile:

```bash
kite run ./script.star --sandboxed
```

To target a specific profile, pass `--sandbox-profile` (with space or `=`):

```bash
kite run ./script.star --sandbox-profile opaque
kite run ./script.star --sandbox-profile=net-access
kite run ./script.star --sandbox-profile host
```

Or use built-in boolean shortcut flags:

```bash
kite run ./script.star --sandbox-opaque
kite run ./script.star --sandbox-net
kite run ./script.star --sandbox-host
```

To override the sandbox execution driver, pass `--sandbox-driver`:

```bash
kite run ./script.star --sandbox-opaque --sandbox-driver=landlock
kite run ./script.star --sandbox-host --sandbox-driver=seatbelt
kite run ./script.star --sandbox-net --sandbox-driver=podman
kite run ./script.star --sandbox-opaque --sandbox-driver=gvisor
kite run ./script.star --sandbox-driver=podman   # Runs default profile in Podman
```

For shebang scripts (`#!/usr/bin/env kite`), configure the sandbox via environment variables:

```bash
# Set profile and/or driver via environment variables
STARKITE_SANDBOX_PROFILE=opaque STARKITE_SANDBOX_DRIVER=seatbelt ./script.star
```

## Built-in sandbox profiles

Starkite provides three standard sandbox profiles and corresponding CLI shortcut flags:

| Profile | Shortcut Flag | Network | Filesystem Access | Purpose |
|---|---|---|---|---|
| `opaque` | `--sandbox-opaque` | None / Loopback | `$CWD` read/write, `/tmp` tmpfs | Completely offline execution; writes restricted to the working directory. |
| `net-access` | `--sandbox-net`, `--sandbox-net-access` | Full network | `opaque` + read-only CA certificates and `/etc/resolv.conf` | Outbound network allowed (e.g. HTTP clients or Git operations). |
| `host` | `--sandbox-host` | Full network | `net-access` + read-only `$HOME`, `/usr`, `/bin`, `/lib` | Allows reading host files and running system binaries while preventing modifications. |

## Custom sandbox profiles

Define custom sandbox profiles in `~/.starkite/config.yaml`. The `sandbox:` section is a pure profile mapping, where `default` is the profile selected when `--sandboxed` is passed without a specific profile:

```yaml
# ~/.starkite/config.yaml
sandbox:
  # The default profile applied when `--sandboxed` is used alone
  default:
    base: net-access
    driver: podman             # Optional: binds a default driver to this profile

  # Strictly confined script profile with no subprocess execution
  strict:
    base: opaque
    no_exec: true              # Blocks all subprocess execution (execve)

  # Custom named profile with targeted binary execution allowlist
  dev:
    base: host                 # Inherits host settings
    allowed_exec_paths:        # Restricts execution to specific binaries
      - /bin/echo
      - /usr/bin/git
    mounts:
      - source: $HOME/.cache
        destination: $HOME/.cache
        mode: rw

  ci-builder:
    driver: docker             # Profile-bound driver
    base: net-access
    mounts:
      - source: $CWD
        destination: /workspace
        mode: rw
```

Execute with custom profiles:

```bash
kite run ./build.star --sandbox-profile ci-builder --sandbox-driver podman  # CLI driver overrides profile default
```

## Security Invariants & Confinement

Starkite sandbox drivers enforce four core security invariants across native OS and container runtimes.

### Write-XOR-Execute (W^X)

To prevent code injection and dynamic binary dropping, all writable locations—including `$CWD`, `/tmp`, and any bind mount with `mode: rw`—are mounted with `noexec` access rights. Scripts cannot generate a binary or script file inside a writable scratch directory and then execute it directly as a subprocess. Binaries must reside on immutable, read-only system paths or explicitly mounted read-only directories.

### Subprocess Execution Lockdown

You can constrain or completely eliminate subprocess execution within a sandbox:

- **`no_exec` (boolean)**: Completely disables process execution. Any attempt to invoke a subprocess (`SYS_EXECVE`/`SYS_EXECVEAT` in Linux Seccomp-BPF or `process-exec` in macOS Seatbelt) immediately fails with an execution policy denial.
- **`allowed_exec_paths` (list of strings)**: Restricts executable paths to an allowlist of absolute binary paths. Spawning any binary outside this list is denied.

### Shared Memory & IPC Isolation

Host shared memory segments (`/dev/shm` and `/run/shm`) and System V IPC primitives (`shmget`, `semget`, `msgget`) present cross-process attack surfaces. Starkite isolates shared memory across all drivers:

- **Landlock & Seccomp-BPF**: System V IPC syscalls are denied by the in-process Seccomp-BPF filter, and `/dev/shm` paths are excluded from Landlock rulesets.
- **Seatbelt**: macOS SBPL profiles deny POSIX and System V shared memory descriptors.
- **Containers**: Container runtimes are launched with `--ipc=private`, preventing access to the host's IPC namespace.

### Process Inspection & Signal Boundary

On Linux, the Seccomp-BPF filter is installed unconditionally across all profiles, including the `host` profile and `network: host`. The filter prevents sandboxed processes from inspecting or tampering with host processes (`SYS_PTRACE`, `SYS_PROCESS_VM_READV`, `SYS_PROCESS_VM_WRITEV`) and forbids sending signals (`SYS_KILL`) to processes outside the sandbox process tree.

## Container Driver Execution & Image Packaging

When using container sandbox drivers (`podman`, `docker`, `nerdctl`), Starkite executes scripts within an OCI container:

- **Default Image**: Container drivers default to `ghcr.io/project-starkite/starkite:latest`. Custom images can be specified using `image:` in profile configuration or `--sandbox-image`.
- **Image Composition**: The official `ghcr.io/project-starkite/starkite:latest` container is a minimal distroless image containing the `kite` engine as its entrypoint. For executing general OS utilities or custom toolchains inside a container sandbox, specify an image containing the required binaries (such as `alpine:latest` or custom developer images).
- **Cross-Platform Host Support**: On non-Linux hosts (such as macOS), container drivers execute the Linux `kite` binary packaged within the OCI container, seamlessly mounting the local workspace into `/workspace` without attempting to mount the host's native (Mach-O) binary.
- **Local Testing & Development**: When developing or testing container isolation locally without pulling the remote registry image, build the local container image using the Makefile:

```bash
# Build local container image with Podman
make podman-build

# Or with Docker
make docker-build
```

## Starlark `sandbox` Module API

In addition to CLI-level process isolation, Starkite provides a built-in `sandbox` Starlark module for programmatic sandbox creation and child script execution.

### Creating and Executing in a Sandbox

```python
# Query host driver
driver = sandbox.default_driver()

# Create a configured sandbox instance with execution gating
box = sandbox.config(
    driver="auto",
    network="host",
    no_exec=False,
    allowed_exec_paths=["/bin/echo", "/usr/bin/git"],
    mounts=[
        {"source": ".", "destination": "/workspace", "mode": "rw"},
    ],
    timeout="30s",
)

# Inspect configuration attributes
if box.no_exec:
    print("Subprocess execution is disabled")
print("Permitted binaries:", box.allowed_exec_paths)

# Run commands within the sandbox
result = box.exec("echo", ["hello-from-sandbox"])
if result.ok:
    print(result.stdout)
else:
    print("Execution failed:", result.stderr)
```

### Running Sandboxed Child Scripts

```python
# Run an external Starlark script under sandbox boundaries
result = sandbox.run_script(
    path="./untrusted_subtask.star",
    driver="landlock",
    profile="opaque",
    timeout="10s",
)
```

### Non-Escalation Security Invariant

When running inside an active parent sandbox (e.g. via `kite run --sandbox-opaque`), child sandboxes spawned via the Starlark `sandbox` module cannot elevate privileges beyond the parent. The runtime enforces an intersection rule:

$$\text{Effective Permissions} = \text{Parent Permissions} \cap \text{Requested Permissions}$$

If an outer sandbox has `network: none`, child sandboxes cannot request `network: host`.

## Actionable Denial Diagnostics

When the sandbox intercepts an unauthorized operation, Starkite formats the error with context identifying the active policy constraint:

- **`denied by no_exec policy`**: The profile or sandbox configuration set `no_exec: true`, preventing all child process execution.
- **`denied by allowed_exec_paths policy`**: The requested executable is not in the configured `allowed_exec_paths` allowlist.
- **`denied by W^X noexec policy (binary resides on a writable mount or /tmp)`**: The command attempted to execute a file located within a writable mount or temporary directory. Relocate the binary to a read-only path or run it through an allowlisted system interpreter.
- **`denied by shared memory (/dev/shm) isolation policy`**: Direct access to host shared memory files was blocked.
- **`denied by home directory isolation policy`**: When running with `$CWD` set to `$HOME` or root (`/`), sibling files in the home directory are isolated and inaccessible to protect credentials.

## Combining Sandboxing with Permissions

Sandbox isolation operates at the OS/kernel boundary, while Starkite permissions operate at the language/API boundary. Composing both provides defense in depth:

```bash
kite run ./untrusted.star --sandbox-opaque --permissions=deny-all
```

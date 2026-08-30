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
kite run ./script.star --sandbox
```

To target a specific profile, pass its name:

```bash
kite run ./script.star --sandbox=opaque
kite run ./script.star --sandbox=net-access
kite run ./script.star --sandbox=host
```

To override the sandbox execution driver, pass `--sandbox-driver`:

```bash
kite run ./script.star --sandbox=opaque --sandbox-driver=landlock
kite run ./script.star --sandbox=host --sandbox-driver=seatbelt
kite run ./script.star --sandbox=net-access --sandbox-driver=podman
kite run ./script.star --sandbox=opaque --sandbox-driver=gvisor
kite run ./script.star --sandbox-driver=podman   # Runs default profile in Podman
```

For shebang scripts (`#!/usr/bin/env kite`), configure the sandbox via environment variables:

```bash
# Set profile and/or driver via environment variables
STARKITE_SECURITY_SANDBOX=opaque STARKITE_SANDBOX_DRIVER=seatbelt ./script.star
```

## Built-in sandbox profiles

Starkite provides three standard sandbox profiles:

| Profile | Network | Filesystem Access | Purpose |
|---|---|---|---|
| `opaque` | None / Loopback | `$CWD` read/write, `/tmp` tmpfs | Completely offline execution; writes restricted to the working directory. |
| `net-access` | Full network | `opaque` + read-only CA certificates and `/etc/resolv.conf` | Outbound network allowed (e.g. HTTP clients or Git operations). |
| `host` | Full network | `net-access` + read-only `$HOME`, `/usr`, `/bin`, `/lib` | Allows reading host files and running system binaries while preventing modifications. |

## Custom sandbox profiles

Define custom sandbox profiles in `~/.starkite/config.yaml`. The `sandbox:` section is a pure profile mapping, where `default` is the profile selected when `--sandbox` is passed without an argument:

```yaml
# ~/.starkite/config.yaml
sandbox:
  # The default profile applied when `--sandbox` has no value
  default:
    base: net-access
    driver: podman             # Optional: binds a default driver to this profile

  # Custom named profiles
  dev:
    base: host                 # Inherits host settings
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
kite run ./build.star --sandbox=ci-builder
kite run ./build.star --sandbox=ci-builder --sandbox-driver=podman  # CLI driver overrides profile default
```

## Starlark `sandbox` Module API

In addition to CLI-level process isolation, Starkite provides a built-in `sandbox` Starlark module for programmatic sandbox creation and child script execution.

### Creating and Executing in a Sandbox

```python
# Query host driver
driver = sandbox.default_driver()

# Create a configured sandbox instance
box = sandbox.config(
    driver="auto",
    network="host",
    mounts=[
        {"source": ".", "destination": "/workspace", "mode": "rw"},
    ],
    timeout="30s",
)

# Run commands within the sandbox
result = box.exec("echo hello-from-sandbox")
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

When running inside an active parent sandbox (e.g. via `kite run --sandbox=opaque`), child sandboxes spawned via the Starlark `sandbox` module cannot elevate privileges beyond the parent. The runtime enforces an intersection rule:

$$\text{Effective Permissions} = \text{Parent Permissions} \cap \text{Requested Permissions}$$

If an outer sandbox has `network: none`, child sandboxes cannot request `network: host`.

## Combining Sandboxing with Permissions

Sandbox isolation operates at the OS/kernel boundary, while Starkite permissions operate at the language/API boundary. Composing both provides defense in depth:

```bash
kite run ./untrusted.star --sandbox=opaque --permissions=deny-all
```

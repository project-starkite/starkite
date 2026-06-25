---
title: "Sandbox"
description: "OS-level script runtime isolation via gVisor"
weight: 30
---

# Execution Sandbox

Starkite features an OS-level sandbox system that isolates script execution using a [gVisor](https://gvisor.dev) container. Confined scripts have no direct access to the host filesystem, the user's home directory, or host credentials.

> [!IMPORTANT]
> The sandbox system is **Linux-only**. It relies on Linux-specific namespaces and kernel intercepting. On macOS or Windows, running a script with the sandbox enabled returns an error; use [Script Permissions](permission.md) for protection on these platforms.

## Quick start

To execute a script within the default sandbox container, use the `--sandbox` flag:

```bash
kite ./script.star --sandbox
```

To target a specific profile, pass its name to the `--sandbox` flag, or use its shorthand flag alias (such as `--sandbox-opaque`):

```bash
kite ./script.star --sandbox=net-access
kite ./script.star --sandbox-opaque      # Equivalent to --sandbox=opaque
```

For shebang scripts (`#!/usr/bin/env kite`), enable the sandbox using the `STARKITE_SECURITY_SANDBOX` environment variable:

```bash
STARKITE_SECURITY_SANDBOX=opaque ./script.star
```

## Built-in sandbox profiles

Starkite provides three built-in sandbox profiles:

| Profile | Network | Filesystem Mounts | Purpose |
|---|---|---|---|
| `opaque` | Loopback only | `$CWD` read/write, `/tmp` tmpfs | Completely offline execution; writes restricted to the working directory. |
| `net-access` | Full network | `opaque` + read-only `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` | Egress networking allowed (such as HTTP clients or Git operations). |
| `host` | Full network | `net-access` + read-only `$HOME`, `/usr`, `/bin`, `/lib`, `/lib64` | Allows scripts to read home directory files and execute host binary utilities. |

Example using the `host` profile combined with local execution permissions:

```bash
kite ./deploy.star --sandbox=host --allow-local
```

## Custom sandbox profiles

Define custom sandbox profiles in `~/.starkite/config.yaml` under the `sandbox` section.

### Schema fields

| Field | Required | Description |
|---|---|---|
| `base` | No | Base profile to inherit from (`opaque`, `net-access`, `host`) |
| `network` | Yes (no if `base` set) | Network access mode (`host` or `loopback`) |
| `mounts[].source` | For `bind` types | Source path on the host filesystem (supports `$CWD` and `$HOME`) |
| `mounts[].destination` | Yes | Target mount path inside the sandbox |
| `mounts[].type` | No | Mount type (`bind` or `tmpfs`). Defaults to `bind`. |
| `mounts[].mode` | No | Mount permissions (`ro` or `rw`). Binds default to `ro`; tmpfs defaults to `rw`. |

*Path expansions are limited to `$CWD` and `$HOME`. Shell-style expansions (such as `~`) are not supported.*

### Example configuration

```yaml
# ~/.starkite/config.yaml
sandbox:
  default: net-access          # Shortcut for { base: net-access }
  dev:
    base: host                 # Inherits host settings
    mounts:
      - source: $HOME/.cache
        destination: $HOME/.cache
        mode: rw
  k8s-deploy:
    base: net-access           # Inherits egress network and TLS settings
    mounts:
      - source: $HOME/.kube/config
        destination: /etc/kubeconfig
        mode: ro
```

At runtime, execute with the custom profile:

```bash
kite ./deploy.star --sandbox=dev
STARKITE_SECURITY_SANDBOX=k8s-deploy ./deploy.star
```

### Implicit default profile

A profile named `default` in the `sandbox` config is automatically applied when the `--sandbox` flag is passed without specifying a profile name.

```bash
kite ./deploy.star --sandbox   # Applies the custom default profile (net-access)
```

## Combining sandboxing with permissions

You can combine sandbox isolation with Starkite script permissions to enforce high-security runtime policies:

```bash
kite ./untrusted.star --sandbox=opaque --permissions=allow-fs
```

See [Permission Guide](permission.md) for details.

## Sandbox Architecture

Starkite does not require an external container daemon (such as Docker or containerd) or a separate installation of `runsc`. Instead, the gVisor sandbox components are compiled directly into the single `kite` binary.

```
+-----------------------------------------------------------------+
|                         Host System                             |
|  +------------------+                                           |
|  |   kite run --sb  |  (Parent process, acts as coordinator)    |
|  +--------+---------+                                           |
|           | spawns (self-exec via /proc/self/exe)               |
|           v                                                     |
|  +-----------------------------------------------------------+  |
|  |                  gVisor Sandbox Boundary                  |  |
|  |                                                           |  |
|  |   +-------------------+       +-----------------------+   |  |
|  |   |  runsc-sandbox    | <---> |      runsc-gofer      |   |  |
|  |   |  (Sentry Kernel)  |       |  (File System Proxy)  |   |  |
|  |   +---------+---------+       +-----------+-----------+   |  |
|  |             | guest-exec                  | mounts        |  |
|  |             v                             v               |  |
|  |   +-------------------+               +-------+           |  |
|  |   |     /.kite        | <============ |  $CWD | (rw bind) |  |
|  |   | (Contained Process|               +-------+           |  |
|  |   |   runs script)    |                                   |  |
|  |   +-------------------+                                   |  |
|  +-----------------------------------------------------------+  |
+-----------------------------------------------------------------+
```

### Self-Execution Flow

When you run a script with sandboxing enabled, the runtime coordinates the following sequence:

1. **Self-Execution**: The parent `kite` process prepares a temporary OCI bundle directory containing a standard OCI `config.json` and an empty `rootfs`. It then self-executes the `kite` binary multiple times, cosmetically renaming `os.Args[0]` to `runsc-sandbox` and `runsc-gofer` to satisfy gVisor's internal command-line interface:
   * **Sentry (`runsc-sandbox`)**: A user-space kernel written in Go that intercepts all system calls made by the guest process, virtualizing them without exposing the host kernel.
   * **Gofer (`runsc-gofer`)**: A secure file system proxy that mediates all file system access from the sandbox to the host.
2. **Namespace Isolation**: The sandbox isolates the guest process across five namespaces:
   * **PID**: The script process cannot see or signal host processes.
   * **Mount**: The guest process has its own mount table, completely detached from the host `/`.
   * **IPC**: System V IPC and POSIX message queues are isolated.
   * **UTS**: Hostname and domain name are isolated.
   * **User**: The host's unprivileged UID and GID are mapped to container ID `0` (root inside the container), enabling secure, rootless sandbox boot.
3. **Network Namespace Bypass**: To run rootless without requiring network setup privileges, the OCI spec omits the `NetworkNamespace` configuration. Instead, the runner configures gVisor's internal network stack:
   * **Loopback Mode (`strict`/`opaque` profiles)**: Configures `config.NetworkNone`. The guest process sees only a loopback interface (`127.0.0.1`). No packets can leave the sandbox or reach the host.
   * **Host Network Mode (`net-access`/`host` profiles)**: Configures `config.NetworkHost`. The guest process shares the host's network namespace, allowing outbound connections (e.g., HTTP clients, SSH) while retaining filesystem and process isolation.
4. **Contained Execution**: The guest process is launched via the path `/.kite` (which is a read-only bind mount of the host `kite` binary) inside the sandbox. The argument list is prefixed with a private `__runtime__` marker. When the sandboxed `kite` process boots, it detects the marker, strips it, sets `STARKITE_INSIDE_SANDBOX=1` in its environment to prevent recursive sandboxing, and safely executes the Starlark script.

## Linux configuration (Ubuntu 24.04+)

On Ubuntu 24.04 and above, AppArmor restricts unprivileged user namespace creation by default. Running rootless gVisor may trigger the following error:
`sandbox: kernel restricts unprivileged user namespaces`

Choose one of the following setups to resolve this:

### Option A: Disable the restriction globally

For development or test environments:

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

To persist this change across system reboots:

```bash
echo 'kernel.apparmor_restrict_unprivileged_userns=0' | sudo tee /etc/sysctl.d/60-userns.conf
```

### Option B: Configure an AppArmor profile for Starkite

Create an AppArmor rule file at `/etc/apparmor.d/kite` to allow namespace creation for the Starkite binary:

```
abi <abi/4.0>,
include <tunables/global>

profile kite /usr/local/bin/kite flags=(unconfined) {
  userns,
  include if exists <local/kite>
}
```

*Ensure the binary path in the rule matches your actual Starkite installation path.*

Reload the AppArmor configuration:

```bash
sudo apparmor_parser -r /etc/apparmor.d/kite
```

## Limitations

* **OS Support**: Only Linux is supported. gVisor is not compatible with macOS or Windows kernels.
* **FS Egress**: Scripts requiring access to files outside `$CWD` must either run outside the sandbox, or mount explicit host directories in a custom profile.
* **Privileged Ports**: Rootless gVisor containers cannot bind to privileged ports (under 1024).

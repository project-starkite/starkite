---
title: "Sandbox"
description: "OS-level isolation via gVisor — what the script process can see"
weight: 30
---

# Sandbox

The sandbox runs a script inside a [gVisor](https://gvisor.dev) user-space kernel. The script gets a clean view of the filesystem, no access to the user's home directory, and no access to host credentials. It answers *what slice of the host the process can see at all*. This page covers the two built-in profiles, custom profiles, shebang-script integration, the schema, and Ubuntu 24.04+ setup.

The sandbox is **Linux-only**. On macOS or Windows, requesting a sandbox returns an error. For OS-agnostic gating of *which operations a script may invoke*, see [Permission](permission.md). The two compose cleanly.

## Quick start

For an explicit `kite` invocation, use the `--sandbox` flag:

```bash
kite ./script.star --sandbox             # default profile
kite ./script.star --sandbox=strict      # strict profile (offline)
kite test ./tests/ --sandbox
```

For a shebang script (`./script.star` via `#!/usr/bin/env kite`), use the `STARKITE_SECURITY_SANDBOX` env var:

```bash
STARKITE_SECURITY_SANDBOX=strict ./script.star
export STARKITE_SECURITY_SANDBOX=default
./other.star
```

| Lever | Use it for |
|---|---|
| `--sandbox=<profile>` | Explicit `kite run / test / exec / repl / watch`. |
| `STARKITE_SECURITY_SANDBOX=<profile>` | Shebang scripts; or to set a sandbox for a series of runs in a shell. |

Both accept the same profile values. The flag wins when both are set. An unset / empty value runs the script natively.

## Built-in profiles

| Profile | Network | Filesystem |
|---|---|---|
| `default` | Full host network. | `$CWD` rw, `/tmp` tmpfs, ro `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` |
| `strict` | Loopback only. | `$CWD` rw, `/tmp` tmpfs. No `/etc/*`. |

`--sandbox` (no value) and `STARKITE_SECURITY_SANDBOX=default` both select `default`.

### default

`default` provides host network access and TLS verification, with `$HOME` and host credentials hidden.

Inside the sandbox:

- The current directory is readable and writable. Project files, configs, and outputs live here.
- `/tmp` is a private writable tmpfs.
- `/etc/ssl/certs`, `/etc/resolv.conf`, `/etc/hosts`, and `/etc/nsswitch.conf` are read-only. HTTPS verification and DNS resolution work.
- The host network is reachable.

Not visible:

- `$HOME` and everything under it (`~/.ssh`, `~/.aws`, `~/.kube`, …).
- `/etc/passwd`, `/etc/shadow`, the rest of `/etc`.
- Other users' files, system binaries, kernel state.
- Any directory outside the current working directory (unless added by a custom profile).

### strict

`strict` blocks outbound network and removes all `/etc/*` mounts. Filesystem access is limited to `$CWD` (rw) and `/tmp` (private tmpfs).

```bash
kite ./analyze.star --sandbox=strict
STARKITE_SECURITY_SANDBOX=strict ./analyze.star
```

Inside the sandbox:

- The current directory is readable and writable.
- `/tmp` is a private writable tmpfs.
- Loopback networking works inside the sandbox: an `http.server()` and an `http.url("http://127.0.0.1:…")` client in the same script round-trip without leaving the sandbox.

Not available under `strict`:

- Outbound network — packets to non-loopback addresses fail with "network unreachable".
- DNS resolution — `/etc/resolv.conf` is not mounted.
- TLS verification — `/etc/ssl/certs` is not mounted.
- `/etc/hosts`, `/etc/nsswitch.conf`.

## Run from the project directory

The sandbox binds the **current working directory** read-write. Run kite from the project directory, not from `~`:

```bash
cd ~/projects/my-deployment
kite ./deploy.star --sandbox        # only ~/projects/my-deployment is visible

cd ~
kite ~/projects/my-deployment/deploy.star --sandbox  # exposes ALL of $HOME
```

If a script needs an SSH key, copy it into the project directory.

## Shebang scripts

`STARKITE_SECURITY_SANDBOX` from the surrounding environment applies to shebang-launched scripts:

```bash
export STARKITE_SECURITY_SANDBOX=strict
./compute.star
./other.star
unset STARKITE_SECURITY_SANDBOX
./debug.star
```

For a one-off:

```bash
STARKITE_SECURITY_SANDBOX=strict ./compute.star
```

To pin a script's sandbox at the file level, wrap kite in a shim:

```bash
#!/bin/sh
exec env STARKITE_SECURITY_SANDBOX=strict kite "$0.star" "$@"
```

Or with GNU `env -S` (Linux):

```
#!/usr/bin/env -S env STARKITE_SECURITY_SANDBOX=strict kite
```

## Custom profiles

Author a profile YAML and pass either the path or a name registered under the `sandbox:` section of `~/.starkite/config.yaml`.

### By file path

```yaml
# myprofile.yaml — default-like, plus a read-only kubeconfig mount.
network: host

mounts:
  - source: $CWD
    destination: $CWD
    mode: rw

  - destination: /tmp
    type: tmpfs

  - source: /etc/ssl/certs
    destination: /etc/ssl/certs
    mode: ro
    optional: true

  - source: /home/alice/.kube/config
    destination: /etc/kubeconfig
    mode: ro
```

```bash
kite ./deploy.star --sandbox=./myprofile.yaml
STARKITE_SECURITY_SANDBOX=./myprofile.yaml ./deploy.star
```

### By name in `~/.starkite/config.yaml`

```yaml
# ~/.starkite/config.yaml
sandbox:
  k8s-deploy:
    network: host
    mounts:
      - source: $CWD
        destination: $CWD
        mode: rw
      - destination: /tmp
        type: tmpfs
      - source: /home/alice/.kube/config
        destination: /etc/kubeconfig
        mode: ro
```

```bash
kite ./deploy.star --sandbox=k8s-deploy
STARKITE_SECURITY_SANDBOX=k8s-deploy ./deploy.star
```

### Schema

| Field | Required | Allowed values | Notes |
|---|---|---|---|
| `network` | yes | `host`, `sandbox-loopback` | `host` uses the host network. `sandbox-loopback` is loopback-only inside the sandbox. |
| `mounts[].source` | for `bind` | absolute path, or `$CWD` / `$CWD/sub` | Must exist at sandbox start unless `optional: true`. |
| `mounts[].destination` | yes | absolute path | Where the mount appears inside the sandbox. |
| `mounts[].type` | no (default `bind`) | `bind`, `tmpfs` | `tmpfs` ignores `source`. |
| `mounts[].mode` | no | `ro`, `rw` | Default `ro` for binds, `rw` for tmpfs. |
| `mounts[].optional` | no | bool | Bind mounts only. Skip silently when source is absent. |

`$CWD` and `$CWD/sub` are the only path expansions. `~` and other shell-style expansions are not supported.

## Combining with `--permissions`

The sandbox and `--permissions` are independent. They compose:

```bash
kite ./untrusted.star --sandbox=strict --permissions=allow-fs
```

`--permissions` enforces allow/deny rules on Starlark API calls (exec, network, filesystem, k8s, …) inside one process. The sandbox confines the OS view (filesystem visibility, process isolation, network reach) at the kernel level via gVisor. A bypass in one is contained by the other. See [Permission](permission.md).

## Ubuntu 24.04+ setup

Ubuntu 24.04 enables an AppArmor restriction on unprivileged user namespaces by default, which gVisor's rootless mode requires. The kite preflight reports:

```
sandbox: kernel restricts unprivileged user namespaces
```

Two ways to grant the required capability.

### Option A: disable the restriction system-wide

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

Persist across reboots:

```bash
echo 'kernel.apparmor_restrict_unprivileged_userns=0' | \
  sudo tee /etc/sysctl.d/60-userns.conf
```

### Option B: grant `userns,` to the kite binary

Create `/etc/apparmor.d/kite`:

```
abi <abi/4.0>,
include <tunables/global>

profile kite /usr/local/bin/kite flags=(unconfined) {
  userns,
  include if exists <local/kite>
}
```

Reload AppArmor:

```bash
sudo apparmor_parser -r /etc/apparmor.d/kite
```

The binary path may need adjustment for non-default install locations. The profile grants user-namespace creation only to the `kite` binary at the specified path.

## Limits

- macOS and Windows: not supported. The sandbox requires the Linux kernel.
- Scripts that need to read `$HOME`: don't use the sandbox, or write a custom profile that mounts the specific paths the script needs.
- Privileged ports (<1024): rootless gVisor cannot bind them.

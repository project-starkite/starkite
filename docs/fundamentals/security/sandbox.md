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
kite ./script.star --sandbox                 # config-defined default profile, else opaque
kite ./script.star --sandbox=net-access      # a built-in rung by name
kite ./script.star --sandbox-opaque          # boolean alias for --sandbox=opaque
kite test ./tests/ --sandbox
```

For a shebang script (`./script.star` via `#!/usr/bin/env kite`), use the `STARKITE_SECURITY_SANDBOX` env var:

```bash
STARKITE_SECURITY_SANDBOX=opaque ./script.star
export STARKITE_SECURITY_SANDBOX=net-access
./other.star
```

| Lever | Use it for |
|---|---|
| `--sandbox=<profile>` | Explicit `kite run / test / exec / repl / watch`. |
| `--sandbox-opaque` / `--sandbox-net-access` / `--sandbox-host` | Boolean aliases for the built-in rungs. Set at most one sandbox flag. |
| `STARKITE_SECURITY_SANDBOX=<profile>` | Shebang scripts; or to set a sandbox for a series of runs in a shell. |

The flag and the env var accept the same profile values; the flag wins when both are set. An unset / empty value runs the script natively. A bare `--sandbox` (or the value `default`) selects the profile named `default` in `config.yaml`'s `sandbox:` section when one is defined, and the `opaque` rung otherwise.

## Built-in profiles: the rung ladder

Three built-in profiles form a capability ladder; each rung's network mode and mount set is contained in the next.

| Rung | Network | Filesystem | Promise |
|---|---|---|---|
| `opaque` | Loopback only | `$CWD` rw, `/tmp` tmpfs | Sealed compute over the working tree |
| `net-access` | Full host network | opaque + ro `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` | Networked automation, host invisible |
| `host` | Full host network | net-access + ro `$HOME`, `/usr`, `/bin`, `/lib`, `/lib64` | Read the host, write only the tree |

### opaque

`opaque` blocks outbound network and mounts nothing beyond the working tree.

Inside the sandbox:

- The current directory is readable and writable.
- `/tmp` is a private writable tmpfs.
- Loopback networking works inside the sandbox: an `http.server()` and an `http.url("http://127.0.0.1:…")` client in the same script round-trip without leaving the sandbox.

Not available:

- Outbound network — packets to non-loopback addresses fail with "network unreachable".
- DNS resolution, TLS roots, `/etc/*` — nothing is mounted.
- `$HOME`, host binaries, kernel state.

### net-access

`net-access` adds host network access and TLS verification, with `$HOME` and host credentials hidden.

Inside the sandbox, beyond `opaque`:

- The host network is reachable.
- `/etc/ssl/certs`, `/etc/resolv.conf`, `/etc/hosts`, and `/etc/nsswitch.conf` are read-only — HTTPS verification and DNS resolution work. A support file absent on the host is skipped.

Not visible:

- `$HOME` and everything under it (`~/.ssh`, `~/.aws`, `~/.kube`, …).
- `/etc/passwd`, `/etc/shadow`, the rest of `/etc`.
- Host binaries, other users' files, kernel state.

### host

`host` adds a read-only view of `$HOME` and the host binary paths (`/usr`, `/bin`, `/lib`, `/lib64`): host tools run inside the sandbox, and configuration like `~/.kube` and `~/.ssh` is readable.

```bash
kite ./deploy.star --sandbox=host --allow-local
```

What `host` protects — host **integrity**: nothing outside `$CWD` and `/tmp` is writable. Writes cannot reach shell startup files, `~/.ssh`, or system paths, and nothing persists past exit except `$CWD` output. `$HOME` is read-only by design: a writable `$HOME` would be a delayed sandbox escape (`~/.bashrc`, `~/.gitconfig` hooks, and `~/.ssh/authorized_keys` execute outside the sandbox later).

What it does not protect — host **confidentiality**: secrets readable by the invoking user under `$HOME` are readable by the script. Do not use this rung for code you would not let read your home directory.

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
export STARKITE_SECURITY_SANDBOX=opaque
./compute.star
./other.star
unset STARKITE_SECURITY_SANDBOX
./debug.star
```

For a one-off:

```bash
STARKITE_SECURITY_SANDBOX=opaque ./compute.star
```

To pin a script's sandbox at the file level, wrap kite in a shim:

```bash
#!/bin/sh
exec env STARKITE_SECURITY_SANDBOX=opaque kite "$0.star" "$@"
```

Or with GNU `env -S` (Linux):

```
#!/usr/bin/env -S env STARKITE_SECURITY_SANDBOX=opaque kite
```

## Custom profiles

`--sandbox` accepts a profile name only: a built-in rung (`opaque`, `net-access`, `host`) or a profile defined under the `sandbox:` section of `config.yaml`. Both `./config.yaml` and `~/.starkite/config.yaml` are searched; the project-local file wins, so a repository can ship its own sandbox profiles.

### Defining a named profile

A profile value is either a full spec or a **scalar alias** to a built-in rung:

```yaml
# ~/.starkite/config.yaml
sandbox:
  default: net-access          # bare --sandbox now selects net-access on this machine
  k8s-deploy:
    network: host
    mounts:
      - source: $CWD
        destination: $CWD
        mode: rw
      - destination: /tmp
        type: tmpfs
      - source: $HOME/.kube/config
        destination: /etc/kubeconfig
        mode: ro
```

```bash
kite ./deploy.star --sandbox=k8s-deploy
STARKITE_SECURITY_SANDBOX=k8s-deploy ./deploy.star
```

The profile named `default` is reserved: it is what a bare `--sandbox` selects. An alias must name a built-in rung; built-in rung names cannot be redefined.

### Schema

| Field | Required | Allowed values | Notes |
|---|---|---|---|
| `network` | yes | `host`, `loopback` | `host` uses the host network. `loopback` is an isolated, loopback-only network inside the sandbox — no egress. |
| `mounts[].source` | for `bind` | absolute path, or `$CWD` / `$CWD/sub` / `$HOME` / `$HOME/sub` | Must exist at sandbox start; a missing path is an error. |
| `mounts[].destination` | yes | absolute path | Where the mount appears inside the sandbox. |
| `mounts[].type` | no (default `bind`) | `bind`, `tmpfs` | `tmpfs` takes no `source`. |
| `mounts[].mode` | no | `ro`, `rw` | Default `ro` for binds, `rw` for tmpfs. |

`$CWD` and `$HOME` (and their `/sub` forms) are the only path expansions. `~` and other shell-style expansions are not supported. Unknown fields in a profile are an error.

## Combining with `--permissions`

The sandbox and `--permissions` are independent. They compose:

```bash
kite ./untrusted.star --sandbox=opaque --permissions=allow-fs
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

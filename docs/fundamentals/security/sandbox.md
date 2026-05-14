---
title: "Sandbox"
description: "OS-level isolation with --sandbox / STARKITE_SECURITY_SANDBOX (Linux only)"
weight: 3
---

The sandbox runs your script inside a [gVisor](https://gvisor.dev) user-space
kernel. The script gets a clean view of the filesystem, no access to your
home directory, and no access to host credentials.

The sandbox is **Linux-only**. On macOS or Windows, requesting a sandbox
returns an error.

## Quick start

For an explicit `kite` invocation, use the `--sandbox` flag:

```bash
kite script.star --sandbox             # default profile
kite script.star --sandbox=strict      # strict profile (offline)
kite test ./tests/ --sandbox
```

For a shebang script (`./script.star` via `#!/usr/bin/env kite`), use the
`STARKITE_SECURITY_SANDBOX` env var:

```bash
STARKITE_SECURITY_SANDBOX=strict ./script.star
export STARKITE_SECURITY_SANDBOX=default
./other.star
```

| Lever | Use it for |
|---|---|
| `--sandbox=<profile>` | Explicit `kite run / test / exec / repl / watch`. |
| `STARKITE_SECURITY_SANDBOX=<profile>` | Shebang scripts; or to set a sandbox for a series of runs in a shell. |

Both accept the same profile values. The flag wins when both are set.
An unset / empty value runs the script natively.

## Built-in profiles

| Profile | Network | Filesystem |
|---|---|---|
| `default` | Full host network. | `$CWD` rw, `/tmp` tmpfs, ro `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` |
| `strict` | Loopback only. | `$CWD` rw, `/tmp` tmpfs. No `/etc/*`. |

`--sandbox` (no value) and `STARKITE_SECURITY_SANDBOX=default` both select
`default`.

### default

Use `default` when the script needs the network or has to verify TLS, but
shouldn't see your home directory or host credentials.

Inside the sandbox:

- The current directory is readable and writable. Your project files,
  configs, and outputs live here.
- `/tmp` is a private writable tmpfs.
- `/etc/ssl/certs`, `/etc/resolv.conf`, `/etc/hosts`, and
  `/etc/nsswitch.conf` are read-only. HTTPS verification and DNS
  resolution work.
- The host network is reachable.

Not visible:

- `$HOME` and everything under it (`~/.ssh`, `~/.aws`, `~/.kube`, …).
- `/etc/passwd`, `/etc/shadow`, the rest of `/etc`.
- Other users' files, system binaries, kernel state.
- Any directory outside the current working directory (unless added by a
  custom profile).

### strict

Use `strict` for compute-only workloads that don't need outbound network
access — parsing, transforms, analysis over project files.

```bash
kite analyze.star --sandbox=strict
STARKITE_SECURITY_SANDBOX=strict ./analyze.star
```

Inside the sandbox:

- The current directory is readable and writable.
- `/tmp` is a private writable tmpfs.
- Loopback networking works inside the sandbox: an `http.server()` and an
  `http.url("http://127.0.0.1:…")` client in the same script can round-trip.

Not available under `strict`:

- Outbound network — packets to non-loopback addresses fail with "network
  unreachable".
- DNS resolution — `/etc/resolv.conf` is not mounted.
- TLS verification — `/etc/ssl/certs` is not mounted.
- `/etc/hosts`, `/etc/nsswitch.conf`.

## Run from the project directory

The sandbox binds the **current working directory** read-write. Run kite
from the project directory, not from `~`:

```bash
cd ~/projects/my-deployment
kite deploy.star --sandbox        # only ~/projects/my-deployment is visible

cd ~
kite ~/projects/my-deployment/deploy.star --sandbox  # exposes ALL of $HOME
```

If your script needs an SSH key, copy it into the project directory.

## Shebang scripts

`STARKITE_SECURITY_SANDBOX` from the surrounding environment applies to
shebang-launched scripts:

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

Author your own profile YAML and pass either the path or a name registered
in `~/.starkite/security.yaml`.

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
kite deploy.star --sandbox=./myprofile.yaml
STARKITE_SECURITY_SANDBOX=./myprofile.yaml ./deploy.star
```

### By name in `~/.starkite/security.yaml`

```yaml
# ~/.starkite/security.yaml
permissions:
  # ...

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
kite deploy.star --sandbox=k8s-deploy
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

`$CWD` and `$CWD/sub` are the only path expansions. `~` and other
shell-style expansions are not supported.

## Combining with `--permissions`

The sandbox and `--permissions` are independent. They compose:

```bash
kite untrusted.star --sandbox=strict --permissions=strict
```

`--permissions` enforces allow/deny rules on Starlark API calls (exec,
network, filesystem, k8s, …) inside one process. The sandbox confines
the OS view (filesystem visibility, process isolation, network reach)
at the kernel level via gVisor. A bypass in one is contained by the
other. See [Permissions](permissions.md).

## Ubuntu 24.04+ setup

Ubuntu 24.04 enables an AppArmor restriction on unprivileged user
namespaces by default, which gVisor's rootless mode requires. If kite
reports:

```
sandbox: kernel restricts unprivileged user namespaces
```

apply one of the following.

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

Adjust the binary path if needed. This grants user-namespace creation only
to `kite`.

## Limits

- macOS and Windows: not supported. The sandbox requires the Linux kernel.
- Scripts that need to read `$HOME`: don't use the sandbox, or write a
  custom profile that mounts the specific paths the script needs.
- Privileged ports (<1024): rootless gVisor cannot bind them.

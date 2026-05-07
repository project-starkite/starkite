---
title: "Sandbox"
description: "OS-level isolation with --sandbox / STARKITE_SECURITY_SANDBOX (Linux only)"
weight: 3
---

The sandbox runs your script inside a [gVisor](https://gvisor.dev) sandbox:
a user-space kernel that mediates every syscall. The script gets a clean
view of the filesystem, can't see your home directory, and can't read
host credentials.

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
`STARKITE_SECURITY_SANDBOX` env var — shebang lines can't easily carry
flags:

```bash
STARKITE_SECURITY_SANDBOX=strict ./script.star
export STARKITE_SECURITY_SANDBOX=default
./other.star
```

The two are equivalent: same profiles, same syntax. The `--sandbox` flag
wins when both are set.

| Lever | When to use it |
|---|---|
| `--sandbox=<profile>` | Explicit `kite run / test / exec / repl / watch` invocations. |
| `STARKITE_SECURITY_SANDBOX=<profile>` | Shebang scripts; or any time you want to set the sandbox in the shell environment for a series of runs. |

Either way, an unset / empty value means "no sandbox" — the script runs
natively.

## Built-in profiles

There are two built-ins. `--sandbox` (no value) and
`STARKITE_SECURITY_SANDBOX=default` both select `default`.

| Profile | Network | Filesystem |
|---|---|---|
| `default` | Full host network — HTTPS, SSH, DNS, anything reachable from the host. | `$CWD` rw, `/tmp` tmpfs, ro `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` |
| `strict` | Loopback only. In-sandbox servers and clients can talk to each other; nothing reaches outside. | `$CWD` rw, `/tmp` tmpfs. **No `/etc/*` mounts.** No DNS, no TLS roots. |

### Default profile

The default profile is intended for "run a script I might not fully trust"
without needing to think about which files or paths it touches.

**Accessible:**

- The current directory — read **and** write. Files your script needs (config,
  scripts, generated output) belong here.
- `/tmp` — a private writable tmpfs scoped to the sandbox.
- `/etc/ssl/certs`, `/etc/resolv.conf`, `/etc/hosts`, `/etc/nsswitch.conf` —
  read-only, just enough for HTTPS and DNS to work.
- The network — full host network, same reachability as a normal `kite` run.

**Not accessible:**

- `$HOME` and everything under it (`~/.ssh`, `~/.aws`, `~/.kube`, …).
- `/etc/passwd`, `/etc/shadow`, and the rest of `/etc`.
- Other users' files, system binaries, kernel state.
- Any directory outside the current working directory unless your custom
  profile mounts it (see [Custom profiles](#custom-profiles) below).

### Strict profile

`strict` is the maximally-isolated built-in. Same empty rootfs as
default, but with the network and `/etc/*` mounts dropped. Use it for
compute-only workloads — parsing, transforms, analysis over project
files — where the script has no business reaching the network.

```bash
kite analyze.star --sandbox=strict
# or, for shebang scripts:
STARKITE_SECURITY_SANDBOX=strict ./analyze.star
```

What still works:

- The current directory — same `$CWD` rw bind as default.
- `/tmp` — same private tmpfs.
- **Loopback networking inside the sandbox** — an `http.server()` and an
  `http.url("http://127.0.0.1:…")` client can round-trip with each other.
  Useful for in-script test fixtures.

What `strict` blocks compared to default:

- No outbound — packets to non-loopback addresses fail with "network
  unreachable".
- No DNS resolution — `/etc/resolv.conf` isn't there.
- No TLS verification — `/etc/ssl/certs` isn't there.
- No name service config — no `/etc/nsswitch.conf`, no `/etc/hosts`.

If your script needs the network, use `default`. `strict` exists
specifically for the case where you don't.

## Tip: project directories, not `$HOME`

Run sandboxed scripts from the directory containing the project they operate
on. The sandbox binds **the current directory** — running from `~` would
expose your entire home directory through that bind. The integration tests
deliberately use a non-home temp directory for this reason.

```bash
cd ~/projects/my-deployment
kite deploy.star --sandbox        # only ~/projects/my-deployment is visible

cd ~
kite ~/projects/my-deployment/deploy.star --sandbox  # exposes ALL of $HOME — avoid
```

If your script needs an SSH key, copy it into the project directory rather
than relying on `~/.ssh/`.

## Shebang scripts

A `#!/usr/bin/env kite` script picks up `STARKITE_SECURITY_SANDBOX` from
the surrounding environment:

```bash
export STARKITE_SECURITY_SANDBOX=strict
./compute.star            # runs sandboxed
./other_compute.star      # also sandboxed
unset STARKITE_SECURITY_SANDBOX
./debug.star              # runs natively
```

For a one-off invocation:

```bash
STARKITE_SECURITY_SANDBOX=strict ./compute.star
```

Shebang scripts that should *always* run sandboxed can be wrapped:

```bash
#!/bin/sh
exec env STARKITE_SECURITY_SANDBOX=strict kite "$0.star" "$@"
```

…or use a tiny shim with GNU `env -S`:

```
#!/usr/bin/env -S env STARKITE_SECURITY_SANDBOX=strict kite
```

(The `-S` form on `env` requires GNU `env` 8.30+; available on most
modern Linux. macOS `/usr/bin/env` does **not** support `-S`.)

## Custom profiles

Beyond the two built-ins, you can author your own profile YAML. Use it
either by file path or by name in `~/.starkite/security.yaml`. Both
forms work via flag or env var — the value's just passed straight to
the profile resolver.

**As a file path** — pass `--sandbox=./myprofile.yaml` or set
`STARKITE_SECURITY_SANDBOX=./myprofile.yaml`:

```yaml
# myprofile.yaml — a custom profile that mounts the user's kubeconfig
# read-only alongside the default profile's defaults.
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
```

**As a named profile in `~/.starkite/security.yaml`** — same file the
permissions loader reads, with a `sandbox:` section:

```yaml
# ~/.starkite/security.yaml
permissions:
  # ... (consumed by --permissions)

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
# or:
STARKITE_SECURITY_SANDBOX=k8s-deploy ./deploy.star
```

### Schema

| Field | Required | Allowed values | Notes |
|---|---|---|---|
| `network` | yes | `host`, `sandbox-loopback` | `host` shares the host network namespace; `sandbox-loopback` runs gVisor's netstack with only the loopback interface. |
| `mounts[].source` | for `bind` | absolute path, or `$CWD` / `$CWD/sub` | Source path on the host. Must exist at sandbox start unless `optional: true`. |
| `mounts[].destination` | yes | absolute path | Where the mount appears inside the sandbox. |
| `mounts[].type` | no (default `bind`) | `bind`, `tmpfs` | `tmpfs` ignores `source`. |
| `mounts[].mode` | no | `ro`, `rw` | Default `ro` for binds, `rw` for tmpfs. |
| `mounts[].optional` | no | bool | Bind mounts only. Skip silently when source is absent. |

`$CWD` and `$CWD/...` are the only path expansions. `~` and other
shell-style expansions are deliberately unsupported — `$HOME` would make
profiles harder to audit and could surface user-data risks accidentally.

The runner adds `/proc`, `/dev`, and a bind mount of the kite binary at
`/.kite` regardless of your profile — these are plumbing the sandbox
runtime needs.

## Composing with `--permissions`

The sandbox and `--permissions` are independent layers:

| Lever | What it does |
|---|---|
| `--permissions=strict` | Blocks operations at the Starlark API level (`exec()`, file writes, network) — see [Permissions](../permissions/). |
| `--sandbox=…` / `STARKITE_SECURITY_SANDBOX=…` | Confines the OS view (filesystem, processes) — kernel-level. |

Use them together for the strongest restriction:

```bash
kite untrusted.star --sandbox=strict --permissions=strict
```

That combination gives defense in depth: the permissions layer blocks
unwanted module calls in-process, and even if a Starlark module is
compromised, the kernel layer denies the syscalls it would need to reach
your filesystem or network.

Use the sandbox alone when the script legitimately needs `exec()`, file
I/O, or network access but you want to keep it away from your
credentials and unrelated host state.

## Ubuntu 24.04+ setup

Ubuntu 24.04 ships with an AppArmor restriction that blocks unprivileged
user namespaces — the kernel feature gVisor's rootless mode depends on. If
you see an error like:

```
sandbox: kernel restricts unprivileged user namespaces
```

you have two options.

### Option A: turn the restriction off (simplest)

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

To make it persistent across reboots:

```bash
echo 'kernel.apparmor_restrict_unprivileged_userns=0' | \
  sudo tee /etc/sysctl.d/60-userns.conf
```

### Option B: grant `userns,` to the kite binary (more targeted)

Create `/etc/apparmor.d/kite` with:

```
abi <abi/4.0>,
include <tunables/global>

profile kite /usr/local/bin/kite flags=(unconfined) {
  userns,
  include if exists <local/kite>
}
```

Then reload AppArmor:

```bash
sudo apparmor_parser -r /etc/apparmor.d/kite
```

Adjust the path if `kite` lives somewhere else. This grants user-namespace
creation only to `kite`, leaving the system-wide restriction in place.

Starkite does not ship an AppArmor profile — install or write one suited to
your environment.

## When the sandbox is the wrong tool

- **macOS / Windows.** Not supported; gVisor is Linux-only.
- **Scripts that need to read your home directory.** The default profile
  hides `$HOME` on purpose. If you genuinely need it, run without the
  sandbox.
- **Scripts that bind privileged ports (<1024).** Rootless gVisor can't.

## How it works (briefly)

`kite` resolves the profile from `--sandbox` (CLI) or
`STARKITE_SECURITY_SANDBOX` (env). When set, it re-executes itself
inside a one-shot gVisor sandbox built on the fly: an empty rootfs with
only the curated mounts your profile declares, the kite binary itself
bind-mounted at `/.kite`, and a network mode picked from the profile
(host network for `default`, gVisor's loopback-only netstack for
`strict`). The sandbox is destroyed when your script exits.

The inner kite invocation sets `STARKITE_INSIDE_SANDBOX=1` so the
sandbox engagement check short-circuits — no recursion even though the
parent's flag/env is still in scope.

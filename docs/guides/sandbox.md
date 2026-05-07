---
title: "Sandbox"
description: "OS-level isolation with --sandbox (Linux only)"
weight: 3
---

The `--sandbox` flag runs your script inside a [gVisor](https://gvisor.dev)
sandbox: a user-space kernel that mediates every syscall. The script gets a
clean view of the filesystem, can't see your home directory, and can't read
host credentials.

`--sandbox` is **Linux-only**. On macOS or Windows, the flag returns an error.

## Quick start

```bash
kite script.star --sandbox
kite test ./tests/ --sandbox
```

That's it. Your script runs as before; `kite` re-executes itself inside the
sandbox so the user-visible behavior matches an unsandboxed run.

## What the default profile gives you

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
- Any directory outside the current working directory unless you explicitly
  allow it (custom profiles, planned).

A script under `--sandbox` can only see what you'd reasonably expect a script
to see in a freshly-cloned project directory.

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

## Composing with `--permissions`

`--sandbox` and `--permissions` are independent layers:

| Flag | What it does |
|---|---|
| `--permissions=strict` | Blocks operations at the Starlark API level (`exec()`, file writes, network) — see [Permissions](../permissions/). |
| `--sandbox` | Confines the OS view (filesystem, processes) — kernel-level. |

Use them together for the strongest restriction:

```bash
kite untrusted.star --sandbox --permissions=strict
```

Use `--sandbox` alone when the script legitimately needs `exec()`, file I/O,
or network access but you want to keep it away from your credentials and
unrelated host state.

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

## When `--sandbox` is the wrong tool

- **macOS / Windows.** Not supported; gVisor is Linux-only.
- **Scripts that need to read your home directory.** The default profile
  hides `$HOME` on purpose. If you genuinely need it, run without
  `--sandbox`.
- **Scripts that bind privileged ports (<1024).** Rootless gVisor can't.

## How it works (briefly)

`kite --sandbox` re-executes itself inside a one-shot gVisor sandbox built on
the fly: an empty rootfs with only the curated mounts above, the kite binary
itself bind-mounted at `/.kite`, and the host network shared so HTTP/SSH
work without further configuration. The sandbox is destroyed when your
script exits.

For details on the design, see the proposal under
`project-planning/starkite/sandbox-isolation.md`.

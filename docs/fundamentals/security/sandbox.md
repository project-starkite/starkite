---
title: "Sandbox"
description: "OS-level script runtime isolation via gVisor"
weight: 30
---

# Execution Sandbox

Starkite is shipped with a sandbox system that runs a script contained inside a [gVisor](https://gvisor.dev) container. The script runs isolated with no direct access to the host's filesystem, user's home directory, and no access to host credentials. The sandbox system runs on **Linux-only** because gVisor intercepts Linux syscalls and relies on user namespaces for rootless isolation. On macOS or Windows, requesting to run a script in a sandbox returns an error. On these OS platforms, you can still use script [Permission](permission.md) capabilities to define script protections.

## Quick start

Turn on the sandbox using the `--sandbox` flag when executing `kite` scripts:

```bash
kite ./script.star --sandbox                 # runs script isolated in a sandbox
kite ./script.star --sandbox=net-access      # specifies a sandbox profile to use
kite ./script.star --sandbox-opaque          # boolean alias for --sandbox=opaque
```

For a shebang script (`./script.star` via `#!/usr/bin/env kite`), use the `STARKITE_SECURITY_SANDBOX` env var:

```bash
STARKITE_SECURITY_SANDBOX=opaque ./script.star
export STARKITE_SECURITY_SANDBOX=net-access
./other.star
```

## Built-in sandbox profiles

Starkite comes with three built-in sandbox profiles that are defined below in increasing permissiveness.

| Rung | Network | Filesystem | Description |
|---|---|---|---|
| `opaque` | Loopback only | `$CWD` rw, `/tmp` tmpfs | Scripts can only access current working dir with no external networking |
| `net-access` | Full host network | opaque + ro `/etc/{ssl/certs,resolv.conf,hosts,nsswitch.conf}` | Outbound (egress) networking possible |
| `host` | Full host network | net-access + ro `$HOME`, `/usr`, `/bin`, `/lib`, `/lib64` | Scripts can read host's $HOME and execute binaries |

### opaque

`opaque` blocks outbound network and mounts nothing beyond the working tree.

- Scripts can read/write the current directory.
- `/tmp` is mounted as private writable tmpfs.
- Networking is through loopback where traffic never leaves the container.

### net-access

`net-access` adds host network access and TLS verification in addition to `opaque`.

- Script can read/write files in current working directory only.
- Outside networking traffic is reachable via the host's network.
- Read-only access to `/etc/ssl/certs`, `/etc/resolv.conf`, `/etc/hosts`, and `/etc/nsswitch.conf` for networking support.

### host
This is the most permissive profile. It adds read-only access to `$HOME` and to the default binary paths (`/usr`, `/bin`, `/lib`, `/lib64`).

```bash
kite ./deploy.star --sandbox=host --allow-local
```

## Custom profiles

Define custom profiles under the `sandbox:` section of `config.yaml`, in addition to the built-in profiles.

### Sandbox profile schema

The `sandbox` entry in `config.yaml` is a map used to specify sandbox rules for a running script. The following table shows members of the map.

| Field | Required | Allowed values | Notes |
|---|---|---|---|
| `base` | no | `opaque`, `net-access`, `host` | Allows a new rule to use a built-in as a base rule. |
| `network` | yes (no when `base` provided) | `host`, `loopback` | `host` uses the host network. `loopback` is loopback-only network inside the sandbox. |
| `mounts[].source` | for type `bind` | absolute path, `$CWD` or `$HOME` | Source path must exist at sandbox start. |
| `mounts[].destination` | yes | absolute path | Where the mount appears inside the sandbox. |
| `mounts[].type` | no (default `bind`) | `bind`, `tmpfs` | `tmpfs` takes no `source`. |
| `mounts[].mode` | no | `ro`, `rw` | Default `ro` for binds, `rw` for tmpfs. |

`$CWD` and `$HOME` (and their `/sub` forms) are the only path expansions. `~` and other shell-style expansions are not supported. 
Unknown fields in a profile are an error.

### Sandbox example
The following shows a config.yaml file with three profiles: `default`, `dev`, and `k8s-deploy`:

```yaml
# ~/.starkite/config.yaml
sandbox:
  default: net-access          # shortcut for { base: net-access }
  dev:
    base: host                 # uses built-in host as parent
    mounts:
      - source: $HOME/.cache
        destination: $HOME/.cache
        mode: rw
  k8s-deploy:
    base: net-access           # inherits host network + TLS/DNS support files
    mounts:
      - source: $HOME/.kube/config
        destination: /etc/kubeconfig
        mode: ro
```

At runtime, the custom sandbox profile can be selected as shown below:

```bash
kite ./deploy.star --sandbox=dev
STARKITE_SECURITY_SANDBOX=k8s-deploy ./deploy.star
```

### Setting a default profile

A sandbox profile named `default` has a special meaning. At runtime it will be automatically applied (if defined) when a profile name is not provided at launch time. For instance, using the previous `config.yaml` above, following will cause script to run in a sandbox with `net-access` only:

```bash
kite ./deploy.star --sandbox
```

## Combining sandbox with permissions

Starkite allows users to combine both sandbox and permission to create high security postures for running scripts. While the sandbox isolates the running script, additional permissions can be specified to restrict Starkite API calls.

```bash
kite ./untrusted.star --sandbox=opaque --permissions=allow-fs
```

See [Permission](permission.md).

## Ubuntu 24.04+ setup

On Ubuntu 24.04 and above, AppArmor restricts unprivileged user access to namespaces by default. This causes problems for gVisor's rootless mode:

```
sandbox: kernel restricts unprivileged user namespaces
```

### Option A: disable the restriction

On a dev or testing environment, disabling that restriction may be an option.

```bash
sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0
```

Or, persist that change across reboots:

```bash
echo 'kernel.apparmor_restrict_unprivileged_userns=0' | \
  sudo tee /etc/sysctl.d/60-userns.conf
```

### Option B: setup apparmor profile for Starkite binary

You can also create a Apparmor profile to allow the Starkite binary to create namespaces.

First, create a Apparmor rule file for Starkite's binary `/etc/apparmor.d/kite`:

```
abi <abi/4.0>,
include <tunables/global>

profile kite /usr/local/bin/kite flags=(unconfined) {
  userns,
  include if exists <local/kite>
}
```

> Note: make sure to set the proper path of the binary in the rule file above.

Next, reload AppArmor:

```bash
sudo apparmor_parser -r /etc/apparmor.d/kite
```

The profile grants user-namespace creation only to the `kite` binary at the specified path.

## Sandbox Limits

- macOS and Windows: not supported as the sandbox uses gVisor which requires the Linux kernel.
- Scripts that need to read `$HOME`: don't use the sandbox, or write a custom profile that mounts the specific paths the script needs.
- Privileged ports (<1024): rootless gVisor cannot bind them.

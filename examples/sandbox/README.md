# Sandbox examples

Run any of these scripts with the sandbox engaged:

```bash
# default profile (network + curated /etc mounts)
kite run ./netaccess-http-fetch.star --sandboxed --permissions=allow-net
# or with shortcut:
kite run ./netaccess-http-fetch.star --sandbox-net --permissions=allow-net

# opaque rung (no network, no host credentials)
kite run ./opaque-compute.star --sandbox-profile=opaque --permissions=allow-fs
# or with shortcut:
kite run ./opaque-compute.star --sandbox-opaque --permissions=allow-fs

# both layers — sandbox + permissions
kite run ./defense-in-depth.star --sandbox-opaque --permissions=deny-all

# programmatic execution using Starlark sandbox module
kite run ./sandbox-module-exec.star --permissions=allow-all
```

Or override the backend driver via `--sandbox-driver`:

```bash
kite run ./opaque-compute.star --sandbox-opaque --sandbox-driver=landlock
kite run ./opaque-compute.star --sandbox-opaque --sandbox-driver=seatbelt
kite run ./netaccess-http-fetch.star --sandbox-net --sandbox-driver=podman
```

Or via environment variables:

```bash
STARKITE_SANDBOX_PROFILE=net-access STARKITE_SANDBOX_DRIVER=podman ./netaccess-http-fetch.star
```

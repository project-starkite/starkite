# Sandbox examples

Run any of these scripts with the sandbox engaged:

```bash
# default profile (network + curated /etc mounts)
kite run ./netaccess-http-fetch.star --sandbox --permissions=allow-net

# opaque rung (no network, no host credentials)
kite run ./opaque-compute.star --sandbox=opaque --permissions=allow-fs

# both layers — sandbox + permissions
kite run ./defense-in-depth.star --sandbox=opaque --permissions=deny-all

# programmatic execution using Starlark sandbox module
kite run ./sandbox-module-exec.star --permissions=allow-all
```

Or via compound CLI selector specifying both driver and profile:

```bash
kite run ./opaque-compute.star --sandbox=landlock:opaque
kite run ./opaque-compute.star --sandbox=seatbelt:opaque
kite run ./netaccess-http-fetch.star --sandbox=podman:net-access
```

Or via environment variable:

```bash
STARKITE_SECURITY_SANDBOX=net-access ./netaccess-http-fetch.star
```

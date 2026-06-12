# Sandbox examples

Run any of these scripts with the sandbox engaged:

```bash
# default profile (network + curated /etc mounts)
kite ./netaccess-http-fetch.star --sandbox

# opaque rung (no network, no host /etc)
kite ./opaque-compute.star --sandbox=opaque

# both layers — sandbox + permissions
kite ./defense-in-depth.star --sandbox=opaque --permissions=deny-all
```

Or via env var, useful for shebang scripts:

```bash
STARKITE_SECURITY_SANDBOX=net-access ./netaccess-http-fetch.star
```

The sandbox is Linux-only. See `docs/fundamentals/security/sandbox.md` for setup
on Ubuntu 24.04+.

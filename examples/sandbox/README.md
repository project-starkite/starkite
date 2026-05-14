# Sandbox examples

Run any of these scripts with the sandbox engaged:

```bash
# default profile (network + curated /etc mounts)
kite default-http-fetch.star --sandbox

# strict profile (no network, no host /etc)
kite strict-compute.star --sandbox=strict

# both layers — sandbox + permissions
kite defense-in-depth.star --sandbox=strict --permissions=strict
```

Or via env var, useful for shebang scripts:

```bash
STARKITE_SECURITY_SANDBOX=default ./default-http-fetch.star
```

The sandbox is Linux-only. See `docs/fundamentals/security/sandbox.md` for setup
on Ubuntu 24.04+.

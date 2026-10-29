# Sandbox examples

Starkite runs all scripts under an OS-level sandbox by default (`deny-all`). Use `--sandbox-*` flags or profiles to grant capabilities:

```bash
# Network access (grants outbound network and unveils system DNS/TLS certificates)
kite run ./netaccess-http-fetch.star --sandbox-net-allow

# Working directory file processing (sandbox-path profile allows $CWD access)
kite run ./opaque-compute.star --sandbox-profile=sandbox-path

# Default zero-trust execution (deny-all: both kernel sandbox and Starlark gate active)
kite run ./defense-in-depth.star

# Programmatic sandbox execution (run unconfined to spawn nested child sandboxes)
kite run ./sandbox-module-exec.star --sandbox-enabled=false

# Seccomp-BPF network isolation test (network: none blocks socket creation vs --sandbox-net-allow)
kite run ./seccomp-network-test.star --sandbox-call-allow=http.client
kite run ./seccomp-network-test.star --sandbox-net-allow
```

Or select the backend driver via `--sandbox-driver`:

```bash
kite run ./opaque-compute.star --sandbox-profile=sandbox-path --sandbox-driver=landlock
kite run ./opaque-compute.star --sandbox-profile=sandbox-path --sandbox-driver=seatbelt
kite run ./netaccess-http-fetch.star --sandbox-net-allow --sandbox-driver=podman
```

Or configure via environment variables:

```bash
STARKITE_SANDBOX_PROFILE=sandbox-net STARKITE_SANDBOX_DRIVER=podman ./netaccess-http-fetch.star
```

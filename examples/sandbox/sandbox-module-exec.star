#!/usr/bin/env kite
# sandbox-module-exec.star — Demonstrates programmatic sandbox execution via
# the Starlark sandbox module.
#
# Run:
#   kite run ./sandbox-module-exec.star --permissions=allow-all

# Query default driver available on this platform
driver_name = sandbox.default_driver()
print("default driver: %s" % driver_name)

# Configure a sandbox instance programmatically
box = sandbox.config(
    network="host",
    mounts=[
        {"source": ".", "destination": "/workspace", "mode": "rw"},
    ],
)
print("configured sandbox: driver=%s" % box.driver)

# Execute a command inside the configured sandbox
res = box.exec("echo hello-from-starlark-sandbox")
assert(res.ok, "sandbox execution failed: %s" % res.stderr)
assert("hello-from-starlark-sandbox" in res.stdout, "unexpected output: %s" % res.stdout)
print("box.exec ok: %s" % res.stdout.strip())

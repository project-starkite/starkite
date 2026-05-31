---
title: "Run commands across SSH hosts"
description: "Multi-host command execution with ssh.config()"
weight: 4
---

# Run commands across SSH hosts

Starkite's `ssh` module is designed for fleet operations — running the same command across a set of hosts and collecting per-host results. `ssh.config()` builds a client bound to a list of hosts with shared credentials. Every method on the client (`exec`, `upload`, `download`) fans out across the list and returns a per-host result array. Connection setup happens once when the client is created and is reused across calls.

The pattern fits SRE workflows: check uptime across a web fleet, push config files to a node group in parallel, gather log snippets after an incident. Single-host invocations use the same API with a one-element host list.

**Source:** [`examples/core/remote-check.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/remote-check.star) (excerpted)

## Script

```python
#!/usr/bin/env kite

# Configure once, reuse across calls
fleet = ssh.config(
    hosts = ["web-1", "web-2", "web-3"],
    user  = "deploy",
    key   = "~/.ssh/id_ed25519",
    timeout = "30s",
)

# One command, three hosts
results = fleet.exec("uptime -p")

# Tabular output
t = table.new(["HOST", "STATUS", "OUTPUT"])
for r in results:
    status = "OK" if r.ok else "FAIL"
    t.add_row(r.host, status, r.stdout.strip() if r.ok else r.stderr[:60])
print(t.render())
```

## Run it

```bash
kite run examples/core/remote-check.star
```

Expected output (host-specific values differ):

```
+--------+--------+--------------------+
| HOST   | STATUS | OUTPUT             |
+--------+--------+--------------------+
| web-1  | OK     | up 14 days, 3 hours |
| web-2  | OK     | up 7 days, 2 hours  |
| web-3  | OK     | up 21 days          |
+--------+--------+--------------------+
```

## What's happening

- `ssh.config(hosts=[...], user=..., key=...)` returns an `SSHClient`. The `hosts` list determines fan-out width.
- `.exec(cmd)` returns a `list[SSHResult]`, one per host. Each result has `.host`, `.ok`, `.stdout`, `.stderr`, `.exit_code`.
- For sudo, jump-hosts, file transfer (SCP), and concurrent execution policy, see the full `ssh` reference.

## See also

- [`ssh` reference](../references/api/ssh.md) — connection options, `exec`, `upload`, `download`, sudo, jump hosts
- [`table` reference](../references/api/table.md) — ASCII tables for tabular output

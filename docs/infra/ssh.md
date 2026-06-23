---
title: "Remote commands (SSH)"
description: "Multi-host command execution with ssh.config()"
weight: 40
---

# Run commands across SSH hosts

Fleet operations rarely target a single machine. You want to ask the same question of a set of hosts — what's the uptime, is the service running, did the config land — and collect the answers in one place. The `ssh` module is built for exactly that shape of work. You configure a client once against a list of hosts, and every method on it fans out across that list and hands back a per-host result.

The pattern fits the everyday SRE loop: check uptime across a web fleet, push a config file to a node group in parallel, gather log snippets after an incident. A single host is not a special case — it is a one-element host list, run through the same API. Connection setup happens once, when you build the client, and is reused across every subsequent call, so you pay the handshake cost a single time rather than on each command.

**Source:** [`examples/core/remote-check.star`](https://github.com/project-starkite/starkite/blob/main/examples/core/remote-check.star) (excerpted)

## Script

You build the client with `ssh.config()`, naming the hosts and the credentials they share, then call `exec` to send one command to all of them at once. The result is a list you can walk straight into a table:

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

The loop reads each result's `.ok` flag to decide between the command's `.stdout` and its `.stderr`, so a host that failed shows its error in the same row a successful host shows its output. Nothing here special-cases the count of hosts — the same loop handles one host or a hundred.

## Run it

Hand the script to `kite run` as you would any other:

```bash
kite run ./examples/core/remote-check.star
```

Each host contributes one row, and the `STATUS` column tells you at a glance which ones answered (host-specific values differ):

```
+--------+--------+--------------------+
* HOST   | STATUS | OUTPUT             |
+--------+--------+--------------------+
| web-1  | OK     | up 14 days, 3 hours |
| web-2  | OK     | up 7 days, 2 hours  |
| web-3  | OK     | up 21 days          |
+--------+--------+--------------------+
```

A failed host would show `FAIL` and the start of its `stderr` instead, so one unreachable machine does not abort the run or hide the hosts that did respond.

## What's happening

The whole flow rests on two pieces. `ssh.config(hosts=[...], user=..., key=...)` returns an `SSHClient`, and the `hosts` list you pass decides the fan-out width — every later call dispatches to exactly those hosts. Then `.exec(cmd)` returns a `list[SSHResult]`, one entry per host, each carrying `.host`, `.ok`, `.stdout`, `.stderr`, and `.code`. Because the result is an ordinary list, you process it with the loops and comprehensions you already know.

This sits in the `allow-net` permission profile: `ssh` connect and transfer are network operations, so a script that uses them needs `--permissions=allow-net` (or a looser profile). Under the default `deny-all` the calls are blocked, which keeps a script from reaching out over the network unless you say so.

When you outgrow the basic loop — sudo, jump hosts, file transfer over SCP, concurrent execution policy — the full reference covers the additional options.

## See also

- [`ssh` reference](../references/api/ssh.md) — connection options, `exec`, `upload`, `download`, sudo, jump hosts
- [`table` reference](../references/api/table.md) — ASCII tables for tabular output

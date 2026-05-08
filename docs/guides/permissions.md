---
title: "Permissions"
description: "Allow/deny rules controlling what scripts can do"
weight: 2
---

`--permissions` controls what gated operations a script may perform —
filesystem access, command execution, network calls, Kubernetes
operations, and other privileged actions. It works on every OS and runs
in-process: no subprocess, no kernel features needed.

For OS-level isolation (filesystem visibility, process containment),
see the [Sandbox](sandbox.md) guide. The two compose.

## Trust mode (default)

Without `--permissions`, scripts run in trust mode and may perform any
operation:

```bash
kite script.star
```

Trust mode is the right default for scripts you wrote or audited
yourself. For scripts you don't fully trust, pick a profile.

## Quick start

```bash
kite script.star --permissions=strict      # built-in: $CWD-only fs
kite script.star --permissions=deny-all    # block everything
kite script.star --permissions=./team.yaml#deploy  # custom profile
```

Or via per-script frontmatter (the script declares the rules it needs):

```python
#!/usr/bin/env kite
# permissions: strict

print("running with strict permissions")
```

The `--permissions` flag wins when both are set.

## Built-in profiles

| Profile | What it allows |
|---|---|
| `allow-all` | Every gated operation. Equivalent to trust mode. |
| `strict` | Filesystem read/write/delete under `$CWD` only. Everything else (exec, network, env, ssh, k8s, ai, mcp, io) is denied. |
| `deny-all` | Nothing. Pure utility modules (`strings`, `json`, `yaml`, `time`, `regexp`, `math`, …) still work because they don't go through the permission check. |

```bash
kite analyze.star --permissions=strict
kite untrusted.star --permissions=deny-all
```

## Custom profiles

Author your own profile YAML — by file path or by name in
`~/.starkite/security.yaml`.

### By file path

```yaml
# team-deploy.yaml
permissions:
  deploy:
    default: deny
    allow:
      - fs.read($CWD/**)
      - fs.write($CWD/build/**)
      - os.exec(make*)
      - os.exec(go*)
      - http.client
    deny:
      - http.client(*.internal.*)
```

```bash
kite build.star --permissions=./team-deploy.yaml
```

When the file holds exactly one profile, the `#name` fragment is
optional. With multiple, select one explicitly:

```yaml
# team.yaml
permissions:
  deploy: { default: deny, allow: [...] }
  ci:     { default: deny, allow: [...] }
```

```bash
kite build.star --permissions=./team.yaml#deploy
```

### By name in `~/.starkite/security.yaml`

```yaml
# ~/.starkite/security.yaml
permissions:
  deploy:
    default: deny
    allow:
      - fs.read($CWD/**)
      - fs.write($CWD/build/**)
      - http.client
  ci:
    default: deny
    allow:
      - "fs.*"
      - os.exec
```

```bash
kite build.star --permissions=deploy
kite ci-task.star --permissions=ci
```

### Inline rules

For one-off invocations, pass rules directly on the command line. The
value must start with `allow:` or `deny:`. Multiple rules within one
clause are comma-separated; clauses are separated by `;`:

```bash
# Single clause: allow fs.read only
kite script.star --permissions=allow:fs.read

# Multiple rules in one clause
kite script.star --permissions='allow:fs.read,fs.write,os.exec'

# Multiple clauses: allow these, then explicitly deny something
kite script.star --permissions='allow:fs.read($CWD/**),fs.write($CWD/**);deny:http.client'
```

Inline rules use `default: deny` — anything not explicitly allowed is
denied.

## Rule grammar

A rule has the form:

```
module.category[(functions:resource)]
```

The functions list and resource are both optional. When both appear,
they're separated by `:`.

| Pattern | Matches |
|---|---|
| `*.*` | every module, every category, every operation |
| `fs.*` | every category in `fs` |
| `fs.read` | any function in `fs.read`, any resource |
| `fs.read(/etc/**)` | any function in `fs.read`, resource matching glob |
| `fs.read(read_file:*)` | only the `read_file` function, any resource |
| `fs.read(read_file,read_bytes:/etc/**)` | either function, resource matching glob |
| `os.exec(make*)` | any function in `os.exec`, resource (the command string) starting with `make` |

Deny rules are evaluated first; allow rules second; the profile's
`default` resolves the unmatched case.

### Path expansion

`$CWD` and `$HOME` expand at startup using the process's working
directory and the user's home:

```yaml
allow:
  - fs.read($CWD/**)        # any file under the project directory
  - fs.read($HOME/.config/myapp/*)
```

Resources without these prefixes are matched verbatim against globs.

### Function lists vs resources

The contents inside parentheses are parsed as a function list only when
they consist of bare identifiers separated by commas, followed by `:`.
Otherwise they're treated as a resource pattern:

```
fs.read(/etc/**)              → resource: /etc/**
fs.read(read_file:*)          → functions: [read_file], resource: *
fs.read(read_file,glob:/x/*)  → functions: [read_file, glob], resource: /x/*
fs.read(/some,path:with-colon)→ resource: /some,path:with-colon  (no valid funclist prefix)
```

## Modules and categories

These categories go through the permission check. Anything not listed
(string manipulation, data encoding, math, time, regexp, templates,
etc.) is unchecked and always works.

| Module | Categories | What's checked |
|---|---|---|
| `fs` | `read`, `write`, `delete` | path access |
| `os` | `exec`, `env`, `process` | command execution, env reads/writes, chdir/exit |
| `http` | `client`, `server` | outgoing HTTP, listening servers |
| `ssh` | `connect`, `transfer` | remote exec, SCP up/down |
| `k8s` | `read`, `write`, `exec`, `config` | API access, kubectl-exec, kubeconfig load |
| `ai` | `generate` | LLM calls (model name as resource) |
| `mcp` | `client`, `server` | MCP connections + servers |
| `io` | `prompt` | interactive prompts |

## Composing with `--sandbox`

`--permissions` and `--sandbox` are independent layers. Use them
together for defense in depth:

```bash
kite untrusted.star --sandbox=strict --permissions=strict
```

`--permissions` blocks operations at the Starlark API level inside one
process. `--sandbox` confines the OS view (filesystem, processes,
network) at the kernel level via gVisor. A bypass in one is contained
by the other. See [Sandbox](sandbox.md) for sandbox details.

## Permission errors

When a rule denies an operation:

```python
# Under --permissions=strict
os.exec("echo hi")
# Error: permission denied: os.exec.exec("echo hi") — no matching allow rule
```

Error messages list the matched rule (for deny) or the available allow
rules (when nothing matched).

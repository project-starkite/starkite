---
title: "Getting Started"
description: "Install starkite and write your first script"
weight: 1
---

# Getting Started

Starkite (pronounced *"star-kite"*) is a secure runtime for cloud-native and agentic-AI automation. Scripts are written in [Starlark](what-is-starlark.md) — a deterministic, Python-derived language — and run inside a single binary that ships its own batteries: a module library, a permission engine, and an optional gVisor sandbox.

This page walks you from a fresh install to running your first script in under five minutes.

## Editions

Starkite ships as four independent binaries that share the same script language and core modules. Pick the one that matches what you want to automate:

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | you want everything in one binary — recommended for new users |
| `kitecmd` | base only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

A single host can install one, two, or all four. Each is a stand-alone binary. `kite` is a strict superset of `kitecmd`/`kitecloud`/`kiteai`. See [Editions](../fundamentals/editions.md) for the deeper picture.

## Install

See [Install (native)](install/native.md) for source builds and pre-built releases, or [Install (container)](install/container.md) to pull `ghcr.io/project-starkite/kite`.

After install, verify:

```bash
kite version
```

## Your first script

Create `hello.star`:

```python
#!/usr/bin/env kite
# hello.star — your first starkite script

name = var_str("name", "World")
print("Hello, " + name + "!")

log.info("Running on", attrs={
    "platform": runtime.platform(),
    "arch":     runtime.arch(),
})
```

Run it three different ways — they're all equivalent:

```bash
kite hello.star                   # path → run
kite run hello.star               # explicit subcommand
chmod +x hello.star && ./hello.star   # shebang
```

Pass a variable:

```bash
kite hello.star --var name=Alice
```

You should see:

```
Hello, Alice!
time=... level=INFO msg="Running on" platform=darwin arch=arm64
```

## Other things to try

| Command | What it does |
|---|---|
| `kite repl` | Interactive REPL — explore modules and try expressions |
| `kite exec 'print(os.exec("hostname"))'` | Run a one-liner without a script file |
| `kite validate hello.star` | Parse-and-typecheck without executing |
| `kite test path/to/tests/` | Run all `*_test.star` files under a directory |
| `kite watch hello.star` | Re-run on every save |

## Run with restricted permissions

By default `kite` runs in **trust mode** — scripts can do anything the user can do. The `--permissions=strict` flag flips the default to deny-all, and every privileged operation (filesystem write, command exec, network call, even `var_str`) must be explicitly granted via a permission rule:

```bash
kite hello.star --permissions=strict   # fails: no rules → every op is denied
```

`--permissions` is most useful with a profile or a frontmatter block in the script that declares the rules the script needs. See [Permissions](../fundamentals/security/permissions.md) for the rule syntax and the built-in profiles.

## Run inside a sandbox (Linux)

For untrusted scripts, the `--sandbox` flag runs the script inside a [gVisor](https://gvisor.dev) user-space kernel. The script sees only the current directory, has no access to `$HOME` or host credentials, and uses a clean view of the filesystem:

```bash
kite hello.star --sandbox             # default profile: network ok, no $HOME
kite hello.star --sandbox=strict      # offline, $CWD-only
```

For shebang scripts, set `STARKITE_SECURITY_SANDBOX` instead:

```bash
STARKITE_SECURITY_SANDBOX=strict ./hello.star
```

The sandbox is Linux-only and composes with `--permissions` for defense in depth. See [Sandbox](../fundamentals/security/sandbox.md).

## What's next

- [What is Starlark?](what-is-starlark.md) — the language behind starkite scripts
- [CLI Reference](../references/cli/index.md) — all available subcommands and flags
- [API Reference](../references/api/index.md) — the full builtin module catalog
- [Language](../fundamentals/language.md) — variables and the `try_` pattern
- [Permissions](../fundamentals/security/permissions.md) — rules, profiles, and the security model
- [Sandbox](../fundamentals/security/sandbox.md) — OS-level isolation with gVisor (Linux)

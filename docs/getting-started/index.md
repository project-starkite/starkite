---
title: "Getting Started"
description: "Install starkite and write your first script"
weight: 1
---

# Getting Started

This page walks you from a fresh checkout to running your first script in under five minutes.

## Editions

Starkite ships as three independent binaries that share a common runtime. Pick the one that matches what you want to automate — they all share the same script language and core modules:

| Binary | Adds on top of base | Use when |
|---|---|---|
| `kite` | base modules only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kite-cloud` | Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kite-ai` | LLM clients, MCP server/client | agentic AI tools and orchestration |

A single host can install one, two, or all three. Each is a stand-alone binary.

## Install

### From source (recommended during development)

The repository is a Go workspace with one module per edition. Build the editions you need:

```bash
git clone https://github.com/project-starkite/starkite.git
cd starkite

make build-core    # produces ./kite
make build-cloud   # produces ./kite-cloud
make build-ai      # produces ./kite-ai
# or:
make build         # builds all three
```

Or build a single edition directly:

```bash
cd core  && go build -o ../kite .
cd cloud && go build -o ../kite-cloud .
cd ai    && go build -o ../kite-ai .
```

Move the binary onto your `PATH`:

```bash
sudo install -m 0755 ./kite /usr/local/bin/kite
```

### From GitHub Releases

Download a pre-built binary for your platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

Release assets follow the `<binary>-<os>-<arch>` pattern:

- `kite-linux-amd64`, `kite-linux-arm64`, `kite-darwin-amd64`, `kite-darwin-arm64`, `kite-windows-amd64.exe`
- `kite-cloud-*` (same OS/arch matrix)
- `kite-ai-*` (same OS/arch matrix)

Rename the downloaded file to `kite` (or `kite-cloud` / `kite-ai`), make it executable, and place it on your `PATH`.

## Verify the install

```bash
kite version
```

Expected output (your commit and Go version will differ):

```
kite version v0.1.0 (base)
  edition: base
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

`kite-cloud version` reports `(cloud)`; `kite-ai version` reports `(ai)`.

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

## Run with a sandbox

By default `kite` runs in **trust mode** — scripts can do anything the user can do. The `--sandbox` flag flips the default to deny-all, and every privileged operation (filesystem write, command exec, network call, even `var_str`) must be explicitly granted via a permission rule:

```bash
kite hello.star --sandbox   # fails: no rules → every op is denied
```

The `--sandbox` flag is most useful with a profile or a frontmatter block in the script that declares the rules the script needs. See [Permissions](../guides/permissions.md) for the rule syntax and the built-in profiles.

## What's next

- [CLI Reference](../cli/index.md) — all available subcommands and flags
- [Module Reference](../modules/index.md) — the full builtin module catalog
- [Variables](../guides/variables.md) — `--var`, `--var-file`, and the `var_*` builtins
- [Error Handling](../guides/error-handling.md) — the `try_` pattern
- [Permissions](../guides/permissions.md) — sandbox rules and profiles

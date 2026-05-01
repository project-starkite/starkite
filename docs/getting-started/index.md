---
title: "Getting Started"
description: "Install starkite and write your first script"
weight: 1
---

# Getting Started

This page walks you from a fresh checkout to running your first script in under five minutes.

## Editions

Starkite ships as four independent binaries that share the same script language and core modules. Pick the one that matches what you want to automate:

| Binary | Modules | Use when |
|---|---|---|
| `kite` | base + Kubernetes + GenAI/MCP (all-in-one) | you want everything in one binary — recommended for new users |
| `kitecmd` | base only (os, fs, http, ssh, json, yaml, time, log, …) | system scripts, CI tasks, general automation |
| `kitecloud` | base + Kubernetes (`k8s` module + `kite kube` subcommands) | cloud-native ops, manifest workflows |
| `kiteai` | base + LLM clients + MCP server/client | agentic AI tools and orchestration |

A single host can install one, two, or all four. Each is a stand-alone binary. `kite` is a strict superset of `kitecmd`/`kitecloud`/`kiteai`, so most examples on this site work with any edition that includes the modules they touch.

## Install

### From source (recommended during development)

The repository is a Go workspace with one module per edition. Build the editions you need — local builds land in `./bin/`:

```bash
git clone https://github.com/project-starkite/starkite.git
cd starkite

make build              # all four binaries → ./bin/
# or:
make build-all          # ./bin/kite       (all-in-one)
make build-base         # ./bin/kitecmd   (base only)
make build-cloud        # ./bin/kitecloud  (base + k8s)
make build-ai           # ./bin/kiteai     (base + LLM/MCP)
```

Move the binary onto your `PATH`:

```bash
sudo install -m 0755 ./bin/kite /usr/local/bin/kite
```

### From GitHub Releases

Download a pre-built binary for your platform from [GitHub Releases](https://github.com/project-starkite/starkite/releases).

Release assets follow the `<binary>-<os>-<arch>` pattern:

- `kite-linux-amd64`, `kite-linux-arm64`, `kite-darwin-amd64`, `kite-darwin-arm64`, `kite-windows-amd64.exe`
- `kitecmd-*`, `kitecloud-*`, `kiteai-*` (same OS/arch matrix)

Rename the downloaded file to `kite` (or `kitecmd` / `kitecloud` / `kiteai`), make it executable, and place it on your `PATH`.

## Verify the install

```bash
kite version
```

Expected output (your commit and Go version will differ):

```
kite version v0.1.0 (all)
  edition: all
  commit:  <git-sha>
  built:   <timestamp>
  go:      go1.26.1
  os/arch: darwin/arm64
```

`kitecmd version` reports `(base)`, `kitecloud version` reports `(cloud)`, `kiteai version` reports `(ai)`.

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

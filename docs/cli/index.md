---
title: "CLI Reference"
description: "Starkite command-line interface"
weight: 10
---

Starkite ships as four binaries — `kite` (all-in-one), `kitecmd` (base only), `kitecloud` (base + Kubernetes), `kiteai` (base + LLM/MCP). Every command on this page works in any edition that includes the modules it touches; edition-specific commands are flagged below.

The examples below use `kite` for brevity. Substitute `kitecmd`/`kitecloud`/`kiteai` if you've installed a lean edition.

## Commands

### Script execution

Available in **every** edition (base, cloud, ai, all).

| Command | Purpose |
|---------|---------|
| [`kite run`](run.md) | Execute a starkite script |
| [`kite exec`](exec.md) | Execute inline Starlark code |
| [`kite repl`](repl.md) | Start an interactive Read-Eval-Print-Loop (REPL) |
| [`kite watch`](watch.md) | Watch and re-execute script on file changes |
| [`kite test`](test.md) | Run test functions in `_test.star` files |
| [`kite validate`](validate.md) | Validate script syntax without executing |
| [`kite init`](init.md) | Scaffold a new starkite project |

### Maintenance

Available in **every** edition.

| Command | Purpose |
|---------|---------|
| [`kite version`](version.md) | Print version information |
| [`kite update`](update.md) | Update starkite to the latest version |
| [`kite edition`](edition.md) | Manage starkite editions (base, cloud, ai) |
| [`kite module`](module.md) | Manage external modules (starlark + WASM) |

### Cloud commands

Available in `kite` (all-in-one) and `kitecloud`.

| Command | Purpose |
|---------|---------|
| [`kite kube`](kube.md) | Kubernetes artifact generation (`gen-controller-artifacts`, `gen-webhook-artifacts`) |

## Global Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--var key=value` | Set a script variable | |
| `--var-file path` | Load variables from YAML file | |
| `--output format` | Output format: text, json, yaml, table | `text` |
| `--debug` | Enable debug logging | `false` |
| `--dry-run` | Preview commands without executing | `false` |
| `--timeout seconds` | Script execution timeout | `300` |
| `--trust` | Trust mode: allow all operations | `false` [^1] |
| `--sandbox` | Sandbox mode: restrict to safe operations | `false` |

[^1]: When neither `--trust` nor `--sandbox` is set, trust mode is the default behavior. The flag itself defaults to `false`; setting it explicitly only matters when overriding an env-var-configured sandbox.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `STARKITE_DEBUG` | Set to `1` or `true` to enable debug mode |
| `STARKITE_OUTPUT` | Default output format |
| `STARKITE_TIMEOUT` | Default timeout in seconds |
| `STARKITE_VAR_*` | Variable injection (e.g., `STARKITE_VAR_DB_HOST=localhost` → `var_str("db.host")`) |

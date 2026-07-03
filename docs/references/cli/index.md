---
title: "CLI Reference"
description: "Starkite command-line interface"
weight: 10
---

The default binary is `kite` (all-in-one); the lean editions — `kitecmd` (base only), `kitecloud` (base + Kubernetes), `kiteai` (base + LLM/MCP) — are subsets for space-conscious targets. Every command works in any edition that includes the modules it touches; edition-specific commands are flagged below.

Examples use `kite`; substitute `kitecmd`/`kitecloud`/`kiteai` for the lean editions.

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
| [`kite init`](init.md) | Scaffold a new starkite module |

### Maintenance

Available in **every** edition.

| Command | Purpose |
|---------|---------|
| [`kite version`](version.md) | Print version information |
| [`kite update`](update.md) | Update starkite to the latest version |
| [`kite module`](module.md) | Manage external modules |

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
| `--permissions name` | Permission profile name: a built-in (`deny-all`/`allow-fs`/`allow-net`/`allow-local`/`allow-all`/`allow-all-shell`) or a profile defined in `config.yaml`; unset resolves to the config `default` profile or `deny-all` | `""` |
| `--deny-all`, `--allow-fs`, `--allow-net`, `--allow-local`, `--allow-all`, `--allow-all-shell` | Boolean aliases for `--permissions=<profile>`. Set at most one; not combinable with `--permissions` | `false` |
| `--sandbox[=profile]` | OS-level sandbox profile (Linux only): a built-in rung (`opaque`/`net-access`/`host`) or a `config.yaml` profile. Bare `--sandbox` selects the config `default` profile, else `opaque`. See [Sandbox guide](../../fundamentals/security/sandbox.md). | `""` |
| `--sandbox-opaque`, `--sandbox-net-access`, `--sandbox-host` | Boolean aliases for `--sandbox=<rung>`. Set at most one sandbox flag | `false` |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `STARKITE_DEBUG` | Set to `1` or `true` to enable debug mode |
| `STARKITE_OUTPUT` | Default output format |
| `STARKITE_TIMEOUT` | Default timeout in seconds |
| `STARKITE_VAR_*` | Variable injection (e.g., `STARKITE_VAR_DB_HOST=localhost` → `var_str("db.host")`) |
| `STARKITE_SECURITY_SANDBOX` | Sandbox profile for shebang-launched scripts (same syntax as `--sandbox`). Linux only. |

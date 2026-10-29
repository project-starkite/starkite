---
title: "CLI Reference"
description: "Starkite command-line interface"
weight: 10
---

The default binary is `kite` (all-in-one); the lean editions — `kitecmd` (base only), `kitecloud` (base + Kubernetes), `kiteai` (base + MCP) — are subsets for space-conscious targets. Every command works in any edition that includes the modules it touches; edition-specific commands are flagged below.

Examples use `kite`; substitute `kitecmd`/`kitecloud`/`kiteai` for the lean editions.

## Commands

### Script execution

Available in **every** edition (base, cloud, ai, all).

| Command | Purpose |
|---------|---------|
| [`kite run`](run.md) | Execute a Starlark script or module directory |
| [`kite exec`](exec.md) | Launch an external native process or shell script in the sandbox |
| [`kite eval`](eval.md) | Evaluate inline Starlark expressions and snippets |
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
| `--sandbox-profile name` | Sandbox profile name: a built-in rung (`deny-all`/`sandbox-path`/`sandbox-exec`/`sandbox-net`/`unconfined`) or a `config.yaml` profile. Default is `deny-all`. | `""` |
| `--sandbox-path-read paths` | Grant read access to host paths (comma-separated or repeatable). | |
| `--sandbox-path-write paths` | Grant read/write access to host paths (comma-separated or repeatable). | |
| `--sandbox-exec-allow bins` | Allow execution of specific binaries, paths, or `*` (comma-separated or repeatable). | |
| `--sandbox-exec-deny bins` | Deny execution of specific binaries (comma-separated or repeatable). | |
| `--sandbox-call-allow rules` | Allow specific Starlark module calls (e.g. `fs.read`, `http.client`). | |
| `--sandbox-call-deny rules` | Deny specific Starlark module calls. | |
| `--sandbox-net-allow[=targets]` | Allow outbound network access to endpoint(s) or `*` for unrestricted host networking (repeatable, comma-separated). | `*` |
| `--sandbox-net-deny[=targets]` | Deny outbound network access to endpoint(s) or `*` for air-gapping (repeatable, comma-separated). | `*` |
| `--sandbox-net-mode mode` | Network confinement mode (`host`, `loopback`, `none`). | `""` |
| `--sandbox-max-memory limit` | Maximum memory limit for sandboxed process (e.g. `512MB`, `2GB`). | `""` |
| `--sandbox-driver driver` | Sandbox execution driver (`auto`/`landlock`/`seatbelt`/`windows`/`podman`/`docker`/`nerdctl`/`gvisor`). | `""` |
| `--sandbox-image image` | OCI image for container sandbox drivers (`podman`/`docker`/`nerdctl`). | `""` |
| `--sandbox-enabled=false` | Disable sandbox confinement and run unconfined. Default is `true`. | `true` |
| `--env, -e KEY[=VALUE]` | Forward host environment variable (`KEY`) or inject assignment (`KEY=VALUE`). | |
| `--env-inherit` | Inherit the full ambient host environment. | `false` |

## Environment Variables

| Variable | Description |
|----------|-------------|
| `STARKITE_DEBUG` | Set to `1` or `true` to enable debug mode |
| `STARKITE_OUTPUT` | Default output format |
| `STARKITE_TIMEOUT` | Default timeout in seconds |
| `STARKITE_VAR_*` | Variable injection (e.g., `STARKITE_VAR_DB_HOST=localhost` → `var_str("db.host")`) |
| `STARKITE_SANDBOX_PROFILE` | Sandbox profile for shebang-launched scripts (same syntax as `--sandbox-profile`). |
| `STARKITE_SANDBOX_DRIVER` | Sandbox execution driver override (`auto`, `landlock`, `seatbelt`, `podman`, `docker`, `nerdctl`, `gvisor`). |

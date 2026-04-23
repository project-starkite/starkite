---
title: "kite module"
description: "Manage external starkite modules"
weight: 24
---

Manage external starkite modules. Modules extend starkite with additional functionality and come in two types:

- **starlark** — script modules installed from git repositories
- **wasm** — WebAssembly modules installed from local paths or git

Installed modules live under `~/.starkite/modules/` and are discovered automatically at runtime.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `kite module install <source>` | Install from a git repo or local path |
| `kite module list` | List installed modules |
| `kite module update <name>` | Pull the latest version of an installed starlark module |
| `kite module remove <name>` | Delete an installed module. Aliases: `rm`, `uninstall` |
| `kite module info <name>` | Show detailed info about an installed module |

## `kite module install <source>`

### Source formats (starlark)

| Source | Meaning |
|--------|---------|
| `github.com/user/repo` | HTTPS clone from GitHub |
| `gitlab.com/user/repo` | HTTPS clone from GitLab |
| `bitbucket.org/user/repo` | HTTPS clone from Bitbucket |
| `user/repo` | Short form for `github.com/user/repo` |
| `github.com/user/repo@v1.0.0` | Specific tag |
| `github.com/user/repo@main` | Specific branch |
| `github.com/user/repo@abc1234` | Specific commit |
| `git@github.com:user/repo.git` | SSH clone |

### Source formats (WASM)

Local directory containing a `module.yaml` + `.wasm` file, a `.wasm` file directly, or a git repository containing the same. Use `--type wasm` to force WASM detection.

### Flags

| Flag | Description |
|------|-------------|
| `--as <name>` | Install with a custom local name (overrides the repo-derived default) |
| `--force` | Overwrite an existing installation |
| `--type` | Module type: `starlark` or `wasm`. Auto-detected when omitted |

### Examples

```bash
# Install a starlark module from GitHub
kite module install github.com/user/kite-helm

# Short form with custom name
kite module install user/helm-module --as helm

# Pin to a version
kite module install github.com/user/kite-helm@v1.0.0

# Reinstall, overwriting the existing copy
kite module install --force github.com/user/kite-helm

# Install a WASM module from a local directory
kite module install --type wasm ./path/to/echo

# Install a WASM plugin from a git repo
kite module install --type wasm github.com/user/wasm-plugin
```

Auto-detection picks WASM when the source ends in `.wasm` or a local directory's `module.yaml` contains a `wasm:` field.

## `kite module list`

Lists installed modules with name, type, version, and source:

```bash
kite module list
# NAME   TYPE      VERSION   SOURCE
# ----   ----      -------   ------
# helm   starlark  v1.0.0    github.com/user/kite-helm
# echo   wasm      -         (local)
```

## `kite module update <name>`

Updates an installed **starlark** module by pulling the latest from its git repository. WASM modules cannot be updated in place — reinstall with `--force` instead.

```bash
kite module update helm
```

## `kite module remove <name>`

Removes an installed module and its files.

```bash
kite module remove helm
kite module rm echo
```

## `kite module info <name>`

Shows detailed info: name, type, path, version, repository, entry point. For WASM modules, also shows the `.wasm` file, exported functions, and declared permissions.

```bash
kite module info helm
# Name:        helm
# Type:        starlark
# Path:        /Users/you/.starkite/modules/helm
# Version:     v1.0.0
# Repository:  github.com/user/kite-helm
# Description: Helm chart operations for starkite
```

## Related

- [WASM plugins guide](../guides/wasm-plugins.md) — how to author WASM modules

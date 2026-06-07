---
title: "kite module"
description: "Manage external starkite modules"
weight: 24
---

Manage external starkite modules — Starlark script modules installed from a git repository or a local directory.

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

### Source formats

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
| `./path/to/module` | Local directory (copied, not cloned) |

### Flags

| Flag | Description |
|------|-------------|
| `--as <name>` | Install with a custom local name (overrides the repo-derived default) |
| `--force` | Overwrite an existing installation |

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

# Install from a local directory
kite module install ./path/to/my-module
```

## `kite module list`

Lists installed modules with name, type, version, and source:

```bash
kite module list
# NAME   TYPE      VERSION   SOURCE
# ----   ----      -------   ------
# helm   starlark  v1.0.0    github.com/user/kite-helm
```

## `kite module update <name>`

Updates an installed module by pulling the latest from its git repository.

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

Shows detailed info: name, type, path, version, repository, entry point.

```bash
kite module info helm
# Name:        helm
# Type:        starlark
# Path:        /home/alice/.starkite/modules/helm
# Version:     v1.0.0
# Repository:  github.com/user/kite-helm
# Description: Helm chart operations for starkite
```

## Related

- [Modules concept](../../fundamentals/modules.md) — auto-loading and `load()`

---
title: "kite update"
description: "Update starkite to the latest version"
weight: 21
---

Check for and install the latest version of starkite. Downloads the latest GitHub release and replaces the current binary. Any installed edition binaries (kite-cloud, kite-ai) are also updated to the same version.

## Usage

```bash
kite update [--check | --force]
```

## Flags

| Flag | Description |
|------|-------------|
| `--check` | Check for updates without installing. Prints the available version and exits |
| `--force` | Force update even if the current version matches or the current binary is a dev build |

## Behavior

- Refuses to update a dev build unless `--force` is passed.
- Reports current and latest versions.
- Updates the base `kite` binary first, then every installed edition binary in turn.
- On partial failure (e.g., one edition fails to update), continues with the rest and reports warnings.

## Examples

### Update to latest

```bash
kite update
```

### Check without installing

```bash
kite update --check
# Current version: 0.1.0-dev
# Latest version:  0.2.0
# Update available: 0.1.0-dev → 0.2.0
# Run 'kite update' to install.
```

### Force reinstall

```bash
kite update --force
```

## Exit codes

- `0` — update succeeded, or already up-to-date
- non-zero — update check or install failed

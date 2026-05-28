---
title: "kite edition"
description: "Manage starkite editions"
weight: 23
---

Manage installed starkite editions. Editions add feature sets on top of the base `kitecmd` binary — `cloud` adds Kubernetes support, `ai` adds LLM/MCP modules.

The `edition` subcommand adds Kubernetes or LLM functionality to a `kitecmd` install without reinstalling — it fetches the matching edition binary on demand and lets `kitecmd` delegate to it transparently. Not needed for the all-in-one `kite` binary, which already bundles every edition's modules.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `kite edition use <name>` | Switch active edition. Downloads the binary if not installed. |
| `kite edition remove <name>` | Remove an installed edition. Aliases: `rm`, `uninstall`. |
| `kite edition status` | Show the active edition and list installed editions. |

## `kite edition use <name>`

Switch the active edition. If the edition binary isn't installed, it is downloaded from GitHub Releases automatically.

```bash
kite edition use cloud
```

### Flags

| Flag | Description |
|------|-------------|
| `--from <path>` | Install from a local binary path instead of downloading |
| `--force` | Overwrite an existing installation |

### Examples

```bash
# Switch to cloud edition (downloads if not installed)
kite edition use cloud

# Install cloud from a locally built binary
kite edition use cloud --from ./bin/kitecloud

# Replace an existing ai edition install with a local build
kite edition use ai --from ./bin/kiteai --force

# Switch back to the base edition
kite edition use base
```

## `kite edition remove <name>`

Remove an installed edition. The base edition cannot be removed.

```bash
kite edition remove cloud
kite edition rm ai
```

## `kite edition status`

Show the current active edition, the base binary's version, and every installed edition with its on-disk size.

```bash
kite edition status
# Current edition: ai
# Version:         0.1.0-dev
# Binary edition:  base
#
# Installed editions:
#   * ai    (56 MB)
#     cloud (63 MB)
```

The `*` marker indicates the active edition.

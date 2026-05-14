---
title: "kite edition"
description: "Manage starkite editions"
weight: 23
---

Manage installed starkite editions. Editions add feature sets on top of the base `kitecmd` binary — `cloud` adds Kubernetes support, `ai` adds LLM/MCP modules.

If you installed `kitecmd` (the lean base edition) and later want Kubernetes or LLM functionality without reinstalling, the `edition` subcommand fetches the corresponding edition binary and lets `kitecmd` delegate to it transparently. Users who installed the all-in-one `kite` binary don't need this — it already contains every edition's modules.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `kitecmd edition use <name>` | Switch active edition. Downloads the binary if not installed. |
| `kitecmd edition remove <name>` | Remove an installed edition. Aliases: `rm`, `uninstall`. |
| `kitecmd edition status` | Show the active edition and list installed editions. |

## `kitecmd edition use <name>`

Switch the active edition. If the edition binary isn't installed, it is downloaded from GitHub Releases automatically.

```bash
kitecmd edition use cloud
```

### Flags

| Flag | Description |
|------|-------------|
| `--from <path>` | Install from a local binary path instead of downloading |
| `--force` | Overwrite an existing installation |

### Examples

```bash
# Switch to cloud edition (downloads if not installed)
kitecmd edition use cloud

# Install cloud from a locally built binary
kitecmd edition use cloud --from ./bin/kitecloud

# Replace an existing ai edition install with a local build
kitecmd edition use ai --from ./bin/kiteai --force

# Switch back to the base edition
kitecmd edition use base
```

## `kitecmd edition remove <name>`

Remove an installed edition. The base edition cannot be removed.

```bash
kitecmd edition remove cloud
kitecmd edition rm ai
```

## `kitecmd edition status`

Show the current active edition, the base binary's version, and every installed edition with its on-disk size.

```bash
kitecmd edition status
# Current edition: ai
# Version:         0.1.0-dev
# Binary edition:  base
#
# Installed editions:
#   * ai    (56 MB)
#     cloud (63 MB)
```

The `*` marker indicates the active edition.

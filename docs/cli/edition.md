---
title: "kite edition"
description: "Manage starkite editions"
weight: 23
---

Manage installed starkite editions. Editions add feature sets on top of the base `basekite` binary — `cloud` adds Kubernetes support, `ai` adds LLM/MCP modules.

If you installed `basekite` (the lean base edition) and later want Kubernetes or LLM functionality without reinstalling, the `edition` subcommand fetches the corresponding edition binary and lets `basekite` delegate to it transparently. Users who installed the all-in-one `kite` binary don't need this — it already contains every edition's modules.

## Subcommands

| Subcommand | Purpose |
|------------|---------|
| `basekite edition use <name>` | Switch active edition. Downloads the binary if not installed. |
| `basekite edition remove <name>` | Remove an installed edition. Aliases: `rm`, `uninstall`. |
| `basekite edition status` | Show the active edition and list installed editions. |

## `basekite edition use <name>`

Switch the active edition. If the edition binary isn't installed, it is downloaded from GitHub Releases automatically.

```bash
basekite edition use cloud
```

### Flags

| Flag | Description |
|------|-------------|
| `--from <path>` | Install from a local binary path instead of downloading |
| `--force` | Overwrite an existing installation |

### Examples

```bash
# Switch to cloud edition (downloads if not installed)
basekite edition use cloud

# Install cloud from a locally built binary
basekite edition use cloud --from ./bin/cloudkite

# Replace an existing ai edition install with a local build
basekite edition use ai --from ./bin/aikite --force

# Switch back to the base edition
basekite edition use base
```

## `basekite edition remove <name>`

Remove an installed edition. The base edition cannot be removed.

```bash
basekite edition remove cloud
basekite edition rm ai
```

## `basekite edition status`

Show the current active edition, the base binary's version, and every installed edition with its on-disk size.

```bash
basekite edition status
# Current edition: ai
# Version:         0.1.0-dev
# Binary edition:  base
#
# Installed editions:
#   * ai    (56 MB)
#     cloud (63 MB)
```

The `*` marker indicates the active edition.

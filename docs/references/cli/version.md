---
title: "kite version"
description: "Print version information"
weight: 20
---

Print detailed version information including edition, commit hash, build time, and Go runtime.

## Usage

```bash
kite version [--short | --json]
```

## Flags

| Flag | Description |
|------|-------------|
| `--short` | Print the version number only (no edition, commit, or runtime info) |
| `--json` | Print version info as JSON for machine consumption |

## Examples

### Human-readable (default)

```bash
kite version
# kite version 0.1.0-dev (core)
#   edition: core
#   commit:  abc1234
#   built:   2026-04-23T10:15:30Z
#   go:      go1.26
#   os/arch: darwin/arm64
```

### Short form (just the version)

```bash
kite version --short
# 0.1.0-dev
```

### JSON

```bash
kite version --json
# {
#   "version": "0.1.0-dev",
#   "edition": "core",
#   "commit":  "abc1234",
#   ...
# }
```

### Scripting

```bash
# Check minimum version in CI
if [ "$(kite version --short)" = "0.1.0-dev" ]; then
    echo "dev build detected"
fi
```

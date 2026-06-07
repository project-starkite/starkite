---
title: "kite init"
description: "Scaffold a new starkite module"
weight: 5
---

Scaffold a new starkite module: a directory with `main.star`, `mod.yaml`,
`mod.lock`, and `README.md`.

## Usage

```bash
kite init [directory] [flags]
```

The module's name defaults to the target directory's name. Override it with
`--name`, which accepts `name` or `namespace/name`. A `--template` adds an example
`main.star` and any supporting files on top of the base scaffold.

## Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--name identity` | Module identity: `name` or `namespace/name` | directory name |
| `-t, --template name` | Template overlay to apply | `basic` |
| `--list-templates` | List available templates | |

## Templates

| Template | Description |
|----------|-------------|
| `basic` | Minimal runnable module |
| `kubernetes` | Kubernetes manifest generation |

## Scaffolded files

| File | Purpose |
|------|---------|
| `main.star` | Entry point; defines `main()` |
| `mod.yaml` | Module identity and declared dependencies |
| `mod.lock` | Resolved dependency lockfile (generated; commit it) |
| `README.md` | Module documentation |

## Examples

```bash
kite init                                  # current directory, basic template
kite init ./my-module                      # specific directory
kite init ./my-module --name acme/widget   # explicit identity
kite init --template=kubernetes            # kubernetes template
```

## Related

- [Modules concept](../../fundamentals/modules.md) — module layout, `load()`, dependencies
- [`kite module`](module.md) — install, verify, and manage modules

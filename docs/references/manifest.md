---
title: "Module manifest (mod.yaml)"
description: "Schema for the mod.yaml module manifest"
weight: 35
---

# Module manifest — `mod.yaml`

Every module is a directory containing a `mod.yaml` manifest and a `main.star` entry file. `mod.yaml` is the single source of truth for the module's identity and declared dependencies, independent of where the module lives.

```yaml
namespace: acme
name: app
version: 0.1.0
description: Deployment automation for the acme stack
dependencies:
  acme/slack: gitlab.com/acme/slack@v1.2.0
  acme/leaf: ../leaf
```

## Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | **yes** | The module's name. With `namespace`, forms the `namespace/name` identity used by `load()` and `kite run @namespace/name`. |
| `namespace` | string | no | The identity's namespace. May be omitted when it can be resolved from the install source (a git org) or supplied at install time with `--as <namespace>/<name>`. |
| `version` | string | no | Human-facing version of the module (e.g. `0.1.0`). Informational; not used for resolution. |
| `description` | string | no | One-line summary, shown by `kite module info` and `kite module list`. |
| `dependencies` | map | no | The modules this module `load()`s, mapping each `namespace/name` identity to its source. |

## `dependencies`

Each entry maps a dependency's `namespace/name` identity to a **source** — where to fetch it from — optionally suffixed with `@version` (a git tag, branch, or commit):

```yaml
dependencies:
  acme/slack: gitlab.com/acme/slack@v1.2.0   # git host, pinned tag
  acme/db:    git@github.com:acme/db.git      # SSH
  acme/leaf:  ../leaf                          # local directory
```

The declared **identity** (the key) must match the identity the dependency declares in its own `mod.yaml`; a mismatch is an error at resolution. Source formats are the same as [`kite module install`](cli/module.md#kite-module-install-source).

Running the module resolves this set — and the dependencies' transitive dependencies — into the global cache and records the resolved revisions and content hashes in a generated `mod.lock` beside `mod.yaml`. The manifest declares *what* is needed; `mod.lock` records *exactly* what was resolved. See [Modules → Declaring dependencies](../fundamentals/modules.md#declaring-dependencies).

## Not in the manifest

`mod.yaml` declares no permissions and no minimum runtime version. A loaded module runs under the same runtime permission as the entry script — importing it grants no authority of its own (see [Permission](../fundamentals/security/permission.md)). Scripts that require an unsupported runtime fail fast at execution.

## Related

- [Modules concept](../fundamentals/modules.md) — module layout, `load()`, dependencies, `mod.lock`
- [`kite module`](cli/module.md) — install, list, info, verify, update, remove
- [`kite init`](cli/init.md) — scaffold a module with a starter `mod.yaml`

---
title: "Module manifest (mod.yaml)"
description: "Schema for the mod.yaml module manifest"
weight: 35
---

# Module manifest — `mod.yaml`

Every module is a directory containing a `mod.yaml` manifest and a `main.star` entry file. `mod.yaml` is the single source of truth for the module's identity, declared dependencies, and required runtime capabilities, independent of where the module lives.

```yaml
namespace: acme
name: deployer
version: 0.1.0
description: Deployment automation for the acme stack
dependencies:
  acme/slack: gitlab.com/acme/slack@v1.2.0
  acme/leaf: ../leaf
sandbox:
  network: host
  paths:
    read: ["."]
  exec:
    allow: ["kubectl", "helm"]
  env:
    KUBECONFIG:
```

## Fields

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | string | **yes** | The module's name. With `namespace`, forms the `namespace/name` identity used by `load()` and `kite run namespace/name`. |
| `namespace` | string | no | The identity's namespace. May be omitted when it can be resolved from the install source (a git org) or supplied at install time with `--as <namespace>/<name>`. |
| `version` | string | no | Human-facing version of the module (e.g. `0.1.0`). Informational; not used for resolution. |
| `description` | string | no | One-line summary, shown by `kite module info` and `kite module list`. |
| `dependencies` | map | no | The modules this module `load()`s, mapping each `namespace/name` identity to its source. |
| `sandbox` | map | no | Minimum capability requirements the module needs to function. |

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

## `sandbox` and the Operator Trust Boundary

The optional `sandbox:` section declares the capabilities the module requires:

```yaml
sandbox:
  network: host
  paths:
    read: ["."]
    write: ["./dist"]
  exec:
    allow: ["kubectl", "helm"]
  calls:
    allow: ["k8s.write"]
  env:
    KUBECONFIG:
```

### The Non-Escalation Principle

A module's `sandbox:` section declares the minimum capabilities the module *requires* to operate; it is **never a self-grant**.

Before executing any code, Starkite evaluates all loaded module manifests against the operator's active sandbox profile and CLI flags. If any module declares requirements that exceed what the operator granted, Starkite halts execution immediately with an actionable hint:

```text
Error: module "acme/deployer" requires capabilities exceeding active profile "default":
  - network: host (active: none)
  - exec.allow: ["kubectl", "helm"] (active: none)
  Hint: rerun with required capabilities: kite run --sandbox-net --sandbox-exec-allow=kubectl,helm ./app.star
```

This ensures that third-party or shared modules cannot covertly elevate their privileges beyond the boundaries established by the operator.

## Related

- [Modules concept](../fundamentals/modules.md) — module layout, `load()`, dependencies, `mod.lock`
- [Sandbox guide](../fundamentals/security/sandbox.md) — zero-trust isolation and capability profiles
- [Permission guide](../fundamentals/security/permission.md) — language-level capability enforcement
- [`kite module`](cli/module.md) — install, list, info, verify, update, remove
- [`kite init`](cli/init.md) — scaffold a module with a starter `mod.yaml`

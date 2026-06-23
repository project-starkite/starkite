---
title: "Modules"
description: "Understanding the Starkite module system"
weight: 50
---

# Modules

Starkite uses a module system to organize and distribute automation code. Built-in services (such as filesystems, HTTP, SSH, Kubernetes, and AI tools), third-party libraries, and local helper scripts are all packaged as modules. The module system automates dependency resolution, fetches remote packages, and pins version hashes to guarantee reproducible runs.

For a full catalog of standard modules, see the [API Reference](../references/api/index.md).

## Auto-loaded modules

Standard library modules are automatically loaded into the global scope. You can call their functions directly without writing `load()` statements:

```python
content = read_text("./config.yaml")
data = yaml.decode(content)
print(os.hostname())
```

The modules available by default depend on the Starkite edition:

| Edition | Binary | Included Modules |
|---|---|---|
| Base | `kitecmd` | `os`, `fs`, `http`, `ssh`, `sql`, `json`, `yaml`, `csv`, `gzip`, `zip`, `base64`, `hash`, `strings`, `regexp`, `template`, `time`, `uuid`, `fmt`, `log`, `table`, `concur`, `retry`, `io`, `vars`, `runtime`, `inventory`, `test` |
| Cloud | `kitecloud` | Base modules + `k8s` |
| AI | `kiteai` | Base modules + `ai`, `mcp` |
| All-in-one | `kite` | All modules (Base + Cloud + AI) |

For differences between these binaries, see the [Editions Guide](editions.md).

## Custom modules

A custom module is a directory containing a `mod.yaml` manifest and one or more Starlark (`.star`) source files. The manifest declares the module's namespace, name, and version:

```yaml
# helpers/mod.yaml
namespace: acme
name: helpers
version: 0.1.0
```

```python
# helpers/main.star
def deploy(env): ...
def rollback(env): ...
def _private(): ...     # Private helper (not exported)
```

*   **Entry Point**: Every custom module requires a `main.star` file as its entry point.
*   **Symbol Export**: All top-level symbols defined in the module are public and exported, unless they begin with an underscore (e.g., `_private()`).

For manifest fields, see the [`mod.yaml` Reference](../references/manifest.md).

## Loading modules

Use the built-in `load()` function to import custom or installed modules:

*   **Local modules**: Paths starting with `./`, `../`, or `/` resolve directly from the local filesystem.
*   **Installed modules**: Bare `namespace/name` identifiers resolve from the global cache.

```python
# Load from local filesystem
load("./helpers", "helpers")
load("./lib/util.star", "util")

# Load from global cache (requires installation)
load("acme/slack", "slack")

def main():
    helpers.deploy("production")
    slack.post("#deploys", "shipped")
```

Starlark supports import aliasing: `load("./helpers", h = "helpers")`. Import paths do not specify versions; instead, the lockfile (`mod.lock`) pins versions and revisions automatically.

## Managing the module cache

The global cache at `~/.starkite/modules/` stores modules in version-addressed paths: `<namespace>/<name>@<rev>/`.
*   For Git-sourced modules, `<rev>` is the resolved Git commit SHA.
*   For local directories, `<rev>` is a content hash.

The cache is write-once; multiple revisions of the same module coexist to prevent version conflicts between projects.

| Command | Purpose |
|---------|---------|
| `kite module install <source>` | Install a module from Git or a local path |
| `kite module list` | List installed modules and revisions |
| `kite module info <name>` | Show installation details for a module |
| `kite module verify [name]` | Verify module integrity against recorded content hashes |
| `kite module update <name>` | Fetch the latest revision from the source |
| `kite module remove <name>` | Uninstall a module and its revisions |

For syntax and flags, see the [`kite module` CLI Reference](../references/cli/module.md).

## Declaring dependencies

Starkite uses manifest-based dependency resolution. Define dependencies in the `dependencies` block of your module's `mod.yaml`:

```yaml
# mod.yaml
namespace: acme
name: app
version: 0.1.0
dependencies:
  acme/slack: gitlab.com/acme/slack@v1.2.0
  acme/leaf: ../leaf            # Local filesystem path
```

When you execute `kite run`, the runtime resolves and downloads dependencies, generating a `mod.lock` lockfile:

```yaml
# mod.lock (generated; commit to version control)
version: 1
modules:
  acme/slack:
    source: gitlab.com/acme/slack@v1.2.0
    rev: 9f3c1ab
    hash: sha256:4a7e3...
  acme/leaf:
    source: ../leaf
    rev: 5a7c2ff8977db0d3
    hash: sha256:d82e1...
```

On subsequent runs, the runtime enforces lockfile constraints:
*   **Cache-First**: Locked dependencies load from the local cache without network calls.
*   **Integrity**: Execution fails if a cached module's hash differs from `mod.lock`.
*   **Loose Scripts**: Running a script file directly (`kite run ./script.star`) resolves dependencies from the cache only, writing a script-local `mod.lock`. Remote modules must be pre-installed using `kite module install`.

## Module permissions

Imported modules run within the security context of the parent script. Loading a module does not expand permissions. If a module attempts an action (such as filesystem or network access) that exceeds the parent script's permissions, the runtime raises a permission error and blocks execution.

For detailed security boundaries and sandboxing, see the [Permission Guide](security/permission.md#loaded-modules).

---
title: "Modules"
description: "How starkite organizes built-in capabilities — auto-loading and load()"
weight: 50
---

# Modules

Every capability in a starkite script — filesystem, HTTP, SSH, Kubernetes, LLMs — is exposed as a **module**. Built-in modules are auto-loaded into every script. The `load()` function can be used to pull in installed modules or other modules from the same project.

For available modules, see [References > API](../references/api/index.md).

## Auto-loaded modules

Standard `starkite` modules do not require `load()` statements. Every built-in module is injected into the global scope before the script runs:

```python
# No import needed
content = read_text("config.yaml")
data = yaml.decode(content)
print(os.hostname())
```

Which modules are auto-loaded depends on the edition:

| Edition | Binary | Modules |
|---|---|---|
| Base | `kitecmd` | os, fs, http, ssh, json, yaml, csv, gzip, zip, base64, hash, strings, regexp, template, time, uuid, fmt, log, table, concur, retry, io, vars, runtime, inventory, test |
| Cloud | `kitecloud` | base + `k8s` |
| AI | `kiteai` | base + `ai` + `mcp` |
| All-in-one | `kite` | base + cloud + ai |

See [Editions](editions.md) for the full edition model.

## What a module is

A module is a **directory** containing a `mod.yaml` manifest and one or more `.star` files. 
The manifest declares the module's identity and configuration:

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
def _private(): ...     # not exported
```

The directory's public symbols (everything not starting with `_`) merge into one module. The entry file is always `main.star`. For the full manifest schema, see the [`mod.yaml` reference](../references/manifest.md).

## Loading script modules

`load()` imports a module and binds its public symbols under a single name, accessed as `name.function()`. The reference form determines where the module comes from — a filesystem reference requires a path prefix (`./`, `../`, or `/`); a bare `namespace/name` is an installed-module identity:

```python
# a local module directory or file (path prefix required)
load("./helpers", "helpers")
load("./lib/util.star", "util")

def main():
    helpers.deploy("production")
```

Rename the binding with Starlark's aliasing syntax: `load("./helpers", h = "helpers")`.

An **installed** module is loaded by its bare `namespace/name` identity, resolved from the global cache (and pinned by `mod.lock` when one governs the run):

```python
load("acme/slack", "slack")     # installed via: kite module install ...

def main():
    slack.post("#deploys", "shipped")
```

`load()` never takes a revision — committed code is pinned by `mod.lock`, not by inline revisions. Revision selection (`namespace/name@rev`) is available on the `kite run` and `kite module` command lines.

## Managing modules

The `kite module` subcommands manage the global module cache at `~/.starkite/modules/`. Installed modules are stored version-addressed as `<namespace>/<name>@<rev>/` — `<rev>` is the commit SHA for a git source or a content hash for a local one — and the cache is write-once, so different revisions of the same module coexist.

| Command | Purpose |
|---------|---------|
| `kite module install <source>` | Fetch a module from a git host or local directory into the cache |
| `kite module list` | List installed modules, one row per revision |
| `kite module info <name>` | Show details for a module (every installed revision) |
| `kite module verify [name]` | Re-hash modules and check them against their recorded hash |
| `kite module update <name>` | Fetch the latest revision and add it to the cache |
| `kite module remove <name>` | Delete a module and all of its revisions |

### install

Fetch a module into the cache, where it then loads by its `namespace/name`:

```bash
kite module install gitlab.com/acme/slack        # → load("acme/slack", "slack")
kite module install gitlab.com/acme/slack@v1.2.0 # pin a tag, branch, or commit
kite module install ./my-module --as acme/tools  # local directory, custom identity
```

A source is host-agnostic — any git host works, not just well-known ones:

| Source form | Example |
|-------------|---------|
| `host/org/repo` | `gitlab.com/acme/slack` (cloned over HTTPS) |
| `host/org/repo@version` | `git.internal/acme/slack@v1.2.0` (tag, branch, or commit) |
| SSH | `git@github.com:acme/slack.git` |
| Full URL | `https://github.com/acme/slack` · `file:///path/to/repo` |
| Local directory | `./my-module` (copied, not cloned) |

For a git source the installed revision is the resolved commit SHA and the working `.git` directory is dropped; for a local directory it is a content hash. A module's identity comes from its `mod.yaml`; for a git source the org supplies a fallback namespace, and `--as <namespace>/<name>` can set it for a local directory. Reinstalling identical content is a no-op.

Install validates the module: the source must contain a `mod.yaml` with a `name` and a `main.star` entry file, or the install is rejected.

Run an installed module directly with `kite run acme/slack`. When several revisions are installed, the bare reference runs the newest; pin a specific one with `kite run acme/slack@<rev>`.

### list

```bash
kite module list
# NAME        REV               TYPE      VERSION  SOURCE
# acme/slack  9f3c1ab           starlark  v1.2.0   gitlab.com/acme/slack
```

Each installed revision is its own row, distinguished by the `REV` column.

### info

```bash
kite module info acme/slack
```

Shows name, revision, path, version, source, description, and entry point. When more than one revision is installed, all are listed.

### verify

Re-hash installed modules and compare against the hash recorded at install, detecting on-disk tampering or corruption. With no argument every module is checked; with a `namespace/name`, every revision of that module. Exits non-zero on any failure:

```bash
kite module verify              # check everything
kite module verify acme/slack   # check one module
# ok    acme/slack@9f3c1ab
```

This is the full-content check. At run time, `kite run` verifies locked dependencies the fast way — a stat-only fingerprint comparison — falling back to a full re-hash only when the fingerprint no longer matches.

### update

Fetch the latest revision from the module's recorded source and add it to the cache. Cached revisions are immutable, so an update installs a new revision alongside any existing one rather than overwriting:

```bash
kite module update acme/slack
# Updated acme/slack (a1b2c3d)
```

### remove

Delete a module and all of its cached revisions:

```bash
kite module remove acme/slack
```

See [`kite module`](../references/cli/module.md) for the full flag reference.

## Declaring dependencies

Dependency resolution is automatic: `kite run` fetches a module's dependencies into the global cache, resolves them transitively, and pins them in `mod.lock` — there is no separate install step. The one thing the resolver cannot infer is **where each dependency comes from**: `load("acme/slack")` names an identity, not a fetch location. The `dependencies` map in `mod.yaml` supplies that mapping — each `namespace/name` identity to its source (a git reference or a local path, optionally `source@version`):

```yaml
# app/mod.yaml
namespace: acme
name: app
version: 0.1.0
dependencies:
  acme/slack: gitlab.com/acme/slack@v1.2.0
  acme/leaf: ../leaf            # a local directory
```

Everything after the declaration is automatic. Running the module fetches each declared dependency — and, transitively, the dependencies it declares — into the global cache, and records the result in a generated `mod.lock` beside `mod.yaml`. The lockfile pins every resolved module to a source, an immutable revision, and a content hash:

```yaml
# app/mod.lock (generated; commit it)
version: 1
modules:
  acme/slack:
    source: gitlab.com/acme/slack@v1.2.0
    rev: 9f3c1ab
    hash: sha256:...
  acme/leaf:
    source: ../leaf
    rev: 5a7c2ff8977db0d3
    hash: sha256:...
```

`mod.lock` is committed and reviewable: a changed dependency produces a diff. On later runs, resolution is cache-first and incremental — a locked dependency whose cached tree still verifies is reused without re-fetching, and a hash mismatch is an error. The cache is version-addressed (`<namespace>/<name>@<rev>/`) and write-once, so projects pinning different revisions coexist.

A `load()` of an **undeclared** module still resolves when that module happens to be installed in the cache — but it is not fetched for you, not recorded in `mod.lock`, and not pinned to a revision. Declare every module the code loads; the declaration is what makes the run reproducible.

A **loose script** run directly (`kite run ./deploy.star`, not a module directory) has no `mod.yaml` to declare dependencies. The installed modules it `load()`s are resolved from the cache only — nothing is fetched — and recorded in a `mod.lock` written beside the script. An installed reference that is not already installed is an error; install it first with `kite module install`.

## Module permissions

A loaded module runs under the same runtime permission as the entry script — it declares no capabilities of its own, and importing it grants no authority. A dependency that needs more than the run was granted fails at the gated call, naming the module; the run is then restarted at a higher profile. See [Permission](security/permission.md#loaded-modules).

---
title: "Modules"
description: "Understanding the Starkite module system"
weight: 50
---

# Modules

A module is how Starkite packages capability. Every facility a script reaches for — filesystem, HTTP, SSH, Kubernetes, LLMs — arrives as a module, and so does any reusable code you write yourself. The built-in modules are present in every script without ceremony; your own modules and third-party ones come in through `load()`. The module system exists so that a script's dependencies are named, fetched, pinned, and reproduced the same way every time it runs, rather than copied around by hand.

For available modules, see [References > API](../references/api/index.md).

## Auto-loaded modules

Start with the modules you never have to ask for. The standard `starkite` modules require no `load()` statement; every built-in is injected into the global scope before the script runs, so you call them as though they were language built-ins:

```python
# No import needed
content = read_text("config.yaml")
data = yaml.decode(content)
print(os.hostname())
```

Exactly which modules arrive this way depends on the edition you run, since each edition trades binary size against the capabilities it bundles:

| Edition | Binary | Modules |
|---|---|---|
| Base | `kitecmd` | os, fs, http, ssh, json, yaml, csv, gzip, zip, base64, hash, strings, regexp, template, time, uuid, fmt, log, table, concur, retry, io, vars, runtime, inventory, test |
| Cloud | `kitecloud` | base + `k8s` |
| AI | `kiteai` | base + `ai` + `mcp` |
| All-in-one | `kite` | base + cloud + ai |

See [Editions](editions.md) for the full edition model.

## What a module is

Beyond the built-ins, a module is something you can author. It is a **directory** containing a `mod.yaml` manifest and one or more `.star` files. The manifest declares the module's identity and configuration, which is what lets the rest of the system address it by name:

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

The directory's public symbols — everything not starting with `_` — merge into one module, and the entry file is always `main.star`, so a private helper like `_private()` stays inside the module and never leaks to a caller. For the full manifest schema, see the [`mod.yaml` reference](../references/manifest.md).

## Loading script modules

Once a module exists, you bring it into a script with `load()`. The call imports the module and binds its public symbols under a single name, which you then reach as `name.function()`. What you pass as the reference decides where Starkite looks for the code: the grammar is the Go model, where a filesystem reference must carry a path prefix (`./`, `../`, or `/`) and a bare `namespace/name` is an installed-module identity. A path prefix says "load this from disk":

```python
# a local module directory or file (path prefix required)
load("./helpers", "helpers")
load("./lib/util.star", "util")

def main():
    helpers.deploy("production")
```

That binds the loaded code under `helpers`, and the script calls into it by that name. If the name collides or you want something shorter, rename the binding with Starlark's aliasing syntax: `load("./helpers", h = "helpers")`.

Drop the path prefix and the reference means something different. An **installed** module is loaded by its bare `namespace/name` identity, resolved from the global cache rather than the working tree (and pinned by `mod.lock` when one governs the run):

```python
load("acme/slack", "slack")     # installed via: kite module install ...

def main():
    slack.post("#deploys", "shipped")
```

Notice what the identity form deliberately omits: a revision. `load()` never takes one, because committed code is pinned by `mod.lock`, not by a revision spelled inline — that keeps the source stable while the lockfile decides which bytes run. Revision selection (`namespace/name@rev`) belongs on the `kite run` and `kite module` command lines, where you are choosing a version interactively, not in source a teammate will read.

## Managing modules

Installed modules have to come from somewhere and live somewhere, and that is what the `kite module` subcommands handle. They manage the global module cache at `~/.starkite/modules/`, where installed modules are stored version-addressed as `<namespace>/<name>@<rev>/` — `<rev>` is the commit SHA for a git source or a content hash for a local one. The cache is write-once, so different revisions of the same module coexist instead of overwriting each other, and a pin in one project never disturbs another. The subcommands break down as follows:

| Command | Purpose |
|---------|---------|
| `kite module install <source>` | Fetch a module from a git host or local directory into the cache |
| `kite module list` | List installed modules, one row per revision |
| `kite module info <name>` | Show details for a module (every installed revision) |
| `kite module verify [name]` | Re-hash modules and check them against their recorded hash |
| `kite module update <name>` | Fetch the latest revision and add it to the cache |
| `kite module remove <name>` | Delete a module and all of its revisions |

### install

Everything begins with `install`. It fetches a module into the cache, after which the module loads by its `namespace/name`:

```bash
kite module install gitlab.com/acme/slack        # → load("acme/slack", "slack")
kite module install gitlab.com/acme/slack@v1.2.0 # pin a tag, branch, or commit
kite module install ./my-module --as acme/tools  # local directory, custom identity
```

The source you name can take several shapes, and none of them are tied to a particular forge — any git host works, not just the well-known ones:

| Source form | Example |
|-------------|---------|
| `host/org/repo` | `gitlab.com/acme/slack` (cloned over HTTPS) |
| `host/org/repo@version` | `git.internal/acme/slack@v1.2.0` (tag, branch, or commit) |
| SSH | `git@github.com:acme/slack.git` |
| Full URL | `https://github.com/acme/slack` · `file:///path/to/repo` |
| Local directory | `./my-module` (copied, not cloned) |

However the source is spelled, the cached revision is derived rather than asserted: for a git source it is the resolved commit SHA and the working `.git` directory is dropped; for a local directory it is a content hash. A module's identity comes from its `mod.yaml`; for a git source the org supplies a fallback namespace, and `--as <namespace>/<name>` sets it for a local directory. Reinstalling identical content is a no-op, so re-running install costs nothing when nothing changed.

Install does not take the source on faith. It validates the module first — the source must contain a `mod.yaml` with a `name` and a `main.star` entry file, or the install is rejected — so a broken module never reaches the cache.

Once a module is installed you can run it directly with `kite run acme/slack`. When several revisions are installed the bare reference runs the newest; pin a specific one with `kite run acme/slack@<rev>` when you need an exact build.

### list

To see what install has accumulated, `list` prints one row per cached revision:

```bash
kite module list
# NAME        REV               TYPE      VERSION  SOURCE
# acme/slack  9f3c1ab           starlark  v1.2.0   gitlab.com/acme/slack
```

Because each installed revision is its own row distinguished by the `REV` column, two builds of the same module show up as two lines rather than one.

### info

When the summary row is not enough, `info` expands a single module:

```bash
kite module info acme/slack
```

It shows name, revision, path, version, source, description, and entry point, and when more than one revision is installed it lists all of them.

### verify

Listing tells you what is cached; `verify` tells you whether it is intact. It re-hashes installed modules and compares against the hash recorded at install, catching on-disk tampering or corruption. With no argument every module is checked; with a `namespace/name`, every revision of that module. It exits non-zero on any failure, which makes it usable as a gate in a script:

```bash
kite module verify              # check everything
kite module verify acme/slack   # check one module
# ok    acme/slack@9f3c1ab
```

That is the full-content check, deliberately thorough. At run time `kite run` does not pay for it on every dependency — it verifies locked dependencies the fast way, a stat-only fingerprint comparison, and falls back to a full re-hash only when the fingerprint no longer matches.

### update

When a module's upstream moves on, `update` fetches the latest revision from its recorded source and adds it to the cache. Cached revisions are immutable, so an update installs a new revision alongside any existing one rather than overwriting — the older pin keeps working until you choose to move:

```bash
kite module update acme/slack
# Updated acme/slack (a1b2c3d)
```

### remove

To reclaim the space, `remove` deletes a module and every one of its cached revisions:

```bash
kite module remove acme/slack
```

See [`kite module`](../references/cli/module.md) for the full flag reference.

## Declaring dependencies

Managing modules by hand is fine for the ones you run directly, but a module that loads other modules should not push that work onto whoever runs it. So for dependencies the flow inverts: resolution is automatic. `kite run` fetches a module's dependencies into the global cache, resolves them transitively, and pins them in `mod.lock` — there is no separate install step. The one thing the resolver cannot infer is **where each dependency comes from**: `load("acme/slack")` names an identity, not a fetch location. The `dependencies` map in `mod.yaml` supplies that mapping — each `namespace/name` identity to its source (a git reference or a local path, optionally `source@version`):

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

Loading a module pulls in its code, but it does not pull in any authority — and that boundary matters for the same reason the lockfile does, because it keeps a run's behavior predictable. A loaded module runs under the same runtime permission as the entry script; it declares no capabilities of its own, so importing it grants nothing the script did not already have. A dependency that needs more than the run was granted fails at the gated call, naming the module, and you restart the run at a higher profile rather than the module quietly widening its reach. See [Permission](security/permission.md#loaded-modules).

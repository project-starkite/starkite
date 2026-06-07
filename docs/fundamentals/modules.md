---
title: "Modules"
description: "How starkite organizes built-in capabilities — auto-loading and load()"
weight: 50
---

# Modules

Every capability in a starkite script — filesystem, HTTP, SSH, Kubernetes, LLMs — is exposed as a **module**. Built-in modules are auto-loaded into every script; `load()` pulls in other `.star` files from the same project.

For the per-module API catalog, see [References > API](../references/api/index.md).

## Auto-loaded modules

Unlike standard Starlark, starkite modules do not require `load()` statements. Every built-in module is injected into the global scope before the script runs:

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

A module is a **directory** containing a `mod.yaml` manifest and one or more `.star` files. The manifest declares the module's identity and configuration:

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

The directory's public symbols (everything not starting with `_`) merge into one module. The entry file is always `main.star`.

## Loading script modules

`load()` imports a module and binds its public symbols under a single name, accessed as `name.function()`:

```python
# a local module directory (relative to the calling script)
load("./helpers", "helpers")

def main():
    helpers.deploy("production")
```

Rename the binding with Starlark's aliasing syntax: `load("./helpers", h = "helpers")`.

An **installed** module is loaded by its `namespace/name`:

```python
load("acme/slack", "slack")     # installed via: kite module install ...

def main():
    slack.post("#deploys", "shipped")
```

## Module permissions

A loaded module runs under the same runtime permission as the entry script — it declares no capabilities of its own, and importing it grants no authority. A dependency that needs more than the run was granted fails at the gated call, naming the module; the run is then restarted at a higher profile. See [Permission](security/permission.md#loaded-modules).

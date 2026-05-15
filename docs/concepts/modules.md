---
title: "Modules"
description: "How starkite organizes built-in capabilities — auto-loading and load()"
weight: 50
---

# Modules

Every capability in a starkite script — filesystem, HTTP, SSH, Kubernetes, LLMs — is exposed as a **module**. This page covers how built-in modules are auto-loaded into every script, and how `load()` pulls symbols from other `.star` files in the same project.

For the per-module API catalog, see [References > API](../references/api/index.md).

## Auto-Loaded Modules

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

## Loading Script Modules

`load()` imports symbols from other `.star` files in the same project. Paths are resolved relative to the calling script's directory:

```python
load("helpers.star", "deploy", "rollback")

deploy("production")
```

---
title: "Modules"
description: "How starkite organizes built-in capabilities"
weight: 40
---

Every capability in a starkite script — filesystem, HTTP, SSH, Kubernetes, LLMs — is exposed as a **module**. Modules ship as part of the runtime (auto-loaded, no import needed), are pulled in from other `.star` files via `load()`, or extend the runtime as WebAssembly plugins.

For the per-module API catalog, see [References > API](../references/api/index.md).

## Auto-Loaded Modules

Unlike standard Starlark, starkite modules do not require `load()` statements. Every built-in module is injected into the global scope before your script runs:

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

See [Editions](editions.md) for a deeper explanation of the edition model.

## Loading Script Modules

`load()` imports symbols from other `.star` files in your project. Paths are resolved relative to the calling script's directory:

```python
load("helpers.star", "deploy", "rollback")

deploy("production")
```

## WASM Plugins

You can extend starkite with WebAssembly modules. A plugin consists of a `module.yaml` manifest and a `.wasm` binary, installed under `~/.starkite/modules/wasm/`.

### Manifest

```yaml
# module.yaml
name: myplugin
version: 1.0.0
description: My custom plugin
wasm: myplugin.wasm
min_starkite: "0.0.1"
permissions:
  - log

functions:
  - name: greet
    params:
      - name: name
        type: string
    returns: string

  - name: add
    params:
      - name: a
        type: int
      - name: b
        type: int
    returns: int
```

### Directory Layout

```text
~/.starkite/modules/wasm/
  myplugin/
    module.yaml
    myplugin.wasm
```

### Calling Plugin Functions

Once installed, WASM plugins are auto-loaded alongside built-ins:

```python
result = myplugin.greet("Alice")   # "Hello, Alice!"
sum    = myplugin.add(2, 3)        # 5
```

### Supported Types

| Type | Starlark | WASM |
|------|----------|------|
| `string` | `str` | string |
| `int` | `int` | i64 |
| `float` | `float` | f64 |
| `bool` | `bool` | i32 (0/1) |
| `dict` | `dict` | JSON string |
| `list` | `list` | JSON string |

---
title: "WASM Plugins"
description: "Extending starkite with WebAssembly modules"
weight: 4
---

Starkite supports WebAssembly (WASM) plugins that extend the module system with custom functionality.

## Plugin Structure

A WASM plugin consists of:
- A `module.yaml` manifest
- A `.wasm` binary

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

## Plugin Directory

Plugins are discovered from `~/.starkite/modules/wasm/`. Each plugin lives in its own subdirectory:

```text
~/.starkite/modules/wasm/
  myplugin/
    module.yaml
    myplugin.wasm
```

## Using Plugins

Once installed, WASM plugins are loaded automatically and available as modules:

```python
result = myplugin.greet("Alice")
print(result)  # "Hello, Alice!"

sum = myplugin.add(2, 3)
print(sum)  # 5
```

## Supported Types

| Type | Starlark | WASM |
|------|----------|------|
| `string` | `str` | string |
| `int` | `int` | i64 |
| `float` | `float` | f64 |
| `bool` | `bool` | i32 (0/1) |
| `dict` | `dict` | JSON string |
| `list` | `list` | JSON string |

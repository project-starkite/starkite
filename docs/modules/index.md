---
title: "Module Reference"
description: "Built-in modules available in starkite scripts"
weight: 1
---

Starkite exposes Go's standard library as type-safe, scriptable Starlark modules for general-purpose automation, cloud-native operations, data processing, and GenAI agent workflows. All modules are **auto-loaded** and available in every `.star` script without any import statement.

## Available Modules

| Module | Description |
|--------|-------------|
| [`os`](os.md) | Environment, process info, and command execution |
| [`fs`](fs.md) | Filesystem operations and Path objects |
| [`http`](http.md) | HTTP client, server, and URL builder |
| [`ssh`](ssh.md) | Remote command execution and file transfer |
| [`json`](json.md) | JSON encoding, decoding, and file I/O |
| [`yaml`](yaml.md) | YAML encoding, decoding, and file I/O |
| [`csv`](csv.md) | CSV reading, writing, and file I/O |
| [`gzip`](gzip.md) | Gzip compression and decompression |
| [`zip`](zip.md) | ZIP archive reading and writing |
| [`base64`](base64.md) | Base64 encoding and decoding |
| [`hash`](hash.md) | Cryptographic hash functions |
| [`strings`](strings.md) | String utility functions |
| [`regexp`](regexp.md) | Regular expression matching and replacement |
| [`template`](template.md) | Go text/template rendering |
| [`time`](time.md) | Time, duration, and arithmetic |
| [`uuid`](uuid.md) | UUID generation |
| [`fmt`](fmt.md) | Formatted printing and string formatting |
| [`log`](log.md) | Structured logging |
| [`table`](table.md) | ASCII table rendering |
| [`concur`](concur.md) | Concurrent execution |
| [`retry`](retry.md) | Retry logic with backoff |
| [`io`](io.md) | Interactive user input |
| [`vars`](vars.md) | Typed variable access |
| [`runtime`](runtime.md) | Runtime and platform information |
| [`inventory`](inventory.md) | Inventory management |
| [`test`](test.md) | Testing assertions |
| [`k8s`](k8s.md) | Kubernetes resource management (Cloud edition) |
| [`ai`](ai.md) | Multi-provider LLM client with chat, tools, and agents (AI edition) |
| [`mcp`](mcp.md) | Model Context Protocol server + client (AI edition) |

## Auto-Loading

Unlike standard Starlark, starkite modules do not require `load()` statements. All modules are injected into the global scope automatically:

```python
# No import needed — just use the module directly
content = read_text("config.yaml")
data = yaml.decode(content)
print(os.hostname())
```

## The load() Function

You can still use `load()` to import symbols from other `.star` files in your project:

```python
load("helpers.star", "deploy", "rollback")

deploy("production")
```

The `load()` function searches relative to the current script's directory.

## The try_ Pattern

Every module function that can fail has a corresponding `try_` variant. Instead of raising an error, `try_` functions return a `Result` object:

```python
# Raises an error if the file doesn't exist
content = read_text("/etc/missing")

# Returns a Result — never raises
result = path("/etc/missing").try_read_text()
if result.ok:
    print(result.value)
else:
    print("Error:", result.error)
```

The `Result` type has three attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `ok` | `bool` | `True` if the operation succeeded |
| `value` | `any` | The return value on success |
| `error` | `string` | The error message on failure |

This pattern applies uniformly to all modules — module-level functions, factory functions, and object methods all support `try_` variants. See the [Error Handling guide](../guides/error-handling.md) for more details.

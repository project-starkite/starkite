---
title: "runtime"
description: "Runtime and platform information"
weight: 24
---

The `runtime` module provides information about the current platform, architecture, and runtime environment.

## Built-in Functions

Top-level runtime builtins available in every script without a module prefix:

| Function | Returns | Description |
|----------|---------|-------------|
| `struct(**kwargs)` | `struct` | Create an immutable dot-accessible struct from keyword arguments (`s.field` read, `+` merge) |
| `Result(ok, value=None, error="")` | `Result` | Construct a `Result` value for explicit success or error handling |
| `defer(fn)` | `None` | Register a cleanup function to run when the script exits |
| `on_signal(signal, handler)` | `None` | Register a handler for OS signals (for example, `"SIGINT"`, `"SIGTERM"`) |

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `runtime.platform()` | `string` | Operating system name (e.g. `"linux"`, `"darwin"`, `"windows"`) |
| `runtime.arch()` | `string` | CPU architecture (e.g. `"amd64"`, `"arm64"`) |
| `runtime.cpu_count()` | `int` | Number of logical CPUs |
| `runtime.version()` | `string` | Starkite version string |
| `runtime.go_version()` | `string` | Go runtime version |
| `runtime.uname()` | `dict` | System information dictionary with keys: `system`, `node`, `release`, `version`, `machine` (Unix only; returns an error on Windows) |

## Examples

### Platform detection

```python
if runtime.platform() == "linux":
    exec("apt-get update")
elif runtime.platform() == "darwin":
    exec("brew update")
```

### Architecture-specific builds

```python
os_name = runtime.platform()
arch = runtime.arch()
target = os_name + "/" + arch

print("Building for:", target)
exec("GOOS=%s GOARCH=%s go build -o app ./cmd" % (os_name, arch))
```

### System information

```python
info = runtime.uname()
print("System:", info["system"])
print("Host:", info["node"])
print("Kernel:", info["release"])
print("Arch:", info["machine"])
```

### Version info

```python
print("kite version:", runtime.version())
print("Go version:", runtime.go_version())
print("CPUs:", runtime.cpu_count())
```

### Cross-platform script

```python
bin_name = "myapp"
if runtime.platform() == "windows":
    bin_name += ".exe"

exec("go build -o " + bin_name)
```

### Immutable structs

```python
endpoint = struct(host = "db.internal", port = 5432, tls = True)
print(endpoint.host, endpoint.port)

merged = endpoint + struct(port = 6432, pool = 10)
print(json.encode(merged))
```

---
title: "runtime"
description: "Runtime and platform information"
weight: 24
---

The `runtime` module provides information about the current platform, architecture, and runtime environment.

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

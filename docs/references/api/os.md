---
title: "os"
description: "Environment, process info, and command execution"
weight: 1
---

The `os` module provides access to environment variables, process information, and command execution.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `os.env(name, default="")` | `string` | Get environment variable, with optional default |
| `os.setenv(name, value)` | `None` | Set environment variable |
| `os.cwd()` | `string` | Get current working directory |
| `os.chdir(path)` | `None` | Change current working directory |
| `os.hostname()` | `string` | Get system hostname |
| `os.pid()` | `int` | Get current process ID |
| `os.ppid()` | `int` | Get parent process ID |
| `os.exit(code=0)` | `None` | Exit the process with the given code |
| `os.exec(cmd, shell=None, env=None, cwd=None, timeout="60s")` | `ExecResult` | Execute a system command |
| `os.which(name)` | `string`/`None` | Find executable on PATH |
| `os.username()` | `string` | Get current username |
| `os.userid()` | `string` | Get current user ID |
| `os.groupid()` | `string` | Get current group ID |
| `os.home()` | `string` | Get home directory path |

## Global exec() Alias

The `exec()` function is available as a top-level global, equivalent to `os.exec()`:

```python
# These are identical
result = exec("ls -la")
result = os.exec("ls -la")
```

## ExecResult

The `ExecResult` object returned by `os.exec()` and `exec()` has these attributes:

| Attribute | Type | Description |
|-----------|------|-------------|
| `stdout` | `string` | Standard output of the command |
| `stderr` | `string` | Standard error of the command |
| `code` | `int` | Exit code (0 = success) |
| `ok` | `bool` | `True` if exit code is 0 |
| `error` | `string` | Error message on failure; empty string when `ok` is `True` |

## Examples

### Environment variables

```python
home = os.env("HOME")
path = os.env("MY_VAR", "default_value")

os.setenv("DEPLOY_ENV", "production")
```

### Process information

```python
print("Host:", os.hostname())
print("User:", os.username())
print("PID:", os.pid())
print("CWD:", os.cwd())
print("Home:", os.home())
```

### Command execution

```python
# Simple command
result = exec("git status")
if result.ok:
    print(result.stdout)
else:
    print("Failed:", result.stderr)

# With options
result = os.exec(
    "make build",
    cwd="/home/user/project",
    env={"GOOS": "linux", "GOARCH": "amd64"},
    timeout="120s",
)

# Find an executable
go_path = os.which("go")
if go_path:
    print("Go found at:", go_path)
```

### Changing directories

```python
os.chdir("/tmp")
print(os.cwd())  # /tmp
```

> **Note:**
All `os` functions that can fail support `try_` variants. For example, `os.try_exec()` returns a `Result` instead of raising on non-zero exit codes.


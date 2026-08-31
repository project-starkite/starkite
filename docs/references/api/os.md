---
title: "os"
description: "Environment, process info, and command execution"
weight: 1
keywords: [os, environment, process, exec, command, subprocess, shell, chdir, env, exit]
---

The `os` module provides access to environment variables, process information, and command execution.

## Functions

| Function | Returns | Description |
|----------|---------|-------------|
| `os.env(name, default="")` | `string` | Get environment variable, with optional default |
| `os.setenv(name, value)` | `None` | Set environment variable |
| `os.cwd()` | `string` | Get current working directory |
| `os.chdir(path)` | `None` | Change current working directory |
| `os.temp_dir()` | `string` | Get default temporary directory path (`/tmp` on POSIX, `%TEMP%` on Windows) |
| `os.hostname()` | `string` | Get system hostname |
| `os.pid()` | `int` | Get current process ID |
| `os.ppid()` | `int` | Get parent process ID |
| `os.exit(code=0)` | `None` | Exit the process with the given code |
| `os.exec(cmd, args=[], env=None, cwd=None, timeout="60s", userid=None, groupid=None, input=None, output=None)` | `string` | Execute a binary directly (returns stdout) |
| `os.try_exec(cmd, args=[], env=None, cwd=None, timeout="60s", userid=None, groupid=None, input=None, output=None)` | `ExecResult` | Execute a binary directly, capturing results |
| `os.which(name)` | `string`/`None` | Find executable on PATH |
| `os.username()` | `string` | Get current username |
| `os.userid()` | `string` | Get current user ID |
| `os.groupid()` | `string` | Get current group ID |
| `os.home()` | `string` | Get home directory path |

## Global Aliases

The following functions are available as top-level globals, equivalent to their `os.` counterparts:
* `env(name, default=None)`
* `setenv(name, value)`
* `cwd()`
* `chdir(path)`
* `temp_dir()`
* `hostname()`
* `pid()`
* `ppid()`
* `exit(code=0)`
* `exec(cmd, args=[])`
* `try_exec(cmd, args=[])`
* `which(name)`
* `username()`
* `userid()`
* `groupid()`
* `home()`

```python
# These are identical
result = exec("uname", ["-a"])
result = os.exec("uname", ["-a"])
```

## ExecResult

The `ExecResult` object returned by `os.try_exec()` and `try_exec()` has these attributes:

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
# Direct execution using a single space-separated string
output = exec("git status")
print(output)

# Direct execution with arguments list
result = try_exec("git", ["status"])
if result.ok:
    print(result.stdout)
else:
    print("Failed:", result.error)

# Shell features (pipes and redirects via explicit shell invocation)
result = try_exec("sh", ["-c", "df -h / | tail -1"])
if result.ok:
    print(result.stdout)

# With options
result = os.try_exec(
    "make",
    ["build"],
    cwd="/home/user/project",
    env={"GOOS": "linux", "GOARCH": "amd64"},
    timeout="120s",
)

# With user and group execution switching (requires POSIX and allow-all permissions)
result = os.try_exec(
    "id",
    [],
    userid="nobody",
    groupid="nogroup",
)

# Streaming input and output (Unified Streaming Contract)
# Pipe file content directly as subprocess stdin and redirect stdout to another file
in_file = fs.path("input.txt")
out_file = fs.path("output.txt")
in_file.write_text("stream data")

exec(
    "cat",
    input=in_file.get_reader(),
    output=out_file.get_writer()
)
print("Piped output:", out_file.read_text())

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

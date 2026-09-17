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
| `os.shell(command=None, flag=None, cwd=None, env=None, timeout="60s", userid=None, groupid=None)` | `Shell` | Construct a configured shell execution object |
| `os.sh(...)` | `Shell` | Construct `/bin/sh` shell (`flag="-c"`) |
| `os.bash(...)` | `Shell` | Construct `/bin/bash` shell (`flag="-c"`) |
| `os.zsh(...)` | `Shell` | Construct `/bin/zsh` shell (`flag="-c"`) |
| `os.cmdexe(...)` | `Shell` | Construct Windows `cmd.exe` shell (`flag="/c"`) |
| `os.powershell(...)` | `Shell` | Construct PowerShell shell (`flag="-Command"`, resolves `pwsh` or `powershell.exe`) |
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
* `shell(...)`
* `sh(...)`
* `bash(...)`
* `zsh(...)`
* `cmdexe(...)`
* `powershell(...)`
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

## Shell

The `Shell` object returned by `os.shell()` and the shell factory shortcuts (`os.sh()`, `os.bash()`, `os.zsh()`, `os.cmdexe()`, `os.powershell()`) encapsulates shell configuration and executes scripts via `os.exec`:

### Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `command` | `string` | Shell executable path or binary name |
| `flag` | `string` | Flag passed before the script argument (`-c`, `/c`, `-Command`) |
| `cwd` | `string` | Bound working directory (`""` if unset) |
| `timeout` | `string` | Bound execution timeout string |

### Methods

| Method | Returns | Description |
|--------|---------|-------------|
| `sh.exec(script, cwd=None, env=None, timeout=None, userid=None, groupid=None, input=None, output=None)` | `string` | Execute script via shell; returns stdout. Raises error on non-zero exit. |
| `sh.try_exec(script, cwd=None, env=None, timeout=None, userid=None, groupid=None, input=None, output=None)` | `ExecResult` | Execute script via shell; returns `ExecResult`. |

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
# Single command string with quotes and spaces
output = exec('git commit -m "initial commit"')
print(output)

# Single command string preserving single-quoted literal payloads
output = exec("grep -E '^[0-9]+' data.txt")

# Direct execution with arguments list
result = try_exec("git", ["commit", "-m", "initial commit"])
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

### Shell execution

```python
# Create a shell runner (defaults to /bin/sh on POSIX, cmd.exe on Windows)
sh = os.sh()

# Pipelines and redirections
files = sh.exec("ls -la | grep -E '\\.star$'")
print("Files:", files)

# Multi-line scripts
script = """
set -e
echo "Starting task..."
echo "Completed"
"""
sh.exec(script)

# Error handling with try_exec
res = sh.try_exec("exit 42")
if not res.ok:
    print("Failed with exit code:", res.code)

# Bound options and per-call overrides
ci_shell = os.bash(cwd="/repo", env={"CI": "true"}, timeout="5m")
ci_shell.exec("make test", env={"VERBOSE": "1"})

# PowerShell execution (cross-platform pwsh or Windows PowerShell)
ps = os.powershell()
ps.exec("Get-Process | Select-Object -First 5")
```

> **Note:**
All `os` functions that can fail support `try_` variants. For example, `os.try_exec()` returns a `Result` instead of raising on non-zero exit codes.

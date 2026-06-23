---
title: "System and processes"
description: "Manage host environments, process contexts, and shell command execution with the os module"
weight: 3
---

# System and processes

The `os` module manages host environments, process contexts, and shell command execution. Scripts use it to query system information (like hostname, user accounts, and process IDs), manipulate environment variables, and run external commands as sub-processes.

Source: [sysinfo.star](https://github.com/project-starkite/starkite/blob/main/examples/core/sysinfo.star) (excerpted)

## Permissions

To execute shell commands or read host environment variables, you must run Starkite with a permission profile that authorizes access to those host resources. The runtime evaluates every `os` operation against the following requirements:

* **Inspecting Environment**: Reading environment variables via `os.env` requires `allow-fs` or higher.
* **Local Command Execution**: Running a command located inside the script's working tree (`$CWD`) requires the `allow-local` profile.
* **System Command Execution**: Running a command located anywhere else on the system requires the `allow-all` profile.

To ensure host security, always run scripts at the tightest permission profile that supports the operations they invoke. For example, to run a script that executes local commands under the current directory, use the `--allow-local` flag:

```bash
kite run ./sysinfo.star --allow-local
```

For more information, see the [Permission Guide](../fundamentals/security/permission.md).

## Host and user context

Host properties and user account information can be inspected using built-in functions on the `os` module or their global aliases:

| Attribute / Function | Alias | Purpose |
|----------------------|-------|---------|
| `os.hostname()` | `hostname()` | Returns the system hostname. |
| `os.user.name` | `username()` | Returns the username of the current user. |
| `os.user.id` | `userid()` | Returns the user ID (UID) of the current user. |
| `os.user.gid` | `groupid()` | Returns the primary group ID (GID) of the current user. |
| `os.user.home` | `home()` | Returns the home directory path of the current user. |

Example usage:

```python
# Query host context
printf("Host: %s\n", hostname())

# Query user context
printf("User: %s (UID: %s, GID: %s)\n", os.user.name, os.user.id, os.user.gid)
printf("Home Directory: %s\n", os.user.home)
```

## Environment and directory management

The `os` module provides functions to query and modify environment variables and the current working directory.

* **`os.env(name, [default])`** (Alias: `env`): Retrieves the value of an environment variable. Returns the optional default value if the variable is not set.
* **`os.setenv(name, value)`** (Alias: `setenv`): Sets the value of an environment variable for the script process and any subsequent sub-processes.
* **`os.cwd()`** (Alias: `cwd`): Returns the absolute path of the current working directory.
* **`os.chdir(path)`** (Alias: `chdir`): Changes the script's current working directory for relative path operations and sub-process execution.

Example usage:

```python
# Read and modify environment
port = env("APP_PORT", "8080")
setenv("PORT", port)

# Manage working directory
original_dir = cwd()
chdir("/tmp")
# Relative actions run under /tmp here
chdir(original_dir)
```

## Executable resolution and process IDs

* **`os.pid()`** (Alias: `pid`): Returns the process ID of the running script.
* **`os.ppid()`** (Alias: `ppid`): Returns the parent process ID.
* **`os.which(name)`** (Alias: `which`): Locates an executable in the system `PATH`. Returns `None` if the executable is not found.

Example usage:

```python
# Check process context
printf("PID: %d (Parent PID: %d)\n", pid(), ppid())

# Locate binary paths before execution
git_path = which("git")
if git_path:
    printf("Found git at: %s\n", git_path)
else:
    fail("git is required but not installed on PATH")
```

## Executing local commands

Starkite runs external CLI commands inside a shell wrapper (default: `/bin/sh -c`) and captures their stdout and stderr streams. Use `os.exec` when a command failure should immediately halt execution, or `os.try_exec` to handle exit status codes programmatically:

* **`os.exec(cmd, [options])`** (Alias: `exec`): Executes `cmd` and returns standard output as a string. A non-zero exit code raises a Starlark-level error, halting the script.
* **`os.try_exec(cmd, [options])`** (Alias: `try_exec`): Executes `cmd` and returns an `ExecResult` object. It never raises an error on command failure, allowing you to handle the exit status programmatically.

### Execution options

Both execution functions accept the following keyword arguments:

| Option | Type | Default | Purpose |
|--------|------|---------|---------|
| `shell` | `string` | `"/bin/sh"` | Shell binary used to execute the command string (e.g., `"/bin/bash"`). |
| `cwd` | `string` | `""` | Working directory in which to run the sub-process. |
| `env` | `dict` | `None` | Environment variable overrides (mapping string to string) for the command execution context. |
| `timeout` | `string` | `"60s"` | Time limit for execution (e.g., `"10s"`, `"5m"`). The process is killed if the timeout is exceeded. |

### Handling results

`os.try_exec` returns an `ExecResult` struct containing the following attributes:

* **`.ok`** (`bool`): `True` if the command exited with code `0` and no internal errors occurred.
* **`.code`** (`int`): The integer process exit code returned by the command.
* **`.stdout`** (`string`): The standard output stream captured from the command.
* **`.stderr`** (`string`): The standard error stream captured from the command.
* **`.error`** (`string`): A combined error message if the command failed or timed out.

### Code example

```python
# OS info - halts execution if uname fails
os_info = exec("uname -a")
print("System OS: " + os_info.strip())

# Disk check with try_exec - safe to handle failure programmatically
disk = try_exec(
    "df -h / | tail -1",
    timeout = "5s",
    cwd = "/tmp",
    env = {"LANG": "C"}
)

if disk.ok:
    fields = disk.stdout.split()
    printf("Disk Available: %s (Used: %s)\n", fields[3], fields[4])
else:
    printf("Warning: Disk check failed with code %d\n", disk.code)
```

## See also

* [`os` reference](../references/api/os.md) — `exec`, `try_exec`, `env`, `setenv`, `which`
* [Language: error handling](../fundamentals/language.md#error-handling) — the `try_` pattern and `ExecResult`


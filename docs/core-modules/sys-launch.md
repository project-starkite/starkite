---
title: "Launching processes"
description: "Execute local shell commands and manage sub-processes in Starkite"
weight: 20
---

# Launching processes

Starkite runs external CLI commands inside a shell wrapper (default: `/bin/sh -c`) and captures their stdout and stderr streams. The `os` module provides two functions to execute commands: `os.exec()` for commands that must succeed, and `os.try_exec()` for commands where you want to handle failures programmatically.

---

## Launching Processes

Starkite executes external shell commands by launching a shell wrapper (default: `/bin/sh -c` on Unix/Linux systems) rather than executing target binaries directly via Go's standard `os/exec` package. This shell-wrapping mechanism provides several benefits to script authors, such as native support for shell pipes (`|`), input/output redirections (`>`, `<`), environment variable expansion (e.g., `$VAR`), command substitution, and filename globbing.

The `os.exec()` function is designed for commands that must succeed for the script to continue. When using `os.exec()`, if the command returns a non-zero exit code, the runtime immediately halts execution and raises a Starlark-level error. This behavior ensures that scripting pipelines fail fast when critical commands fail.

```python
def check_os():
    # Halts execution if uname fails
    os_info = os.exec("uname -a")
    print("System OS:", os_info.strip())
```

---

## Programmatic Error Handling (`os.try_exec`)

Use `os.try_exec()` when you need to handle exit status codes programmatically. It never raises a Starlark error on command failure; instead, it returns an `ExecResult` object.

```python
def check_disk_space():
    # Safe to handle failure programmatically
    disk = os.try_exec(
        "df -h / | tail -1",
        timeout = "5s",
        cwd = "/tmp",
        env = {"LANG": "C"},
    )
    
    if disk.ok:
        fields = disk.stdout.split()
        printf("Disk Available: %s (Used: %s)\n", fields[3], fields[4])
    else:
        printf("Warning: Disk check failed with code %d\n", disk.code)
```

---

## Execution Options

Both `os.exec()` and `os.try_exec()` accept the following optional keyword arguments:

| Option | Type | Default | Purpose |
|--------|------|---------|---------|
| `shell` | `string` | `"/bin/sh"` | Shell binary used to execute the command string (e.g., `"/bin/bash"`). |
| `cwd` | `string` | `""` | Working directory in which to run the sub-process. |
| `env` | `dict` | `None` | Environment variable overrides (mapping string to string) for the command execution context. |
| `timeout` | `string` | `"60s"` | Time limit for execution (e.g., `"10s"`, `"5m"`). The process is killed if the timeout is exceeded. |
| `userid` | `string` \| `int` | `None` | User identity under which to run the process (POSIX only). See details below. |
| `groupid` | `string` \| `int` | `None` | Group identity under which to run the process (POSIX only). See details below. |

---

## Handling Results

`os.try_exec()` returns an `ExecResult` struct containing the following attributes:

*   **`.ok`** (`bool`): `True` if the command exited with code `0` and no internal errors occurred.
*   **`.code`** (`int`): The integer process exit code returned by the command.
*   **`.stdout`** (`string`): The standard output stream captured from the command.
*   **`.stderr`** (`string`): The standard error stream captured from the command.
*   **`.error`** (`string`): A combined error message if the command failed or timed out.

---

## User and Group Execution

Local command execution via `os.exec()` and `os.try_exec()` natively supports running sub-processes under specified user and group identities (UID/GID) in POSIX environments (Linux, macOS).

Use the `userid` and `groupid` optional keyword arguments to configure the OS credentials of the spawned process:

*   **`userid`** (`string` | `int`): The username or numeric User ID (UID) of the target user.
*   **`groupid`** (`string` | `int`): The group name or numeric Group ID (GID) of the target group.

### Examples

#### Running as a Specific User (Username)

To run a command as a specific user, pass the username to `userid`:

```python
def query_database():
    # Runs the psql command directly as the postgres user
    result = os.try_exec("psql -c 'SELECT version();'", userid="postgres")
    if result.ok:
        print("Database Version:", result.stdout.strip())
```

#### Running with Numeric IDs

To run a command using specific numeric UID and GID:

```python
def run_unprivileged_task():
    # Runs the command as UID 65534 (nobody) and GID 65534 (nogroup)
    result = os.exec("id", userid=65534, groupid=65534)
    print("Identity:", result.strip())
```

### Required Privileges

Changing process credentials (setuid/setgid) requires the parent `kite` process to have sufficient operating system privileges (typically running as `root` or having `CAP_SETUID`/`CAP_SETGID` capabilities). If `kite` is run under a standard non-privileged user, the OS kernel will reject the credential switch, and `os.exec()` will return a standard OS permission error (e.g., `operation not permitted`).

### Starkite Permission Profile

Because switching process credentials is a highly sensitive operation, the Starkite permission engine requires the **`allow-all`** permission profile when `userid` or `groupid` arguments are used.

```bash
# Run Starkite with required system-level permissions
kite run ./deploy.star --allow-all
```

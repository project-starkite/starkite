---
title: "Process context"
description: "Inspect host information, user profiles, environment variables, and process IDs"
weight: 30
---

# Process context

The `os` module allows you to inspect the host environment, user accounts, current working directories, and process identifiers of the executing script.

---

## Host and User Context

You can query host properties and user account information using the following attributes and functions:

| Attribute / Function | Alias | Purpose |
|----------------------|-------|---------|
| `os.hostname()` | `hostname()` | Returns the system hostname. |
| `os.user.name` | `username()` | Returns the username of the current user. |
| `os.user.id` | `userid()` | Returns the user ID (UID) of the current user. |
| `os.user.gid` | `groupid()` | Returns the primary group ID (GID) of the current user. |
| `os.user.home` | `home()` | Returns the home directory path of the current user. |

### Example

```python
def print_context():
    # Query host context
    printf("Host: %s\n", os.hostname())
    
    # Query user context
    printf("User: %s (UID: %s, GID: %s)\n", os.user.name, os.user.id, os.user.gid)
    printf("Home Directory: %s\n", os.user.home)
```

---

## Environment and Directory Management

You can inspect and modify the script's environment variables and current working directory. Any modifications apply to the script's process and any subsequent sub-processes it spawns.

*   **`os.env(name, [default])`** (Alias: `env`): Retrieves the value of an environment variable. Returns the optional default value if the variable is not set.
*   **`os.setenv(name, value)`** (Alias: `setenv`): Sets the value of an environment variable.
*   **`os.cwd()`** (Alias: `cwd`): Returns the absolute path of the current working directory.
*   **`os.chdir(path)`** (Alias: `chdir`): Changes the current working directory for relative path operations and sub-process execution.

### Example

```python
def manage_environment():
    # Read and modify environment
    port = os.env("APP_PORT", "8080")
    os.setenv("PORT", port)
    
    # Manage working directory
    original_dir = os.cwd()
    os.chdir("/tmp")
    # Relative actions run under /tmp here
    os.chdir(original_dir)
```

---

## Executable Resolution and Process IDs

You can retrieve the process identifiers of the running script and locate binaries in the system `PATH`.

*   **`os.pid()`** (Alias: `pid`): Returns the process ID of the running script.
*   **`os.ppid()`** (Alias: `ppid`): Returns the parent process ID.
*   **`os.which(name)`** (Alias: `which`): Locates an executable in the system `PATH`. Returns `None` if the executable is not found.

### Example

```python
def inspect_process():
    # Check process context
    printf("PID: %d (Parent PID: %d)\n", os.pid(), os.ppid())
    
    # Locate binary paths before execution
    git_path = os.which("git")
    if git_path:
        printf("Found git at: %s\n", git_path)
    else:
        fail("git is required but not installed on PATH")
```

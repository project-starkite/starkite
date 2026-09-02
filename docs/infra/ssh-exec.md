---
title: "Executing host commands"
description: "Run remote commands, handle execution results, and manage environment contexts"
weight: 42
---

# Executing host commands

Once you have configured an SSH client, you can execute shell commands across your target hosts. Starkite dispatches commands concurrently and returns a list of detailed execution results.

## Running Commands

To run a command, call the `client.exec()` method. This function dispatches the command and blocks until execution completes, returning a list of `SSHResult` objects (one per target host).

```python
def check_host_uptime():
    client = ssh.config(
        hosts = ["alice-node-1.local", "alice-node-2.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Run the command on all target hosts concurrently
    results = client.exec("uptime")
    
    # Process the result from each host
    for r in results:
        if r.ok:
            print("Host:", r.host, "Uptime:", r.stdout.strip())
        else:
            log.error("Failed to run command", {"host": r.host, "error": r.stderr})
```

---

## Processing Execution Results

The `client.exec()` method always returns a list of `SSHResult` objects, even when targeting a single host. Each result object exposes the following attributes to let you handle success and failures:

* **`.host`**: The hostname or IP address of the node that returned the result.
* **`.ok`**: A boolean that is `True` if the command executed successfully (returned exit code `0`).
* **`.code`**: The integer exit code returned by the remote shell process.
* **`.stdout`**: The standard output stream returned by the command.
* **`.stderr`**: The standard error stream returned by the command.

---

## Directory and Environment Context

You can control the remote execution context by specifying a working directory (`cwd`) or passing a dictionary of environment variables (`env`):

```python
def run_build_pipeline():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Run build commands in a specific directory with custom environment variables
    results = client.exec(
        cmd = "make build",
        cwd = "/opt/alice-app",
        env = {
            "APP_ENV": "production",
            "BUILD_VERSION": "1.4.0",
        },
    )
    
    for r in results:
        print(r.host, "build exit code:", r.code)
```

---

## Privilege Escalation (Sudo)

For commands requiring root privileges, pass `sudo = True` directly to `client.exec()`. This overrides any client-level default and prefixes the command with `sudo `:

```python
def restart_web_server():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Restarts the service with root privileges
    results = client.exec("systemctl restart nginx", sudo=True)
    for r in results:
        if not r.ok:
            log.error("Failed to restart service", {"host": r.host, "error": r.stderr})
```

### Running as Another User

To execute a command as a different non-root user, combine `sudo = True` with the `as_user` parameter. This prefixes the command with `sudo -u <username> `:

```python
def run_db_vacuum():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        auth  = {
            "user": "alice",
            "key":  "~/.ssh/id_ed25519",
        },
    )
    
    # Executed as: sudo -u postgres vacuumdb -a
    client.exec("vacuumdb -a", sudo=True, as_user="postgres")
```

> [!IMPORTANT]
> Starkite assumes that the SSH user has passwordless `sudo` configured on the target hosts. If the target host prompts for a sudo password, the command will block or fail.

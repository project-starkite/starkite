---
title: "Connecting with SSH"
description: "Configure SSH clients, manage private key authentication, and establish connections"
weight: 40
---

# Connecting with SSH

The `ssh` module provides a secure client to connect to remote hosts. Connections are configured once using the `ssh.config()` function and are automatically managed by the runtime, reusing active sessions to eliminate connection handshake overhead.

## Configuring the Client

To establish a connection, build an SSH client by specifying target hosts, the SSH username, and the path to your private key:

```python
def main():
    # Configure the SSH client
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
        timeout = "10s",
    )
    print("SSH client configured for:", client.hosts)
```

The `hosts` parameter accepts a list of target hostnames or IP addresses. The client will establish connections to all named hosts.

---

## Configuration Parameters

You can customize connection behavior using the following optional arguments in `ssh.config()`:

* **`user`**: The SSH username used to establish the connection (defaults to the current local system user if omitted).
* **`key`**: The local path to your private SSH key (highly recommended over passwords).
* **`key_passphrase`**: The passphrase to decrypt your private key, if required.
* **`port`**: The target SSH port (defaults to `22`).
* **`timeout`**: A duration string (e.g., `"10s"`, `"30s"`) defining the connection timeout.
* **`host_key_check`**: A boolean (defaults to `True`) to verify the remote host's signature against your `known_hosts` file.
* **`max_retries`**: The number of reconnection attempts if a connection is dropped.

---

## Configuring Default Privileges (Sudo)

If your automation primarily performs administrative operations (such as installing software, restarting system daemons, or modifying system configurations), you can enable `sudo` globally at the client level.

### What `sudo = True` Does under the Hood
When you configure `sudo = True` in `ssh.config()`, Starkite establishes a client-wide default that automatically prefixes all subsequent remote commands with `sudo `. By default, `sudo` elevates the execution context of the command to the `root` user.

### Combining with the SSH User
Starkite establishes the initial SSH connection using the SSH user specified in the configuration (e.g., `user = "alice"`). 

When `sudo = True` is enabled, the runtime authenticates as `alice` and executes the command as `sudo <cmd>`.

> [!IMPORTANT]
> This assumes that the SSH user `alice` is configured in the remote host's `sudoers` file and has passwordless `sudo` privileges. If the remote host prompts for a password, the execution will block or fail.

### Combining with Target Users (as_user)
If you need to run commands as a specific non-root user (e.g., executing database commands as the `postgres` user or running web server actions as `www-data`), you can combine the client-wide `sudo = True` setting with the `as_user` parameter during command execution:

```python
def run_database_maintenance():
    # 1. Connect as 'alice' with global sudo enabled
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
        sudo  = True,
    )
    
    # 2. Execute a command as 'postgres'
    # Starkite translates this to: sudo -u postgres vacuumdb -a
    client.exec("vacuumdb -a", as_user="postgres")
```

For more details on executing commands and overriding defaults on a per-call basis, see the [Executing host commands](ssh-exec.md) guide.

---

## Required Permissions

Establishing network connections over SSH requires explicit authorization. Starkite gates the `ssh` module behind the **`allow-net`** permission profile to prevent scripts from initiating unauthorized network connections.

To run a script that connects to remote hosts over SSH, you must pass the `--permissions=allow-net` flag at the command line:

```bash
# Run the script with network permissions
kite run ./deploy.star --permissions=allow-net
```

Under the default `deny-all` profile, any attempt to call `ssh.config()` or interact with the `ssh` module will be blocked by the runtime.

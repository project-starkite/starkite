---
title: "Remote commands (SSH)"
description: "Run commands and transfer files over SSH on individual hosts"
weight: 40
---

# Remote commands (SSH)

The `ssh` module allows you to execute commands, manage system services, and transfer files over SSH. By configuring a client with private key authentication, you can automate host-level operations directly from Starlark scripts.

## Configuring a Client

To connect to a host, build a client using `ssh.config()`. All operations executed on the client are dispatched to the configured hosts.

```python
def main():
    # Configure a client for a specific host
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
        timeout = "10s",
    )
    
    # Execute a basic command
    results = client.exec("uptime")
    for r in results:
        if r.ok:
            print(r.host, ":", r.stdout.strip())
        else:
            print("Failed on", r.host, ":", r.stderr)
```

The `hosts` argument accepts a list of target hostnames or IP addresses. All client methods return a list of result objects—one per host—even when targeting a single host.

## Privilege Escalation (sudo)

For administrative tasks, set `sudo=True` in `client.exec()`. You can also execute the command as a specific user using the `as_user` parameter:

```python
def restart_service():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
    )
    
    # Run service restart with sudo privileges
    results = client.exec("systemctl restart nginx", sudo=True)
    for r in results:
        if not r.ok:
            log.error("Failed to restart service", {"host": r.host, "error": r.stderr})
```

## Running with Directory and Environment Context

You can control the execution environment of the remote command by setting the working directory (`cwd`) and passing a dictionary of environment variables (`env`):

```python
def run_deploy():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
    )
    
    # Run deployment commands in a specific directory with environment variables
    results = client.exec(
        cmd = "make install",
        cwd = "/opt/alice-app",
        env = {"APP_ENV": "production", "VERSION": "1.4.0"},
    )
    for r in results:
        print(r.host, "exit code:", r.code)
```

## File Transfers

The `ssh` module supports file upload and download operations, returning a list of transfer result objects.

### Uploading Files

Upload local configuration files or payloads to remote hosts using `client.upload()`:

```python
def deploy_config():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
    )
    
    # Upload a local config file and set its permissions
    results = client.upload(
        src = "configs/app.conf",
        dst = "/etc/alice-app/app.conf",
        mode = "0640",
    )
    for r in results:
        if r.ok:
            print(r.host, "uploaded", r.bytes, "bytes")
```

### Downloading Files

Retrieve log files, backups, or diagnostics from remote hosts using `client.download()`:

```python
def fetch_logs():
    client = ssh.config(
        hosts = ["alice-node-1.local"],
        user  = "alice",
        key   = "~/.ssh/id_ed25519",
    )
    
    # Download remote application logs to a local directory
    results = client.download(
        src = "/var/log/alice-app/error.log",
        dst = "./logs/",
    )
    for r in results:
        if r.ok:
            print(r.host, "downloaded log to", r.dst)
```

> [!NOTE]
> When downloading files from multiple hosts to the same destination directory, Starkite automatically appends the hostname to the local file path to prevent naming collisions.

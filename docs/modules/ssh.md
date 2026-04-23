---
title: "ssh"
description: "Remote command execution and file transfer over SSH"
weight: 4
---

The `ssh` module provides remote command execution and file transfer over SSH connections.

## Configuration

Create an SSH client with `ssh.config()`:

```python
client = ssh.config(
    hosts=["web-1", "web-2", "web-3"],
    user="deploy",
    key="~/.ssh/id_ed25519",
    port=22,
    timeout="30s",
    keep_alive_interval="30s",
)
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hosts` | `list[string]` | required | Target hostnames or IPs |
| `user` | `string` | current user | SSH username |
| `key` | `string` | `""` | Path to private key file |
| `password` | `string` | `""` | SSH password (prefer keys) |
| `port` | `int` | `22` | SSH port |
| `timeout` | `string` | `"30s"` | Connection timeout |
| `keep_alive_interval` | `string` | `"30s"` | Keep-alive interval |

## SSHClient Methods

### exec

Execute a command on all configured hosts.

```python
results = client.exec(cmd, sudo=False, as_user="", cwd="", env={})
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cmd` | `string` | required | Command to execute |
| `sudo` | `bool` | `False` | Run with sudo |
| `as_user` | `string` | `""` | Run as a specific user (with sudo) |
| `cwd` | `string` | `""` | Working directory for the command |
| `env` | `dict` | `{}` | Environment variables |

Returns a `list[SSHResult]`, one per host.

### upload

Upload a local file to all configured hosts.

```python
results = client.upload(src, dst, mode="0644")
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `src` | `string` | required | Local source file path |
| `dst` | `string` | required | Remote destination path |
| `mode` | `string` | `"0644"` | File permissions on remote |

Returns a `list[SSHTransferResult]`, one per host.

### download

Download a file from all configured hosts.

```python
results = client.download(src, dst)
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `src` | `string` | required | Remote source file path |
| `dst` | `string` | required | Local destination path |

Returns a `list[SSHTransferResult]`, one per host. When downloading from multiple hosts, the local filename is suffixed with the hostname to avoid collisions.

## SSHResult

Returned by `client.exec()`, one per host.

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | `string` | Hostname this result is from |
| `stdout` | `string` | Standard output |
| `stderr` | `string` | Standard error |
| `code` | `int` | Exit code |
| `ok` | `bool` | `True` if exit code is 0 |
| `dry_run` | `bool` | `True` if running in dry-run mode |

## SSHTransferResult

Returned by `client.upload()` and `client.download()`, one per host.

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | `string` | Hostname this result is from |
| `ok` | `bool` | `True` if transfer succeeded |
| `bytes` | `int` | Number of bytes transferred |
| `src` | `string` | Source path |
| `dst` | `string` | Destination path |

## Examples

### Remote command execution

```python
client = ssh.config(
    hosts=["app-1", "app-2"],
    user="deploy",
    key="~/.ssh/deploy_key",
)

# Run a command on all hosts
results = client.exec("uptime")
for r in results:
    print(r.host, ":", r.stdout.strip())

# Run with sudo
results = client.exec("systemctl restart myapp", sudo=True)
for r in results:
    if not r.ok:
        print("FAILED on", r.host, ":", r.stderr)

# Run in a specific directory with env vars
results = client.exec(
    "make deploy",
    cwd="/opt/myapp",
    env={"VERSION": "2.0.0"},
)
```

### File transfer

```python
client = ssh.config(
    hosts=["web-1", "web-2", "web-3"],
    user="deploy",
    key="~/.ssh/deploy_key",
)

# Upload config to all hosts
results = client.upload("nginx.conf", "/etc/nginx/nginx.conf", mode="0644")
for r in results:
    if r.ok:
        print(r.host, ": uploaded", r.bytes, "bytes")

# Download logs from all hosts
results = client.download("/var/log/app.log", "./logs/")
```

> **Note:**
All `SSHClient` methods support `try_` variants. For example, `client.try_exec(cmd)` returns a `Result` wrapping the list of `SSHResult` objects instead of raising on connection errors.


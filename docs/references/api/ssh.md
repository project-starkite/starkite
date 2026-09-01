---
title: "ssh"
description: "Remote command execution and file transfer over SSH"
weight: 4
keywords: [ssh, remote, command, execution, connect, scp, transfer, file, tunnel]
---

The `ssh` module provides remote command execution and file transfer over SSH connections.

## One-Shot Execution

For quick commands or health checks, call `ssh.exec()` or `ssh.try_exec()` directly without initializing a client:

```python
# One-shot command across a Fleet
results = ssh.exec("uptime", fleet=web_fleet, user="deploy")

# One-shot multi-command pipeline
results = ssh.exec(
    commands = [
        "git pull origin main",
        "systemctl restart webapp",
    ],
    hosts         = ["192.168.1.10", "192.168.1.11"],
    user          = "deploy",
    exec_on_error = "stop",
)

# Safe execution with try_exec
res = ssh.try_exec("hostname", hosts=["10.0.0.1"], user="admin")
if not res.ok:
    print("Execution failed:", res.error)
```

## Key Generation (`ssh.keygen`)

Generate in-memory or on-disk cryptographic SSH keypairs:

```python
# In-memory Ed25519 keypair
keys = ssh.keygen(type="ed25519", comment="cluster-admin")
print(keys.public_key)   # "ssh-ed25519 AAA... cluster-admin\n"
print(keys.fingerprint)  # "SHA256:..."

# On-disk keypair with 0600 permissions
keys = ssh.keygen(
    type       = "rsa",
    bits       = 4096,
    path       = "~/.ssh/id_cluster_rsa",
    passphrase = "optional_passphrase",
    overwrite  = True,
)
```

### Keygen Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `type` | `string` | `"ed25519"` | Algorithm (`"ed25519"`, `"rsa"`, `"ecdsa"`) |
| `bits` | `int` | `0` | RSA bits (`2048`, `3072`, `4096`) or ECDSA bits (`256`, `384`, `521`) |
| `comment` | `string` | `""` | Comment appended to public key line |
| `passphrase` | `string` | `""` | Optional passphrase to encrypt private key |
| `path` | `string` | `""` | Optional file path to write private key (`0600`) and `.pub` (`0644`) |
| `overwrite` | `bool` | `False` | Overwrite existing files when `path` is set |

### `SSHKeyPair` Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `public_key` | `string` | OpenSSH authorized public key line |
| `private_key` | `string` | PEM-encoded OpenSSH private key |
| `fingerprint` | `string` | SHA256 key fingerprint |
| `type` | `string` | Algorithm name (`"ed25519"`, `"rsa"`, `"ecdsa"`) |
| `comment` | `string` | Comment string |
| `path` | `string` | Saved private key path (or `""`) |
| `pub_path` | `string` | Saved public key path (or `""`) |

## Public Key Distribution (`ssh.copy_id`)

Distribute public keys to remote target hosts or fleets idempotently:

```python
# Distribute key to a fleet with one-shot ssh.copy_id
keys = ssh.keygen(type="ed25519", comment="deploy-key")
ssh.copy_id(
    key          = keys.public_key,
    fleet        = pi_cluster,
    user         = "pi",
    ask_password = True,  # Prompt once in terminal for fleet auth
)

# Distribute key via an existing SSH client instance
client = ssh.config(hosts=["192.168.1.50"], user="admin", password="password123")
results = client.copy_id(key=keys.public_key, sudo=True, as_user="pi")
```

### Copy ID Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `key` | `string` | `""` | Public key string or file path (reads local `~/.ssh/` or `ssh-agent` if omitted) |
| `use_agent` | `bool` | `False` | Explicit signal to use `ssh-agent` |
| `as_user` | `string` | `""` | Target user to install key for |
| `sudo` | `bool` | `False` | Run installer with sudo |
| `ask_password` | `bool` | `False` | Prompt operator for password in terminal (`isatty`) |

Returns a `list[SSHResult]`, one per host.

## Configuration

Create an SSH client with `ssh.config()`:

```python
# Option A: Target a Fleet directly
client = ssh.config(
    fleet=web_fleet,
    user="deploy",
    key="~/.ssh/id_ed25519",
    port=22,
    timeout="30s",
    exec_max_workers=16,
)

# Option B: Target hosts shortcut (list of strings or single string)
client = ssh.config(
    hosts=["web-1", "web-2", "web-3"],
    user="deploy",
    key="~/.ssh/id_ed25519",
)
```

### Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `fleet` | `Fleet` \| `source` | `None` | Compute resource `Fleet` instance to target |
| `hosts` | `list[string]` \| `string` | `[]` | Shortcut for target hostnames or IPs |
| `user` | `string` | current user | SSH username |
| `key` | `string` | `""` | Path to private key file |
| `key_passphrase` | `string` | `""` | Passphrase for private key |
| `password` | `string` | `""` | SSH password (prefer keys) |
| `use_agent` | `bool` | `false` | Authenticate using `ssh-agent` ($SSH_AUTH_SOCK) |
| `sudo` | `bool` | `false` | Default sudo execution policy for all commands |
| `port` | `int` | `22` | SSH port |
| `timeout` | `string` | `"30s"` | Connection timeout |
| `exec_policy` | `string` | `"concurrent"` | Execution strategy (`"concurrent"` or `"linear"`) |
| `exec_max_workers` | `int` | `0` | Max concurrent worker goroutines (`0` = unconstrained) |
| `exec_on_error` | `string` | `"stop"` | Multi-command error policy (`"stop"` or `"continue"`) |
| `jump_host` | `string` | `""` | Bastion jump host address |
| `jump_user` | `string` | `user` | Bastion jump host username (falls back to `user`) |
| `jump_key` | `string` | `key` | Bastion jump host private key (falls back to `key`) |
| `jump_password` | `string` | `password` | Bastion jump host password (falls back to `password`) |
| `jump_port` | `int` | `port` | Bastion jump host SSH port (falls back to `port` or `22`) |
| `host_key_check` | `bool` | `true` | Verify host key against known hosts |
| `max_retries` | `int` | `0` | Max reconnection retries |
| `keep_alive_interval` | `string` | `"30s"` | Keep-alive interval |

## SSHClient Methods

### exec

Execute a single command or a multi-command pipeline on all configured hosts.

```python
# Single string command
results = client.exec("k3s kubectl apply -f deploy.yaml", sudo=True)

# Structured argument list (safe argument passing)
results = client.exec("git", ["commit", "-m", "release v0.1.0"], cwd="/opt/app")

# Multi-command pipeline
batch_results = client.exec(
    commands = [
        "git pull origin main",
        "npm install --production",
        "systemctl restart webapp",
    ],
    exec_on_error = "stop",
)
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cmd` | `string` | required (if no `commands`) | Command or binary name to execute |
| `args` | `list[string]` | `[]` | Positional argument list |
| `commands` | `list[string]` | `[]` | Multi-command sequence to execute per host |
| `exec_max_workers` | `int` | client default | Override maximum concurrent active workers |
| `exec_on_error` | `string` | client default | Error handling policy (`"stop"` or `"continue"`) |
| `sudo` | `bool` | `False` | Run with sudo |
| `as_user` | `string` | `""` | Run as a specific user (with sudo) |
| `cwd` | `string` | `""` | Working directory for the command |
| `env` | `dict` | `{}` | Environment variables |

Returns `list[SSHResult]` when executing a single command, or `list[SSHBatchResult]` when `commands` is provided.

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

### copy_id

Distribute a public key to all hosts in the client fleet.

```python
results = client.copy_id(key=pub_key, sudo=True, as_user="pi")
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `key` | `string` | `""` | Public key string or file path |
| `as_user` | `string` | `""` | Target user to install key for |
| `sudo` | `bool` | `False` | Run installer with sudo |
| `ask_password` | `bool` | `False` | Prompt operator for password in terminal |

Returns a `list[SSHResult]`, one per host.

## SSHResult

Returned by `client.exec()` (or items within `SSHBatchResult.steps`), one per host.

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | `string` | Hostname this result is from |
| `cmd` | `string` | The command string executed |
| `stdout` | `string` | Standard output |
| `stderr` | `string` | Standard error |
| `code` | `int` | Exit code |
| `ok` | `bool` | `True` if exit code is 0 |
| `dry_run` | `bool` | `True` if running in dry-run mode |

## SSHBatchResult

Returned by `client.exec(commands=[...])` or `ssh.exec(commands=[...])`, one per host.

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | `string` | Hostname this batch result is from |
| `ok` | `bool` | `True` if all executed commands succeeded (code 0) |
| `stopped_early` | `bool` | `True` if execution stopped before reaching the end due to an error |
| `steps` | `list[SSHResult]` | List of individual step results in execution order |
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

# Run a multi-command sequence with fail-fast stopping
batch_results = client.exec(
    commands=[
        "git fetch origin main",
        "git checkout -q main",
        "systemctl restart myapp",
    ],
    exec_on_error="stop",
)
for b in batch_results:
    if not b.ok:
        print("Host %s failed (stopped early: %s)" % (b.host, b.stopped_early))
    for step in b.steps:
        print("  [%s] code=%d: %s" % (step.cmd, step.code, step.stdout.strip()))
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

## Testing helpers

Two additional builtins spin up a self-contained SSH server and client key for Starlark integration tests. **They are only registered when the runtime is started with `TestMode=true`** — `kite test` enables this automatically. They are not available in regular `kite run`/`kite exec`/`kite repl` scripts; attempting to call them there raises `undefined: test_server`.

| Function | Returns | Description |
|----------|---------|-------------|
| `ssh.test_server(user="testuser", password="")` | `ssh.test_server` | In-process SSH server on a random localhost port |
| `ssh.test_key()` | struct with `.path` | Generate an ed25519 key pair on disk; returns a struct whose `.path` is the private-key path |

### `ssh.test_server` methods

| Method | Description |
|--------|-------------|
| `.start()` | Begin accepting connections. Call before the first client connects |
| `.shutdown()` | Stop the server and release the port |
| `.port()` | Return the dynamically assigned listen port (int) |
| `.addr()` | Return the `host:port` string |
| `.add_file(path, content, mode="0644")` | Pre-populate a virtual file on the server for download/exec scenarios |
| `.uploaded(path)` | Return the file uploaded to `path` as `{"path": ..., "content": ..., "mode": ...}`, or `None` |
| `.handle_exec(fn)` | Register an exec handler: `fn(cmd) -> (stdout, stderr, exit_code)` |

### Example — validate SCP upload in a test

```python
def test_upload():
    srv = ssh.test_server(user="u", password="p")
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="u", password="p",
        port=srv.port(), host_key_check=False,
    )

    path = "/tmp/upload_test"
    write_text(path, "hello")
    client.upload(path, "/remote/file.txt")

    uploaded = srv.uploaded("/remote/file.txt")
    assert(uploaded.content == "hello")

    fs.path(path).remove()
    srv.shutdown()
```

### Example — authenticate with a generated key

```python
def test_pubkey_auth():
    key = ssh.test_key()
    srv = ssh.test_server(user="deploy")
    srv.start()

    client = ssh.config(
        hosts=["127.0.0.1"], user="deploy",
        key=key.path,
        port=srv.port(), host_key_check=False,
    )
    results = client.exec("echo ok")
    assert(results[0].ok)
    srv.shutdown()
```


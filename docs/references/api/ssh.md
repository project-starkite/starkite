---
title: "ssh"
description: "Remote command execution and file transfer over SSH"
weight: 4
keywords: [ssh, remote, command, execution, connect, scp, transfer, file, tunnel]
---

The `ssh` module provides remote command execution and file transfer over SSH connections.

## Architecture

The module exposes two distinct operational tiers:

1. **One-Shot Execution (`ssh.exec`, `ssh.copy_id`)**: Lightweight, direct functions for ad-hoc commands against literal hosts (`hosts=["192.168.1.10"]`). Uses flat credential parameters and does not accept `fleet` or bastion jump hosts.
2. **Configured Client (`ssh.config`)**: Client constructor supporting complex topologies (`fleet`, bastion jump tunnels) and execution controls. Uses structured `auth={...}` and `jump={...}` parameter objects.

---

## One-Shot Execution

For quick commands or checks against explicit target addresses without configuring a client, call `ssh.exec()` or `ssh.try_exec()` directly:

```python
# Execute single command on literal hosts
results = ssh.exec("uptime", hosts=["192.168.1.10", "192.168.1.11"], user="deploy", key="~/.ssh/id_ed25519")

# Multi-command pipeline
results = ssh.exec(
    commands = [
        "git pull origin main",
        "systemctl restart webapp",
    ],
    hosts         = ["192.168.1.10"],
    user          = "deploy",
    exec_on_error = "stop",
)

# Safe execution with try_exec
res = ssh.try_exec("hostname", hosts=["10.0.0.1"], user="admin", prompt=True)
if not res.ok:
    print("Execution failed:", res.error)
```

### One-Shot Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `cmd` | `string` | required (if no `commands`) | Command string to execute |
| `args` | `list[string]` | `[]` | Structured argument list (e.g. `ssh.exec("git", ["status", "-s"], ...)` ) |
| `commands` | `list[string]` | `[]` | Sequential command pipeline |
| `hosts` | `list[string]` \| `string` | required | Target host IP or hostname list |
| `user` | `string` | current OS user | Target SSH username |
| `key` | `string` | `""` | Path to private key file |
| `passphrase` | `string` | `""` | Passphrase for private key |
| `password` | `string` | `""` | Target SSH password |
| `use_agent` | `bool` | `False` | Authenticate with local `ssh-agent` |
| `prompt` | `bool` | `False` | Interactively prompt operator for password/passphrase if unset |
| `port` | `int` | `22` | Target SSH port |
| `sudo` | `bool` | `False` | Prefix command with sudo |
| `as_user` | `string` | `""` | Execute as target user with `sudo -u <user>` |
| `cwd` | `string` | `""` | Remote working directory |
| `env` | `dict` | `{}` | Remote environment variables |
| `timeout` | `string` | `"30s"` | Connection timeout |
| `exec_on_error` | `string` | `"stop"` | Multi-command error policy (`"stop"` or `"continue"`) |
| `host_key_check` | `bool` | `True` | Verify host key against known hosts |
| `dry_run` | `bool` | `False` | Output execution plan without opening connections |

> [!NOTE]
> Module-scope functions (`ssh.exec`, `ssh.copy_id`) target literal addresses only. Passing `fleet` or bastion parameters (`jump_host`, `jump`) will return an error instructing you to use `ssh.config()`.

---

## Client Configuration (`ssh.config`)

Initialize an `SSHClient` instance using `ssh.config()` when targeting `fleet` inventories, routing through bastion jump hosts, or managing persistent connections:

```python
# Targeting a Fleet with keypair and agent authentication
client = ssh.config(
    fleet = web_fleet,
    auth  = {
        "user":      "deploy",
        "key":       "~/.ssh/id_ed25519",
        "use_agent": True,
    },
    exec_max_workers = 8,
)

# Routing through a bastion jump host
client = ssh.config(
    hosts = ["10.0.1.10", "10.0.1.11"],
    auth  = {
        "user": "appuser",
        "key":  "~/.ssh/id_ed25519",
    },
    jump  = {
        "host": "bastion.corp.net",
        "user": "vladimir",
        "key":  "~/.ssh/bastion_key",
        "port": 22,
    },
)
```

### Top-Level Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `fleet` | `Fleet` \| `source` | `None` | Compute resource `Fleet` instance |
| `hosts` | `list[string]` \| `string` | `[]` | Target hostnames or IP addresses |
| `auth` | `dict` | `{}` | Target authentication dictionary |
| `jump` | `dict` | `None` | Bastion jump host configuration dictionary |
| `port` | `int` | `22` | Default target SSH port |
| `timeout` | `string` | `"30s"` | Connection timeout duration |
| `max_retries` | `int` | `3` | Maximum connection retry attempts |
| `exec_policy` | `string` | `"concurrent"` | Execution strategy (`"concurrent"` or `"linear"`) |
| `exec_max_workers` | `int` | `0` | Max concurrent worker goroutines (`0` = unconstrained) |
| `exec_on_error` | `string` | `"stop"` | Default error policy for batch commands (`"stop"` or `"continue"`) |
| `known_hosts_file` | `string` | `""` | Custom `known_hosts` file path |
| `host_key_check` | `bool` | `True` | Verify remote host public keys against known hosts |
| `keep_alive_interval` | `string` | `"30s"` | SSH keep-alive probe interval |
| `keep_alive_max` | `int` | `3` | Maximum unanswered keep-alive probes before disconnect |
| `sudo` | `bool` | `False` | Default sudo execution policy |
| `as_user` | `string` | `""` | Default user target for `sudo -u` |
| `cwd` | `string` | `""` | Default remote working directory |
| `env` | `dict` | `{}` | Default remote environment variables |
| `dry_run` | `bool` | `False` | Run in dry-run simulation mode |

### `auth` Sub-Object

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `user` | `string` | current OS user | SSH login username |
| `key` | `string` | `""` | Path to private key file (expands `~`) |
| `passphrase` | `string` | `""` | Passphrase to decrypt private key |
| `password` | `string` | `""` | Plaintext password |
| `use_agent` | `bool` | `False` | Authenticate using `ssh-agent` |
| `prompt` | `bool` | `False` | Interactively prompt in terminal if password or key passphrase is required |

### `jump` Sub-Object

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | `string` | required | Bastion gateway hostname or IP address |
| `port` | `int` | `22` | Bastion SSH port |
| `user` | `string` | `auth.user` | Bastion login username |
| `key` | `string` | `auth.key` | Bastion private key file path |
| `passphrase` | `string` | `auth.passphrase` | Passphrase for bastion private key |
| `password` | `string` | `auth.password` | Bastion login password |
| `use_agent` | `bool` | `auth.use_agent` | Authenticate to bastion with `ssh-agent` |
| `prompt` | `bool` | `auth.prompt` | Prompt in terminal for bastion credentials if required |

### `SSHClient` Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `auth` | `dict` | Resolved target authentication dictionary |
| `jump` | `dict` \| `None` | Resolved bastion jump dictionary |
| `hosts` | `list[string]` | List of resolved target host addresses |
| `fleet` | `Fleet` \| `None` | Associated fleet instance |
| `exec_policy` | `string` | Configured execution policy (`"concurrent"` or `"linear"`) |
| `exec_max_workers` | `int` | Concurrency worker limit |
| `exec_on_error` | `string` | Default batch error policy (`"stop"` or `"continue"`) |

### `SSHClient` Methods

All execution and file transfer methods support `try_` variants (e.g. `client.try_keyscan()`, `client.try_exec()`) that return `Result` values without raising on errors.

| Method | Description |
|--------|-------------|
| `client.exec(cmd, ...)` | Execute single command or pipeline across target hosts |
| `client.upload(src, dst, ...)` | Upload files or directories to target hosts via SCP |
| `client.download(src, dst, ...)` | Download files from target hosts via SCP |
| `client.copy_id(key, ...)` | Install public key into target hosts' `authorized_keys` |
| `client.keyscan(...)` | Discover and inspect public host keys across target hosts |

---

## Public Key Distribution (`copy_id`)

Installs public keys into `~/.ssh/authorized_keys` on target hosts idempotently.

### Module-Scope (`ssh.copy_id`)

```python
# Install public key onto direct hosts
ssh.copy_id(
    key     = "~/.ssh/id_ed25519.pub",
    hosts   = ["192.168.1.50", "192.168.1.51"],
    user    = "admin",
    prompt  = True,  # Prompt once in terminal for remote user password
    as_user = "deploy",
    sudo    = True,
)
```

### Method-Level (`client.copy_id`)

Inherits authentication, bastion routing, and target inventory from the configured client:

```python
client = ssh.config(
    fleet = pi_cluster,
    auth  = {"user": "pi", "password": "initial_password"},
    jump  = {"host": "bastion.lan", "user": "vladimir"},
)

# Installs key through bastion tunnel onto all fleet nodes
results = client.copy_id(key="~/.ssh/id_ed25519.pub", sudo=True)
```

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `key` | `string` | `""` | Public key string or file path (reads local `~/.ssh/` or `ssh-agent` if omitted) |
| `as_user` | `string` | `client.defaultAsUser` | Remote user whose `authorized_keys` will receive the key |
| `sudo` | `bool` | `client.defaultSudo` | Execute installation script with sudo |
| `prompt` | `bool` | `client.prompt` | Prompt in terminal for password |

---

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
| `passphrase` | `string` | `""` | Passphrase to encrypt private key |
| `path` | `string` | `""` | File path to write private key (`0600`) and `.pub` (`0644`) |
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

---

## Host Key Discovery (`ssh.keyscan`)

Discover and inspect remote public host keys without authentication (OpenSSH `ssh-keyscan` equivalent) to bootstrap `known_hosts`:

```python
# In-memory scan of target hosts
keys = ssh.keyscan(["192.168.1.50", "192.168.1.51"])
for k in keys:
    print(k.host, k.type, k.fingerprint)

# Scan and append to ~/.ssh/known_hosts (with deduplication and conflict checks)
ssh.keyscan(
    hosts = ["rbp4-1"],
    save  = True,                   # Append to known_hosts
    path  = "~/.ssh/known_hosts",    # Default
)

# Scan private hosts behind a bastion jump host
private_keys = ssh.keyscan(
    hosts = ["10.0.1.10", "10.0.1.11"],
    jump  = {"host": "bastion.corp.net", "user": "admin", "key": "~/.ssh/bastion_key"},
    save  = True,
)

# Scan through an existing client instance
client = ssh.config(hosts=["192.168.1.50"], auth={"user": "deploy"})
keys = client.keyscan(save=True)
```

### Keyscan Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `hosts` | `list[string]` \| `string` | required | Target host IP addresses or hostnames (can include `:port`) |
| `port` | `int` | `22` | Default target SSH port |
| `timeout` | `string` | `"5s"` | Connection and handshake timeout duration |
| `type` | `string` | `""` | Filter scan to a specific algorithm (`"ed25519"`, `"rsa"`, `"ecdsa"`) |
| `types` | `list[string]` | `[]` | Probe for multiple algorithms sequentially |
| `save` | `bool` | `False` | Append discovered keys to known_hosts file |
| `path` | `string` | `"~/.ssh/known_hosts"` | Destination known_hosts file path |
| `hash` | `bool` | `False` | Hash hostnames in known_hosts file (`|1|...`) |
| `jump` | `dict` | `None` | Optional bastion jump host configuration |

### `SSHHostKey` Attributes

| Attribute | Type | Description |
|-----------|------|-------------|
| `host` | `string` | Target hostname or IP address |
| `port` | `int` | Target SSH port |
| `type` | `string` | Host key algorithm (e.g. `"ssh-ed25519"`, `"rsa-sha2-512"`) |
| `public_key` | `string` | OpenSSH authorized public key line |
| `fingerprint` | `string` | SHA256 key fingerprint (`"SHA256:..."`) |
| `line` | `string` | Standard OpenSSH `known_hosts` entry line |
| `hashed_line` | `string` | Hashed OpenSSH `known_hosts` entry line |

---

## SSHResult

Returned by `client.exec()` (or items within `SSHBatchResult.steps`), one per host.

| Attribute | Type | Description |
|-----------|------|-------------|
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
    hosts = ["app-1", "app-2"],
    auth  = {
        "user": "deploy",
        "key":  "~/.ssh/deploy_key",
    },
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
    hosts = ["web-1", "web-2", "web-3"],
    auth  = {
        "user": "deploy",
        "key":  "~/.ssh/deploy_key",
    },
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


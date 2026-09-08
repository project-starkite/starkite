---
title: "Connecting to a daemon"
description: "Configure container daemon discovery, endpoint connections, and socket resolution for Docker and Podman"
weight: 2
---

# Connecting to a daemon

The `containers` module interacts directly with Docker and Podman REST API endpoints (v1.45) over Unix domain sockets, Windows named pipes, and TCP endpoints. It requires no external CLI binaries (`docker` or `podman`) in `$PATH`.

## Ambient Daemon Discovery

By default, calling `containers.config()` without arguments probes for active daemon endpoints in standard platform locations:

### Linux & macOS

1. `$DOCKER_HOST` environment variable if set.
2. Standard Docker socket: `/var/run/docker.sock`.
3. Podman rootless socket: `$XDG_RUNTIME_DIR/podman/podman.sock` or `/run/user/<uid>/podman/podman.sock`.
4. Podman system socket: `/run/podman/podman.sock`.

### Windows

1. `$DOCKER_HOST` environment variable if set.
2. Docker Desktop named pipe: `\\.\pipe\docker_engine`.

```python
def main():
    # Connect using ambient daemon discovery
    client = containers.config()

    # Verify connection
    if client.ping():
        info = client.version()
        print("Connected to daemon:", info["Version"], "API:", info["ApiVersion"])
```

The underlying client is written in pure Go without CGo dependencies, communicating over Unix domain sockets on Linux/macOS and Win32 named pipes on Windows.

---

## Explicit Daemon Configuration

To connect to a specific socket path, remote daemon, or configure custom timeouts, use `containers.config()`:

```python
def main():
    # Target Podman rootless socket
    podman = containers.config(
        host = "unix:///run/user/1000/podman/podman.sock",
        timeout = "15s",
    )

    # Target remote Docker daemon over TCP
    remote = containers.config(
        host = "tcp://192.168.1.50:2375",
        timeout = "30s",
    )

    # Target Windows named pipe explicitly
    win = containers.config(
        host = "npipe:////./pipe/docker_engine",
    )
```

### Supported Endpoint Formats

| Protocol / Scheme | Example Syntax | Platform |
|:---|:---|:---|
| Unix socket | `unix:///var/run/docker.sock` or `/var/run/docker.sock` | Linux, macOS |
| TCP endpoint | `tcp://127.0.0.1:2375` or `http://10.0.0.1:2375` | Cross-platform |
| Windows named pipe | `npipe:////./pipe/docker_engine` or `\\.\pipe\docker_engine` | Windows |

---

## Verifying Daemon Connection

The `Client` object provides helper methods and properties to verify connectivity and inspect daemon metadata:

```python
def print_daemon_info():
    client = containers.config()

    # Ping returns True if daemon responds
    if not client.ping():
        fail("Container daemon is unreachable")

    # Inspect endpoint properties
    print("Endpoint:", client.endpoint)
    print("Socket Path:", client.socket)

    # Retrieve version information dictionary
    v = client.version()
    print("Engine Version:", v["Version"])
    print("API Version:", v["ApiVersion"])
    print("Operating System:", v["Os"])
    print("Architecture:", v["Arch"])
```

### Version Dictionary Fields

* `Version` (*str*): The container engine version string (e.g., `"27.1.1"`).
* `ApiVersion` (*str*): Daemon REST API version (e.g., `"1.45"`).
* `MinAPIVersion` (*str*): Minimum supported API version.
* `GitCommit` (*str*): Git commit SHA of the engine build.
* `GoVersion` (*str*): Go runtime version used to build the engine.
* `Os` (*str*): Daemon operating system (e.g., `"linux"`, `"windows"`).
* `Arch` (*str*): Target architecture (e.g., `"amd64"`, `"arm64"`).
* `KernelVersion` (*str*): Operating system kernel version.

---

## Safe Error Handling (`try_config`)

To handle environments where a container engine may not be running without raising an unhandled script error, use `containers.try_config()`:

```python
def main():
    res = containers.try_config()
    if not res.ok:
        print("No container daemon available:", res.error)
        return

    client = res.value
    print("Container client ready:", client.endpoint)
```

---

## Permissions

Connecting to a container daemon socket requires the `containers.connect` permission. This capability is granted by default in the `allow-local` and `allow-all` permission profiles.

To run scripts that connect to a container daemon:

```bash
kite run ./check_daemon.star --permissions=allow-local
```

You can also grant fine-grained permissions:

```bash
kite run ./check_daemon.star --permissions=containers.connect
```
